package block

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/HeaInSeo/tori/protoio"
	pb "github.com/HeaInSeo/tori/protos/ichthys/v1"
	"github.com/HeaInSeo/tori/rules"
)

// TDI-I12: the FileBlock publication authority must isolate a duplicate role-in-row to
// its single subject coordinate. A duplicate in subject "sample1" must NOT abort the
// whole batch: the healthy sibling subject "sample2" must still publish, and the
// collided subject's source files must remain visible (routed to invalid_files, never
// silently dropped and never resolved into an arbitrary healthy winner).
func TestGenerateFileBlock_IsolatesDuplicateConflictWithoutWholeBatchFailure(t *testing.T) {
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
	if err := os.WriteFile(filepath.Join(dir, "rule.json"), data, 0600); err != nil {
		t.Fatalf("write rule.json: %v", err)
	}

	files := []string{
		// subject sample1: duplicate R1 candidates -> CONFLICT (isolated)
		"sample1_S1_L001_R1_001.fastq.gz",
		"sample1__S1_L001_R1_001.fastq.gz",
		// subject sample2: healthy R1 + R2 -> published normally
		"sample2_S2_L001_R1_001.fastq.gz",
		"sample2_S2_L001_R2_001.fastq.gz",
	}

	fb, err := GenerateFileBlock(dir, files)
	if err != nil {
		t.Fatalf("expected no whole-batch error under I12 conflict isolation, got %v", err)
	}

	// Healthy sibling survives: exactly one valid published row (sample2 R1+R2).
	if len(fb.GetRows()) != 1 {
		t.Fatalf("expected 1 healthy sibling row to publish, got %d", len(fb.GetRows()))
	}
	cells := fb.GetRows()[0].GetCells()
	if cells["R1"] != "sample2_S2_L001_R1_001.fastq.gz" || cells["R2"] != "sample2_S2_L001_R2_001.fastq.gz" {
		t.Fatalf("unexpected healthy sibling row cells: %#v", cells)
	}

	// The conflicted subject's source files must remain visible in the invalid report,
	// and neither candidate may become a published healthy winner.
	matches, err := filepath.Glob(filepath.Join(dir, "invalid_files_*.txt"))
	if err != nil {
		t.Fatalf("glob invalid files: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected one invalid files report for the isolated conflict, got %d", len(matches))
	}
	report, err := os.ReadFile(matches[0]) //nolint:gosec // test-controlled temp path
	if err != nil {
		t.Fatalf("read invalid report: %v", err)
	}
	for _, want := range []string{"sample1_S1_L001_R1_001.fastq.gz", "sample1__S1_L001_R1_001.fastq.gz"} {
		if !strings.Contains(string(report), want) {
			t.Fatalf("expected conflicted candidate %q to remain visible in invalid report, got:\n%s", want, report)
		}
	}
}

func TestGenerateFileBlock_UsesHeaderExactValidationForMissingExtraRoles(t *testing.T) {
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
	if err := os.WriteFile(filepath.Join(dir, "rule.json"), data, 0600); err != nil {
		t.Fatalf("write rule.json: %v", err)
	}

	files := []string{
		"sample1_S1_L001_R1_001.fastq.gz",
		"sample1_S1_L001_EXTRA_001.fastq.gz",
	}

	fb, err := GenerateFileBlock(dir, files)
	if err != nil {
		t.Fatalf("GenerateFileBlock error: %v", err)
	}
	if len(fb.GetRows()) != 0 {
		t.Fatalf("expected missing/extra role row to be excluded from FileBlock rows, got %d rows", len(fb.GetRows()))
	}

	matches, err := filepath.Glob(filepath.Join(dir, "invalid_files_*.txt"))
	if err != nil {
		t.Fatalf("glob invalid files: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected one invalid files report, got %d", len(matches))
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
