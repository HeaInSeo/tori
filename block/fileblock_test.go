package block

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/seoyhaein/tori/protoio"
	pb "github.com/seoyhaein/tori/protos/ichthys/v1"
	"github.com/seoyhaein/tori/rules"
)

func TestGenerateFileBlock_PreservesDuplicateCollisionTypedError(t *testing.T) {
	dir := t.TempDir()
	ruleSet := rules.RuleSet{
		Delimiter:   []string{"_", "."},
		Header:      []string{"R1", "R2"},
		RowRules:    rules.RowRules{MatchParts: []int{0, 1, 2, 4, 5, 6}},
		ColumnRules: rules.ColumnRules{MatchParts: []int{3}},
	}

	data, err := json.Marshal(ruleSet)
	if err != nil {
		t.Fatalf("marshal rule set: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "rule.json"), data, 0644); err != nil {
		t.Fatalf("write rule.json: %v", err)
	}

	files := []string{
		"sample1_S1_L001_R1_001.fastq.gz",
		"sample1__S1_L001_R1_001.fastq.gz",
	}

	_, err = GenerateFileBlock(dir, files)
	if err == nil {
		t.Fatalf("expected duplicate collision error, got nil")
	}

	var dupErr *rules.DuplicateCollisionError
	if !errors.As(err, &dupErr) {
		t.Fatalf("expected DuplicateCollisionError via errors.As, got %T", err)
	}
	if len(dupErr.Entries) == 0 {
		t.Fatalf("expected non-empty duplicate entries")
	}

	entry := dupErr.Entries[0]
	if entry.ReasonCode != "duplicate_role_in_row" || entry.RoleKey != "R1" {
		t.Fatalf("expected duplicate structure information to survive wrapping: reason=%q role=%q", entry.ReasonCode, entry.RoleKey)
	}
}

func TestGenerateDataBlock_WritesMergedDataBlock(t *testing.T) {
	out := filepath.Join(t.TempDir(), "datablock.pb")
	input := []*pb.FileBlock{
		{BlockId: "block-1"},
		{BlockId: "block-2"},
	}

	if err := GenerateDataBlock(input, out); err != nil {
		t.Fatalf("GenerateDataBlock error: %v", err)
	}

	dataBlock, err := protoio.LoadDataBlock(out)
	if err != nil {
		t.Fatalf("LoadDataBlock error: %v", err)
	}

	if len(dataBlock.Blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(dataBlock.Blocks))
	}
	if dataBlock.Blocks[0].GetBlockId() != "block-1" || dataBlock.Blocks[1].GetBlockId() != "block-2" {
		t.Fatalf("unexpected block ids: %q %q", dataBlock.Blocks[0].GetBlockId(), dataBlock.Blocks[1].GetBlockId())
	}
	if dataBlock.GetUpdatedAt() == nil {
		t.Fatalf("expected UpdatedAt to be set")
	}
}

func TestConvertMapToFileBlockBuildsStableAssemblyShape(t *testing.T) {
	rows := map[int]map[string]string{
		20: {
			"R2": "sample_L001_R2.fastq.gz",
			"R1": "sample_L001_R1.fastq.gz",
		},
		10: {
			"R1": "other_L001_R1.fastq.gz",
		},
	}
	headers := []string{"R1", "R2"}

	got := ConvertMapToFileBlock(rows, headers, "block-123")
	if got == nil {
		t.Fatalf("expected non-nil FileBlock")
	}
	if got.GetBlockId() != "block-123" {
		t.Fatalf("unexpected block id: %q", got.GetBlockId())
	}
	if len(got.GetColumnHeaders()) != 2 || got.GetColumnHeaders()[0] != "R1" || got.GetColumnHeaders()[1] != "R2" {
		t.Fatalf("unexpected column headers: %#v", got.GetColumnHeaders())
	}
	if len(got.GetRows()) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(got.GetRows()))
	}
	if got.GetRows()[0].GetRowNumber() != 10 || got.GetRows()[1].GetRowNumber() != 20 {
		t.Fatalf("unexpected row ordering: %d, %d", got.GetRows()[0].GetRowNumber(), got.GetRows()[1].GetRowNumber())
	}
	if got.GetRows()[0].GetCells()["R1"] != "other_L001_R1.fastq.gz" {
		t.Fatalf("unexpected first row cells: %#v", got.GetRows()[0].GetCells())
	}
	if got.GetRows()[1].GetCells()["R1"] != "sample_L001_R1.fastq.gz" || got.GetRows()[1].GetCells()["R2"] != "sample_L001_R2.fastq.gz" {
		t.Fatalf("unexpected second row cells: %#v", got.GetRows()[1].GetCells())
	}
}

func TestMergeFileBlocksFromDataBuildsDataBlockShape(t *testing.T) {
	input := []*pb.FileBlock{
		{BlockId: "block-1"},
		{BlockId: "block-2"},
	}

	got, err := MergeFileBlocksFromData(input)
	if err != nil {
		t.Fatalf("MergeFileBlocksFromData error: %v", err)
	}
	if got == nil {
		t.Fatalf("expected non-nil DataBlock")
	}
	if got.GetUpdatedAt() == nil {
		t.Fatalf("expected UpdatedAt to be set")
	}
	if len(got.GetBlocks()) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(got.GetBlocks()))
	}
	if got.GetBlocks()[0].GetBlockId() != "block-1" || got.GetBlocks()[1].GetBlockId() != "block-2" {
		t.Fatalf("unexpected merged block ids: %q %q", got.GetBlocks()[0].GetBlockId(), got.GetBlocks()[1].GetBlockId())
	}
}
