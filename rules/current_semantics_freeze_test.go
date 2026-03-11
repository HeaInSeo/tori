package rules

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"sort"
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
	data, err := os.ReadFile(path)
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
	data, err := os.ReadFile(path)
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

// This test records known as-is overwrite behavior for duplicate collisions.
// It does not assert the final intended duplicate handling policy.
func TestCurrentSemanticsFreeze_FixtureD_DuplicateCollisionCurrentBehavior(t *testing.T) {
	fx := loadFreezeFixture(t, "fixture_d_duplicate_collision_current_behavior.json")

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

// This test records serialization/output behavior only (not grouping semantics).
// It does not assert the final intended column ordering policy.
func TestCurrentSemanticsFreeze_FixtureE_ExportColumnOrderCurrentSerializationBehavior(t *testing.T) {
	fx := loadExportFreezeFixture(t, "fixture_e_export_column_order_current_serialization_behavior.json")
	resultMap := toIndexedRows(t, fx.ResultMap)

	outputDir := t.TempDir()
	if err := ExportResultsCSV(resultMap, fx.Headers, outputDir); err != nil {
		t.Fatalf("ExportResultsCSV error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(outputDir, "fileblock.csv"))
	if err != nil {
		t.Fatalf("failed to read fileblock.csv: %v", err)
	}

	gotLines := strings.Split(strings.TrimSpace(string(data)), "\n")
	wantLines := fx.ExpectedCSVLines
	if strings.Join(gotLines, "\n") != strings.Join(wantLines, "\n") {
		t.Fatalf("csv lines mismatch\nwant=%v\ngot=%v", wantLines, gotLines)
	}
}
