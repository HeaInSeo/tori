package protoio

import (
	"os"
	"path/filepath"
	"testing"

	pb "github.com/seoyhaein/api-protos/gen/go/datablock/ichthys"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestSaveMessageAndLoadDataBlockRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "datablock.pb")
	want := &pb.DataBlock{
		UpdatedAt: timestamppb.Now(),
		Blocks: []*pb.FileBlock{
			{BlockId: "block-1"},
		},
	}

	if err := SaveMessage(path, want, 0o644); err != nil {
		t.Fatalf("SaveMessage error: %v", err)
	}

	got, err := LoadDataBlock(path)
	if err != nil {
		t.Fatalf("LoadDataBlock error: %v", err)
	}
	if got.GetUpdatedAt() == nil {
		t.Fatalf("expected UpdatedAt to be set")
	}
	if len(got.GetBlocks()) != 1 || got.GetBlocks()[0].GetBlockId() != "block-1" {
		t.Fatalf("unexpected blocks: %+v", got.GetBlocks())
	}
}

func TestSaveMessageSupportsFileBlockBinaryWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fileblock.pb")
	fileBlock := &pb.FileBlock{
		BlockId:       "file-block-1",
		ColumnHeaders: []string{"R1", "R2"},
	}

	if err := SaveMessage(path, fileBlock, 0o644); err != nil {
		t.Fatalf("SaveMessage error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if len(data) == 0 {
		t.Fatalf("expected non-empty protobuf file")
	}
}

func TestLoadDataBlockReturnsErrorForMissingFile(t *testing.T) {
	_, err := LoadDataBlock(filepath.Join(t.TempDir(), "missing.pb"))
	if err == nil {
		t.Fatalf("expected error for missing file")
	}
}

func TestLoadDataBlockReturnsErrorForInvalidBinary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invalid.pb")
	if err := os.WriteFile(path, []byte("not-a-protobuf"), 0o644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	_, err := LoadDataBlock(path)
	if err == nil {
		t.Fatalf("expected error for invalid protobuf binary")
	}
}

func TestLoadMessageDecodesIntoProvidedProtoMessage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fileblock.pb")
	want := &pb.FileBlock{
		BlockId:       "file-block-1",
		ColumnHeaders: []string{"R1", "R2"},
	}

	if err := SaveMessage(path, want, 0o644); err != nil {
		t.Fatalf("SaveMessage error: %v", err)
	}

	got := &pb.FileBlock{}
	if err := loadMessage(path, got); err != nil {
		t.Fatalf("loadMessage error: %v", err)
	}
	if got.GetBlockId() != want.GetBlockId() {
		t.Fatalf("unexpected block id: %q", got.GetBlockId())
	}
	if len(got.GetColumnHeaders()) != 2 || got.GetColumnHeaders()[1] != "R2" {
		t.Fatalf("unexpected headers: %+v", got.GetColumnHeaders())
	}
}
