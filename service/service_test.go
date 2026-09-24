package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/HeaInSeo/tori/config"
	d "github.com/HeaInSeo/tori/db"
	"github.com/HeaInSeo/tori/protoio"
	pb "github.com/HeaInSeo/tori/protos/ichthys/v1"
	_ "github.com/mattn/go-sqlite3"
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

// TDI-I2A service-path regressions. The db package tests call db.SyncFolders with the
// arguments chosen by the test; these drive DataBlockCliService itself, so they fail if
// the service stops passing a configured value through.

// sourceEnvelope reads the envelope the service path persisted.
func sourceEnvelope(t *testing.T, svc *DataBlockCliService) d.SourceEnvelope {
	t.Helper()
	env, ok, err := d.GetSourceEnvelope(context.Background(), svc.db)
	if err != nil || !ok {
		t.Fatalf("GetSourceEnvelope: ok=%v err=%v", ok, err)
	}
	return env
}

func TestServiceSyncFoldersAppliesConfiguredFolderExclusions(t *testing.T) {
	svc, rootDir := newTestDataBlockService(t)
	ctx := context.Background()

	excluded := filepath.Join(rootDir, "excluded_set")
	if err := os.MkdirAll(excluded, 0o755); err != nil {
		t.Fatalf("mkdir excluded_set: %v", err)
	}
	if err := os.WriteFile(filepath.Join(excluded, "stray.fastq.gz"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write excluded fixture: %v", err)
	}
	svc.cfg.FoldersExclusions = []string{"excluded_set"}

	if err := svc.SaveFolders(ctx); err != nil {
		t.Fatalf("SaveFolders error: %v", err)
	}
	if _, err := svc.SyncFolders(ctx); err != nil {
		t.Fatalf("SyncFolders error: %v", err)
	}

	// Observation honors the configured scope.
	folders, err := d.GetFoldersFromDB(svc.db)
	if err != nil {
		t.Fatalf("GetFoldersFromDB: %v", err)
	}
	for _, f := range folders {
		if filepath.Base(f.Path) == "excluded_set" {
			t.Fatalf("configured folder exclusion was ignored: %s is in the inventory", f.Path)
		}
	}

	// The persisted semantic revision carries the configured scope.
	first := sourceEnvelope(t, svc)
	canonical, ok, err := d.SourceRevisionCanonical(ctx, svc.db, first.SourceID, first.CurrentRevisionID)
	if err != nil || !ok {
		t.Fatalf("SourceRevisionCanonical: ok=%v err=%v", ok, err)
	}
	if !strings.Contains(canonical, "excluded_set") {
		t.Fatalf("revision does not record the configured folder exclusion: %s", canonical)
	}

	// A normal CLI seed (SaveFolders before the first sync) is a fresh bootstrap, not a
	// legacy adoption.
	if first.AdoptionOrigin != "bootstrap" || first.InventoryPredatesID {
		t.Errorf("seeded service flow adopted as origin=%q predates=%v, want bootstrap/false",
			first.AdoptionOrigin, first.InventoryPredatesID)
	}

	// Editing the configured folder scope mints a new revision on the same SourceID.
	svc.cfg.FoldersExclusions = append(svc.cfg.FoldersExclusions, "scratch")
	if _, err := svc.SyncFolders(ctx); err != nil {
		t.Fatalf("SyncFolders after scope edit: %v", err)
	}
	second := sourceEnvelope(t, svc)
	if second.SourceID != first.SourceID {
		t.Errorf("scope edit changed SourceID: %s → %s", first.SourceID, second.SourceID)
	}
	if second.CurrentRevisionID == first.CurrentRevisionID {
		t.Fatalf("editing the configured folder exclusions did not mint a revision (still %s)",
			first.CurrentRevisionID)
	}
	n, err := d.CountSourceRevisions(ctx, svc.db, first.SourceID)
	if err != nil {
		t.Fatalf("CountSourceRevisions: %v", err)
	}
	if n != 2 {
		t.Errorf("revision count = %d, want 2", n)
	}
}

// TDI-I2B through the real service caller: the accepted snapshot pins the configured
// source revision, and a configured scope edit is accepted as a new version while the
// earlier version keeps its revision.
func TestServiceSyncFoldersPinsAcceptedSourceRevision(t *testing.T) {
	svc, _ := newTestDataBlockService(t)
	ctx := context.Background()

	if err := svc.SaveFolders(ctx); err != nil {
		t.Fatalf("SaveFolders error: %v", err)
	}
	if _, err := svc.SyncFolders(ctx); err != nil {
		t.Fatalf("SyncFolders error: %v", err)
	}
	r1, ok, err := d.GetAcceptedSourceBasis(ctx, svc.db)
	if err != nil || !ok {
		t.Fatalf("GetAcceptedSourceBasis: ok=%v err=%v", ok, err)
	}
	if env := sourceEnvelope(t, svc); r1.SourceID != env.SourceID || r1.RevisionID != env.CurrentRevisionID {
		t.Fatalf("accepted pin %+v does not name the configured source revision %+v", r1, env)
	}

	svc.cfg.FilesExclusions = append(svc.cfg.FilesExclusions, "*.bam")
	res, err := svc.SyncFolders(ctx)
	if err != nil {
		t.Fatalf("SyncFolders after scope edit: %v", err)
	}
	if res.Outcome != d.OutcomeAcceptedUpdate {
		t.Fatalf("scope edit = %s (%s), want accepted-update", res.Outcome, res.Reason)
	}
	r2, ok, err := d.GetAcceptedSourceBasis(ctx, svc.db)
	if err != nil || !ok {
		t.Fatalf("GetAcceptedSourceBasis after edit: ok=%v err=%v", ok, err)
	}
	if r2.Version <= r1.Version || r2.RevisionID == r1.RevisionID || r2.SourceID != r1.SourceID {
		t.Fatalf("scope edit pin = %+v, want a new version of %s under a new revision (was %+v)", r2, r1.SourceID, r1)
	}
	if old, ok, err := d.GetSourceBasisAt(ctx, svc.db, r1.Version); err != nil || !ok || old != r1 {
		t.Fatalf("earlier version pin rewritten: %+v → %+v (ok=%v err=%v)", r1, old, ok, err)
	}
}

func TestServiceSyncFoldersRecordsConfiguredCredentialRef(t *testing.T) {
	svc, rootDir := newTestDataBlockService(t)
	ctx := context.Background()

	// Opaque references held in variables (gosec G101 flags credential-shaped literals
	// assigned to credential fields; these are references, never secret material).
	firstRef, rotatedRef := "service-ref-v1", "service-ref-v2"

	svc.cfg.AccessCredentialRef = firstRef
	if err := svc.SaveFolders(ctx); err != nil {
		t.Fatalf("SaveFolders error: %v", err)
	}
	if _, err := svc.SyncFolders(ctx); err != nil {
		t.Fatalf("SyncFolders error: %v", err)
	}
	before := sourceEnvelope(t, svc)

	svc.cfg.AccessCredentialRef = rotatedRef
	if _, err := svc.SyncFolders(ctx); err != nil {
		t.Fatalf("SyncFolders after rotation: %v", err)
	}
	after := sourceEnvelope(t, svc)

	_, want, err := d.SourceAccessEndpoint{RootDir: rootDir, CredentialRef: rotatedRef}.EndpointID()
	if err != nil {
		t.Fatalf("EndpointID: %v", err)
	}
	if after.CurrentEndpointID != want {
		t.Errorf("service path did not carry the configured credential ref: endpoint %s, want %s",
			after.CurrentEndpointID, want)
	}
	if after.SourceID != before.SourceID || after.CurrentRevisionID != before.CurrentRevisionID {
		t.Errorf("credential rotation moved identity or meaning: %+v → %+v", before, after)
	}
}

func TestSaveFoldersAndSyncFoldersGenerateDataBlock(t *testing.T) {
	svc, rootDir := newTestDataBlockService(t)
	ctx := context.Background()

	if err := svc.SaveFolders(ctx); err != nil {
		t.Fatalf("SaveFolders error: %v", err)
	}

	res, err := svc.SyncFolders(ctx)
	if err != nil {
		t.Fatalf("SyncFolders error: %v", err)
	}
	if res.Outcome != d.OutcomeAcceptedUpdate {
		t.Fatalf("expected SyncFolders to report accepted-update on first generation, got %v", res.Outcome)
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
