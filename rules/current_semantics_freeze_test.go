package rules

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

type freezeExpected struct {
	Valid   []map[string]string `json:"valid"`
	Invalid []map[string]string `json:"invalid"`
}

type freezeFixture struct {
	Name     string         `json:"name"`
	RuleSet  RuleSet        `json:"ruleSet"`
	Files    []string       `json:"files"`
	Expected freezeExpected `json:"expected"`
}

type exportFreezeFixture struct {
	Name             string                       `json:"name"`
	Headers          []string                     `json:"headers"`
	ResultMap        map[string]map[string]string `json:"resultMap"`
	ExpectedCSVLines []string                     `json:"expectedCsvLines"`
}

func loadFreezeFixture(t *testing.T, fileName string) freezeFixture {
	t.Helper()
	path := filepath.Join("testdata", "phase_a1", fileName)
	data, err := os.ReadFile(path) //nolint:gosec // path is testdata/phase_a1/<literal fileName>, not external input
	if err != nil {
		t.Fatalf("failed to read fixture %s: %v", path, err)
	}
	var fx freezeFixture
	if err := json.Unmarshal(data, &fx); err != nil {
		t.Fatalf("failed to unmarshal fixture %s: %v", path, err)
	}
	return fx
}

func loadExportFreezeFixture(t *testing.T, fileName string) exportFreezeFixture {
	t.Helper()
	path := filepath.Join("testdata", "phase_a1", fileName)
	data, err := os.ReadFile(path) //nolint:gosec // path is testdata/phase_a1/<literal fileName>, not external input
	if err != nil {
		t.Fatalf("failed to read fixture %s: %v", path, err)
	}
	var fx exportFreezeFixture
	if err := json.Unmarshal(data, &fx); err != nil {
		t.Fatalf("failed to unmarshal fixture %s: %v", path, err)
	}
	return fx
}

func toIndexedRows(t *testing.T, in map[string]map[string]string) map[int]map[string]string {
	t.Helper()
	out := make(map[int]map[string]string, len(in))
	for k, v := range in {
		idx, err := strconv.Atoi(k)
		if err != nil {
			t.Fatalf("invalid row index key %q: %v", k, err)
		}
		out[idx] = v
	}
	return out
}

func rowSignature(row map[string]string) string {
	keys := make([]string, 0, len(row))
	for k := range row {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+row[k])
	}
	return strings.Join(parts, "|")
}

func signaturesFromIndexedRows(rows map[int]map[string]string) []string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, rowSignature(row))
	}
	sort.Strings(out)
	return out
}

func signaturesFromRows(rows []map[string]string) []string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, rowSignature(row))
	}
	sort.Strings(out)
	return out
}

