package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	pb "github.com/seoyhaein/api-protos/gen/go/datablock/ichthys"
	"github.com/seoyhaein/tori/config"
	d "github.com/seoyhaein/tori/db"
	"github.com/seoyhaein/tori/protoio"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func writeRuleDirFixture(t *testing.T, rootDir string) {
	t.Helper()

	ruleDir := filepath.Join(rootDir, "sample_set")
	if err := os.MkdirAll(ruleDir, 0o755); err != nil {
		t.Fatalf("mkdir sample_set: %v", err)
	}

	ruleSet := map[string]any{
		"version":     "1",
		"delimiter":   []string{"_", "."},
		"header":      []string{"R1", "R2"},
		"rowRules":    map[string]any{"matchParts": []int{0, 1, 2, 4, 5, 6}},
		"columnRules": map[string]any{"matchParts": []int{3}},
		"sizeRules":   map[string]any{"minSize": 0, "maxSize": 1000},
	}
	data, err := json.Marshal(ruleSet)
	if err != nil {
		t.Fatalf("marshal rule set: %v", err)
	}
	if err := os.WriteFile(filepath.Join(ruleDir, "rule.json"), data, 0o644); err != nil {
		t.Fatalf("write rule.json: %v", err)
	}

	files := []string{
		"sample1_S1_L001_R1_001.fastq.gz",
		"sample1_S1_L001_R2_001.fastq.gz",
	}
	for _, name := range files {
		if err := os.WriteFile(filepath.Join(ruleDir, name), []byte("x"), 0o644); err != nil {
			t.Fatalf("write fixture file %s: %v", name, err)
		}
	}
}

func newTestDataBlockService(t *testing.T) (*DataBlockCliService, string) {
	t.Helper()

	rootDir := t.TempDir()
	writeRuleDirFixture(t, rootDir)

	dbPath := filepath.Join(rootDir, "file_monitor.db")
	dbConn, err := d.ConnectDB("sqlite3", dbPath, true)
	if err != nil {
		t.Fatalf("ConnectDB error: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := dbConn.Close(); closeErr != nil {
			t.Fatalf("close db: %v", closeErr)
		}
	})

	if err := d.InitializeDatabase(dbConn); err != nil {
		t.Fatalf("InitializeDatabase error: %v", err)
	}

	cfg := &config.Config{
		RootDir:         rootDir,
		FilesExclusions: []string{"*.json", "invalid_files", "*.csv", "*.pb"},
	}
	return NewDataBlockCliService(dbConn, cfg), rootDir
}

func writeDataBlockFixture(t *testing.T, rootDir string, updatedAt *timestamppb.Timestamp) *pb.DataBlock {
	t.Helper()

	dataBlock := &pb.DataBlock{
		UpdatedAt: updatedAt,
		Blocks: []*pb.FileBlock{
			{
				BlockId:       "block-1",
				ColumnHeaders: []string{"R1", "R2"},
			},
		},
	}

	path := filepath.Join(rootDir, "datablock.pb")
	if err := protoio.SaveMessage(path, dataBlock, 0o644); err != nil {
		t.Fatalf("SaveMessage error: %v", err)
	}
	return dataBlock
}

