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

// P2-B regression: GenerateFileBlockFromDir writes invalid_files_<timestamp>.txt into the
// scanned directory. A subsequent scan of the same directory must NOT re-ingest that
// generated report as a SOURCE input. Steps: (1) first run creates a conflict report;
// (2) second run over the same directory; (3) the generated report is not observed as a
// source; (4) no spurious subject/member/fact appears from it. Without the invalid_files_*
// source-scan exclusion, the second run would ingest the report and produce a different
// (larger) result than the first.
func TestGenerateFileBlockFromDir_ExcludesGeneratedInvalidReport(t *testing.T) {
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

	// subject sample1 collides (writes an invalid report); subject sample2 is healthy.
	for _, name := range []string{
		"sample1_S1_L001_R1_001.fastq.gz",
		"sample1__S1_L001_R1_001.fastq.gz",
		"sample2_S2_L001_R1_001.fastq.gz",
		"sample2_S2_L001_R2_001.fastq.gz",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(""), 0600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	fb1, err := GenerateFileBlockFromDir(dir)
	if err != nil {
		t.Fatalf("first GenerateFileBlockFromDir: %v", err)
	}
	matches, err := filepath.Glob(filepath.Join(dir, "invalid_files_*.txt"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("expected first run to write one invalid report, got %v %v", matches, err)
	}

	fb2, err := GenerateFileBlockFromDir(dir)
	if err != nil {
		t.Fatalf("second GenerateFileBlockFromDir: %v", err)
	}

	// The second run must observe exactly the same source-derived result as the first:
	// the generated invalid report was NOT ingested as a source.
	if len(fb2.GetRows()) != len(fb1.GetRows()) {
		t.Fatalf("second run row count changed (report re-ingested?): first=%d second=%d",
			len(fb1.GetRows()), len(fb2.GetRows()))
	}
	if len(fb2.GetRows()) != 1 {
		t.Fatalf("expected exactly the healthy sibling row, got %d", len(fb2.GetRows()))
	}
	cells := fb2.GetRows()[0].GetCells()
	if cells["R1"] != "sample2_S2_L001_R1_001.fastq.gz" || cells["R2"] != "sample2_S2_L001_R2_001.fastq.gz" {
		t.Fatalf("unexpected second-run cells: %#v", cells)
	}
	// No spurious member sourced from a generated invalid_files_* report.
	for _, row := range fb2.GetRows() {
		for role, val := range row.GetCells() {
			if strings.HasPrefix(val, "invalid_files") {
				t.Fatalf("generated report leaked in as a source member: role=%s val=%s", role, val)
			}
		}
	}

	// A re-ingested report would also surface as a spurious fact in the invalid report
	// (it splits into a header-mismatched row rather than a published FileBlock row).
	// The second run's invalid report must not list any invalid_files_* filename.
	reports, err := filepath.Glob(filepath.Join(dir, "invalid_files_*.txt"))
	if err != nil {
		t.Fatalf("glob invalid reports: %v", err)
	}
	for _, r := range reports {
		content, rErr := os.ReadFile(r) //nolint:gosec // test-controlled temp path
		if rErr != nil {
			t.Fatalf("read %s: %v", r, rErr)
		}
		for _, line := range strings.Split(strings.TrimSpace(string(content)), "\n") {
			if strings.HasPrefix(line, "invalid_files") {
				t.Fatalf("generated report re-ingested as a source (listed in invalid report): %q", line)
			}
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