func assertContiguousIndices(t *testing.T, rows map[int]map[string]string) {
	t.Helper()
	keys := make([]int, 0, len(rows))
	for k := range rows {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	for i, k := range keys {
		if i != k {
			t.Fatalf("valid row indices are not contiguous: got %v", keys)
		}
	}
}

func TestCurrentSemanticsFreeze_FixtureA_NormalPairEnd(t *testing.T) {
	fx := loadFreezeFixture(t, "fixture_a_normal_pair_end.json")

	grouped, err := GroupFiles(fx.Files, fx.RuleSet)
	if err != nil {
		t.Fatalf("GroupFiles error: %v", err)
	}
	valid, invalid := FilterGroups(grouped, len(fx.RuleSet.Header))

	assertContiguousIndices(t, valid)

	gotValid := signaturesFromIndexedRows(valid)
	wantValid := signaturesFromRows(fx.Expected.Valid)
	if strings.Join(gotValid, "\n") != strings.Join(wantValid, "\n") {
		t.Fatalf("valid rows mismatch\nwant=%v\ngot=%v", wantValid, gotValid)
	}

	gotInvalid := signaturesFromRows(invalid)
	wantInvalid := signaturesFromRows(fx.Expected.Invalid)
	if strings.Join(gotInvalid, "\n") != strings.Join(wantInvalid, "\n") {
		t.Fatalf("invalid rows mismatch\nwant=%v\ngot=%v", wantInvalid, gotInvalid)
	}
}

func TestCurrentSemanticsFreeze_FixtureB_InvalidRow(t *testing.T) {
	fx := loadFreezeFixture(t, "fixture_b_invalid_row.json")

	grouped, err := GroupFiles(fx.Files, fx.RuleSet)
	if err != nil {
		t.Fatalf("GroupFiles error: %v", err)
	}
	valid, invalid := FilterGroups(grouped, len(fx.RuleSet.Header))

	assertContiguousIndices(t, valid)

	gotValid := signaturesFromIndexedRows(valid)
	wantValid := signaturesFromRows(fx.Expected.Valid)
	if strings.Join(gotValid, "\n") != strings.Join(wantValid, "\n") {
		t.Fatalf("valid rows mismatch\nwant=%v\ngot=%v", wantValid, gotValid)
	}

	gotInvalid := signaturesFromRows(invalid)
	wantInvalid := signaturesFromRows(fx.Expected.Invalid)
	if strings.Join(gotInvalid, "\n") != strings.Join(wantInvalid, "\n") {
		t.Fatalf("invalid rows mismatch\nwant=%v\ngot=%v", wantInvalid, gotInvalid)
	}
}

func TestCurrentSemanticsFreeze_FixtureC_TokenizationConsecutiveDelimiters(t *testing.T) {
	fx := loadFreezeFixture(t, "fixture_c_tokenization_consecutive_delimiters.json")

	grouped, err := GroupFiles(fx.Files, fx.RuleSet)
	if err != nil {
		t.Fatalf("GroupFiles error: %v", err)
	}
	valid, invalid := FilterGroups(grouped, len(fx.RuleSet.Header))

	assertContiguousIndices(t, valid)

	gotValid := signaturesFromIndexedRows(valid)
	wantValid := signaturesFromRows(fx.Expected.Valid)
	if strings.Join(gotValid, "\n") != strings.Join(wantValid, "\n") {
		t.Fatalf("valid rows mismatch\nwant=%v\ngot=%v", wantValid, gotValid)
	}

	gotInvalid := signaturesFromRows(invalid)
	wantInvalid := signaturesFromRows(fx.Expected.Invalid)
	if strings.Join(gotInvalid, "\n") != strings.Join(wantInvalid, "\n") {
		t.Fatalf("invalid rows mismatch\nwant=%v\ngot=%v", wantInvalid, gotInvalid)
	}
}

// Fixture D reuses the A-1 duplicate-collision input, but asserts the current
// duplicate semantics instead of the historical overwrite anchor stored in the
// fixture's "expected" block (last-seen R1 silently wins). That overwrite
// expectation is retired and must never come back:
//   - canonical GroupFilesIsolated (docs/duplicate_policy_contract_v0.2.md)
//     publishes no healthy winner for the conflicted subject and keeps it
//     visible as a duplicate_role_in_row conflict;
//   - legacy GroupFiles loudly refuses the batch with DuplicateCollisionError.
//
// Tracking: HeaInSeo/tori#23.
func TestCurrentSemanticsFreeze_FixtureD_DuplicateCollisionFailClosed(t *testing.T) {
	fx := loadFreezeFixture(t, "fixture_d_duplicate_collision_current_behavior.json")

	wantCandidates := []string{
		"sample5_S5_L001_R1_001.fastq.gz",
		"sample5__S5_L001_R1_001.fastq.gz",
	}
	sort.Strings(wantCandidates)
	wantSources := append([]string(nil), fx.Files...)
	sort.Strings(wantSources)

	result := GroupFilesIsolated(fx.Files, fx.RuleSet)
	if len(result.Healthy) != 0 {
		t.Fatalf("conflicted subject must not publish a healthy winner (historical A-1 overwrite), got %v", result.Healthy)
	}
	if len(result.Conflicts) != 1 {
		t.Fatalf("expected 1 subject conflict, got %d: %+v", len(result.Conflicts), result.Conflicts)
	}
	conflict := result.Conflicts[0]
	if strings.Join(conflict.SourceFileNames, "\n") != strings.Join(wantSources, "\n") {
		t.Fatalf("conflict must keep every subject source file\nwant=%v\ngot=%v", wantSources, conflict.SourceFileNames)
	}
	if len(conflict.Roles) != 1 {
		t.Fatalf("expected 1 role collision, got %+v", conflict.Roles)
	}
	role := conflict.Roles[0]
	if role.ReasonCode != "duplicate_role_in_row" || role.Role != "R1" {
		t.Fatalf("unexpected role evidence: %+v", role)
	}
	if strings.Join(role.Candidates, "\n") != strings.Join(wantCandidates, "\n") {
		t.Fatalf("candidates mismatch\nwant=%v\ngot=%v", wantCandidates, role.Candidates)
	}

	grouped, err := GroupFiles(fx.Files, fx.RuleSet)
	if grouped != nil {
		t.Fatalf("legacy GroupFiles must not return a grouping on collision, got %v", grouped)
	}
	var dupErr *DuplicateCollisionError
	if !errors.As(err, &dupErr) {
		t.Fatalf("legacy GroupFiles: expected *DuplicateCollisionError, got %T: %v", err, err)
	}
	if len(dupErr.Entries) != 1 || dupErr.Entries[0].RoleKey != "R1" ||
		strings.Join(dupErr.Entries[0].Candidates, "\n") != strings.Join(wantCandidates, "\n") {
		t.Fatalf("unexpected legacy duplicate entries: %+v", dupErr.Entries)
	}
}

// This test records the historical serialization/output behavior only.
// It is retired from the active baseline after the first canonical column ordering patch.
//
// Tracking: HeaInSeo/tori#23. Classification: HISTORICAL RETIREMENT, not active
// coverage - this skip is expected and must not be counted as a passed gate.
// Kept for historical reference only.
func TestCurrentSemanticsFreeze_FixtureE_ExportColumnOrderCurrentSerializationBehavior(t *testing.T) {
	t.Skip("historical A-1 anchor retired from active baseline after canonical header-ordered export patch; tracked in HeaInSeo/tori#23")

	fx := loadExportFreezeFixture(t, "fixture_e_export_column_order_current_serialization_behavior.json")
	resultMap := toIndexedRows(t, fx.ResultMap)

	outputDir := t.TempDir()
	if err := ExportResultsCSV(resultMap, fx.Headers, outputDir); err != nil {
		t.Fatalf("ExportResultsCSV error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(outputDir, "fileblock.csv")) //nolint:gosec // outputDir is t.TempDir()-scoped, not external input
	if err != nil {
		t.Fatalf("failed to read fileblock.csv: %v", err)
	}

	gotLines := strings.Split(strings.TrimSpace(string(data)), "\n")
	wantLines := fx.ExpectedCSVLines
	if strings.Join(gotLines, "\n") != strings.Join(wantLines, "\n") {
		t.Fatalf("csv lines mismatch\nwant=%v\ngot=%v", wantLines, gotLines)
	}
}