func TestSaveDataBlockToTextFile(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "db.txt")
	dataBlock := &pb.DataBlock{UpdatedAt: timestamppb.Now()}

	if err := SaveDataBlockToTextFile(out, dataBlock); err != nil {
		t.Fatalf("SaveDataBlockToTextFile error: %v", err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if len(data) == 0 {
		t.Fatalf("expected non-empty output file")
	}
	if !strings.Contains(string(data), "updated_at") {
		t.Fatalf("expected textproto output to contain updated_at field")
	}
}

func TestLoadDataBlock(t *testing.T) {
	rootDir := t.TempDir()
	want := writeDataBlockFixture(t, rootDir, timestamppb.Now())

	got, err := LoadDataBlock(filepath.Join(rootDir, "datablock.pb"))
	if err != nil {
		t.Fatalf("LoadDataBlock error: %v", err)
	}
	if got.GetUpdatedAt() == nil {
		t.Fatalf("expected UpdatedAt to be set")
	}
	if got.GetBlocks()[0].GetBlockId() != want.GetBlocks()[0].GetBlockId() {
		t.Fatalf("unexpected block id: %q", got.GetBlocks()[0].GetBlockId())
	}
}

func TestGetDataBlockWithoutTimestampReturnsCurrentData(t *testing.T) {
	svc, rootDir := newTestDataBlockService(t)
	want := writeDataBlockFixture(t, rootDir, timestamppb.Now())

	got, err := svc.GetDataBlock(context.Background(), nil)
	if err != nil {
		t.Fatalf("GetDataBlock error: %v", err)
	}
	if got == nil {
		t.Fatalf("expected DataBlock, got nil")
	}
	if got.GetBlocks()[0].GetBlockId() != want.GetBlocks()[0].GetBlockId() {
		t.Fatalf("unexpected block id: %q", got.GetBlocks()[0].GetBlockId())
	}
}

func TestGetDataBlockWithOlderTimestampReturnsCurrentData(t *testing.T) {
	svc, rootDir := newTestDataBlockService(t)
	serverTS := timestamppb.Now()
	writeDataBlockFixture(t, rootDir, serverTS)

	clientTS := timestamppb.New(serverTS.AsTime().Add(-1))
	got, err := svc.GetDataBlock(context.Background(), clientTS)
	if err != nil {
		t.Fatalf("GetDataBlock error: %v", err)
	}
	if got == nil {
		t.Fatalf("expected DataBlock, got nil")
	}
}

func TestGetDataBlockWithSameTimestampReturnsNil(t *testing.T) {
	svc, rootDir := newTestDataBlockService(t)
	serverTS := timestamppb.Now()
	writeDataBlockFixture(t, rootDir, serverTS)

	got, err := svc.GetDataBlock(context.Background(), serverTS)
	if err != nil {
		t.Fatalf("GetDataBlock error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil DataBlock when timestamps are equal")
	}
}

func TestGetDataBlockWithNewerTimestampReturnsError(t *testing.T) {
	svc, rootDir := newTestDataBlockService(t)
	serverTS := timestamppb.Now()
	writeDataBlockFixture(t, rootDir, serverTS)

	clientTS := timestamppb.New(serverTS.AsTime().Add(1))
	got, err := svc.GetDataBlock(context.Background(), clientTS)
	if err == nil {
		t.Fatalf("expected error when client datablock is newer")
	}
	if got != nil {
		t.Fatalf("expected nil DataBlock on error")
	}
}

func TestSaveFoldersAndSyncFoldersGenerateDataBlock(t *testing.T) {
	svc, rootDir := newTestDataBlockService(t)
	ctx := context.Background()

	if err := svc.SaveFolders(ctx); err != nil {
		t.Fatalf("SaveFolders error: %v", err)
	}

	updated, err := svc.SyncFolders(ctx)
	if err != nil {
		t.Fatalf("SyncFolders error: %v", err)
	}
	if !updated {
		t.Fatalf("expected SyncFolders to report updated=true on first generation")
	}

	dataBlockPath := filepath.Join(rootDir, "datablock.pb")
	info, err := os.Stat(dataBlockPath)
	if err != nil {
		t.Fatalf("stat datablock.pb: %v", err)
	}
	if info.Size() == 0 {
		t.Fatalf("expected datablock.pb to be non-empty")
	}

	dataBlock, err := LoadDataBlock(dataBlockPath)
	if err != nil {
		t.Fatalf("LoadDataBlock error: %v", err)
	}
	if len(dataBlock.GetBlocks()) == 0 {
		t.Fatalf("expected generated DataBlock to contain at least one block")
	}
}
