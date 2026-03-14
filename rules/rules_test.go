package rules

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestSplitFileName(t *testing.T) {
	got := splitFileName("sample1_S1_L001_R1_001.fastq.gz", []string{"_", ".fastq.gz"})
	want := []string{"sample1", "S1", "L001", "R1", "001"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("splitFileName mismatch. got %v want %v", got, want)
	}
}

func TestGroupFilesAndFilterGroups(t *testing.T) {
	files := []string{
		"sample1_S1_L001_R1_001.fastq.gz",
		"sample1_S1_L002_R1_001.fastq.gz",
		"sample2_S2_L001_R1_001.fastq.gz",
	}
	rs := RuleSet{
		Delimiter:   []string{"_", ".fastq.gz"},
		RowRules:    RowRules{MatchParts: []int{0, 1}},
		ColumnRules: ColumnRules{MatchParts: []int{2}},
	}
	grouped, err := GroupFiles(files, rs)
	if err != nil {
		t.Fatalf("GroupFiles returned error: %v", err)
	}
	valid, invalid := FilterGroups(grouped, 2)
	if len(valid) != 1 {
		t.Errorf("expected 1 valid group, got %d", len(valid))
	}
	if len(invalid) != 1 {
		t.Errorf("expected 1 invalid group, got %d", len(invalid))
	}
}

func TestGroupFiles_DuplicateCollisionReturnsTypedError(t *testing.T) {
	files := []string{
		"sample1_S1_L001_R1_001.fastq.gz",
		"sample1__S1_L001_R1_001.fastq.gz",
		"sample1_S1_L001_R2_001.fastq.gz",
	}
	rs := RuleSet{
		Delimiter:   []string{"_", "."},
		RowRules:    RowRules{MatchParts: []int{0, 1, 2, 4, 5, 6}},
		ColumnRules: ColumnRules{MatchParts: []int{3}},
	}

	got, err := GroupFiles(files, rs)
	if err == nil {
		t.Fatalf("expected duplicate collision error, got nil")
	}
	if got != nil {
		t.Fatalf("expected nil grouped result on duplicate collision, got: %v", got)
	}

	var dupErr *DuplicateCollisionError
	if !errors.As(err, &dupErr) {
		t.Fatalf("expected DuplicateCollisionError, got %T", err)
	}
}

func TestGroupFiles_DuplicateCollisionErrorEntryFields(t *testing.T) {
	files := []string{
		"sample1_S1_L001_R1_001.fastq.gz",
		"sample1__S1_L001_R1_001.fastq.gz",
	}
	rs := RuleSet{
		Delimiter:   []string{"_", "."},
		RowRules:    RowRules{MatchParts: []int{0, 1, 2, 4, 5, 6}},
		ColumnRules: ColumnRules{MatchParts: []int{3}},
	}

	_, err := GroupFiles(files, rs)
	if err == nil {
		t.Fatalf("expected duplicate collision error, got nil")
	}

	var dupErr *DuplicateCollisionError
	if !errors.As(err, &dupErr) {
		t.Fatalf("expected DuplicateCollisionError, got %T", err)
	}
	if len(dupErr.Entries) != 1 {
		t.Fatalf("expected 1 duplicate entry, got %d", len(dupErr.Entries))
	}

	entry := dupErr.Entries[0]
	if entry.ReasonCode != "duplicate_role_in_row" {
		t.Fatalf("unexpected reason code: %s", entry.ReasonCode)
	}
	if entry.RowKey == "" {
		t.Fatalf("row key should not be empty")
	}
	if entry.RoleKey != "R1" {
		t.Fatalf("unexpected role key: %s", entry.RoleKey)
	}

	gotCandidates := append([]string(nil), entry.Candidates...)
	sort.Strings(gotCandidates)
	wantCandidates := []string{
		"sample1_S1_L001_R1_001.fastq.gz",
		"sample1__S1_L001_R1_001.fastq.gz",
	}
	sort.Strings(wantCandidates)
	if !reflect.DeepEqual(gotCandidates, wantCandidates) {
		t.Fatalf("unexpected candidates: got=%v want=%v", gotCandidates, wantCandidates)
	}

	gotSource := append([]string(nil), entry.SourceFileNames...)
	sort.Strings(gotSource)
	if !reflect.DeepEqual(gotSource, wantCandidates) {
		t.Fatalf("unexpected source file names: got=%v want=%v", gotSource, wantCandidates)
	}
}

func TestGroupFiles_DuplicateCollisionEntryV01SemanticLock(t *testing.T) {
	files := []string{
		"sample1_S1_L001_R1_001.fastq.gz",
		"sample1__S1_L001_R1_001.fastq.gz",
	}
	rs := RuleSet{
		Delimiter:   []string{"_", "."},
		RowRules:    RowRules{MatchParts: []int{0, 1, 2, 4, 5, 6}},
		ColumnRules: ColumnRules{MatchParts: []int{3}},
	}

	_, err := GroupFiles(files, rs)
	if err == nil {
		t.Fatalf("expected duplicate collision error, got nil")
	}

	var dupErr *DuplicateCollisionError
	if !errors.As(err, &dupErr) {
		t.Fatalf("expected DuplicateCollisionError, got %T", err)
	}
	if len(dupErr.Entries) == 0 {
		t.Fatalf("expected non-empty duplicate entries")
	}

	entry := dupErr.Entries[0]
	if entry.RoleKey != "R1" {
		t.Fatalf("expected RoleKey to reflect current column key semantics, got %q", entry.RoleKey)
	}
	if !reflect.DeepEqual(entry.Candidates, entry.SourceFileNames) {
		t.Fatalf("expected Candidates and SourceFileNames to match in v0.1 semantics: candidates=%v source=%v", entry.Candidates, entry.SourceFileNames)
	}
	if entry.Diagnostic != "" {
		t.Fatalf("expected empty Diagnostic in v0.1 semantics, got %q", entry.Diagnostic)
	}
}

func TestGroupFiles_DuplicateCollisionMultipleEntriesV01SemanticLock(t *testing.T) {
	files := []string{
		"sample1_S1_L001_R1_001.fastq.gz",
		"sample1__S1_L001_R1_001.fastq.gz",
		"sample2_S2_L001_R2_001.fastq.gz",
		"sample2__S2_L001_R2_001.fastq.gz",
	}
	rs := RuleSet{
		Delimiter:   []string{"_", "."},
		RowRules:    RowRules{MatchParts: []int{0, 1, 2, 4, 5, 6}},
		ColumnRules: ColumnRules{MatchParts: []int{3}},
	}

	_, err := GroupFiles(files, rs)
	if err == nil {
		t.Fatalf("expected duplicate collision error, got nil")
	}

	var dupErr *DuplicateCollisionError
	if !errors.As(err, &dupErr) {
		t.Fatalf("expected DuplicateCollisionError, got %T", err)
	}
	if len(dupErr.Entries) == 0 {
		t.Fatalf("expected non-empty duplicate entries")
	}
	if len(dupErr.Entries) != 2 {
		t.Fatalf("expected 2 duplicate entries, got %d", len(dupErr.Entries))
	}

	expectedByRole := map[string][]string{
		"R1": {
			"sample1_S1_L001_R1_001.fastq.gz",
			"sample1__S1_L001_R1_001.fastq.gz",
		},
		"R2": {
			"sample2_S2_L001_R2_001.fastq.gz",
			"sample2__S2_L001_R2_001.fastq.gz",
		},
	}

	for _, entry := range dupErr.Entries {
		if entry.ReasonCode != "duplicate_role_in_row" {
			t.Fatalf("unexpected reason code: %s", entry.ReasonCode)
		}

		wantCandidates, ok := expectedByRole[entry.RoleKey]
		if !ok {
			t.Fatalf("unexpected role key in duplicate entry: %s", entry.RoleKey)
		}

		gotCandidates := append([]string(nil), entry.Candidates...)
		sort.Strings(gotCandidates)
		wantCandidates = append([]string(nil), wantCandidates...)
		sort.Strings(wantCandidates)
		if !reflect.DeepEqual(gotCandidates, wantCandidates) {
			t.Fatalf("unexpected candidates for role %s: got=%v want=%v", entry.RoleKey, gotCandidates, wantCandidates)
		}

		delete(expectedByRole, entry.RoleKey)
	}

	if len(expectedByRole) != 0 {
		t.Fatalf("missing duplicate entries for roles: %v", expectedByRole)
	}
}

func TestGroupFiles_NoDuplicateKeepsNormalBehavior(t *testing.T) {
	files := []string{
		"sample1_S1_L001_R1_001.fastq.gz",
		"sample1_S1_L001_R2_001.fastq.gz",
	}
	rs := RuleSet{
		Delimiter:   []string{"_", "."},
		RowRules:    RowRules{MatchParts: []int{0, 1, 2, 4, 5, 6}},
		ColumnRules: ColumnRules{MatchParts: []int{3}},
	}

	grouped, err := GroupFiles(files, rs)
	if err != nil {
		t.Fatalf("unexpected GroupFiles error: %v", err)
	}
	if len(grouped) != 1 {
		t.Fatalf("expected 1 grouped row, got %d", len(grouped))
	}
	if grouped[0]["R1"] == "" || grouped[0]["R2"] == "" {
		t.Fatalf("expected both R1 and R2 in grouped row: %v", grouped[0])
	}
}

func TestIsValidRuleSet(t *testing.T) {
	rs := RuleSet{
		RowRules:    RowRules{MatchParts: []int{0, 1}},
		ColumnRules: ColumnRules{MatchParts: []int{1}},
	}
	if IsValidRuleSet(rs) {
		t.Errorf("expected rule set to be invalid due to duplicate index")
	}
}

func TestListFilesExclude(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "keep.txt"), []byte(""), 0644); err != nil {
		t.Fatalf("write keep.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "skip.json"), []byte(""), 0644); err != nil {
		t.Fatalf("write skip.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "invalid_files"), []byte(""), 0644); err != nil {
		t.Fatalf("write invalid_files: %v", err)
	}

	files, err := ListFilesExclude(dir, []string{"*.json", "invalid_files"})
	if err != nil {
		t.Fatalf("ListFilesExclude error: %v", err)
	}
	if len(files) != 1 || files[0] != "keep.txt" {
		t.Errorf("unexpected files: %v", files)
	}
}

func TestSaveInvalidFiles(t *testing.T) {
	dir := t.TempDir()
	rows := []map[string]string{
		{"a": "f1"},
		{"b": "f2"},
	}
	if err := SaveInvalidFiles(rows, dir); err != nil {
		t.Fatalf("SaveInvalidFiles error: %v", err)
	}
	matches, err := filepath.Glob(filepath.Join(dir, "invalid_files_*.txt"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("expected invalid file: %v %v", matches, err)
	}
	data, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("failed to read result: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 lines, got %d", len(lines))
	}
}

func TestSaveInvalidFiles_NoRows(t *testing.T) {
	dir := t.TempDir()
	if err := SaveInvalidFiles(nil, dir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "invalid_files_*.txt"))
	if len(matches) != 0 {
		t.Errorf("expected no output file, got %d", len(matches))
	}
}

func TestExportResultsCSV(t *testing.T) {
	dir := t.TempDir()
	result := map[int]map[string]string{
		0: {"A": "a.txt", "B": "b.txt"},
		1: {"A": "c.txt", "B": "d.txt"},
	}
	headers := []string{"A", "B"}
	if err := ExportResultsCSV(result, headers, dir); err != nil {
		t.Fatalf("ExportResultsCSV error: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "fileblock.csv"))
	if err != nil {
		t.Fatalf("failed to read csv: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 3 {
		t.Errorf("expected 3 lines, got %d", len(lines))
	}
	if !strings.Contains(lines[1], "a.txt") {
		t.Errorf("csv content unexpected: %v", lines[1])
	}
}

func TestExportResultsCSV_CanonicalBehavior_DataFollowsHeaderOrdering(t *testing.T) {
	dir := t.TempDir()
	result := map[int]map[string]string{
		0: {"R1": "row0-r1.fastq.gz", "R2": "row0-r2.fastq.gz"},
	}
	headers := []string{"R2", "R1"}

	if err := ExportResultsCSV(result, headers, dir); err != nil {
		t.Fatalf("ExportResultsCSV error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "fileblock.csv"))
	if err != nil {
		t.Fatalf("failed to read csv: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}

	headerColumns := strings.Split(lines[0], ",")
	if !reflect.DeepEqual(headerColumns, []string{"Row", "R2", "R1"}) {
		t.Fatalf("unexpected header row: %v", headerColumns)
	}

	dataColumns := strings.Split(lines[1], ",")
	if len(dataColumns) != 3 {
		t.Fatalf("unexpected data row width: %v", dataColumns)
	}
	if dataColumns[0] != "Row0" {
		t.Fatalf("unexpected row label: %s", dataColumns[0])
	}
	if dataColumns[1] != "row0-r2.fastq.gz" || dataColumns[2] != "row0-r1.fastq.gz" {
		t.Fatalf("expected data row to follow header ordering, got %v", dataColumns)
	}
}

// This test anchors current export behavior only.
// It records that a header-defined but missing column is exported as an empty cell.
func TestExportResultsCSV_CurrentBehaviorAnchor_MissingHeaderColumnExportsEmptyCell(t *testing.T) {
	dir := t.TempDir()
	result := map[int]map[string]string{
		0: {"R1": "row0-r1.fastq.gz"},
	}
	headers := []string{"R2", "R1"}

	if err := ExportResultsCSV(result, headers, dir); err != nil {
		t.Fatalf("ExportResultsCSV error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "fileblock.csv"))
	if err != nil {
		t.Fatalf("failed to read csv: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}

	headerColumns := strings.Split(lines[0], ",")
	if !reflect.DeepEqual(headerColumns, []string{"Row", "R2", "R1"}) {
		t.Fatalf("unexpected header row: %v", headerColumns)
	}

	dataColumns := strings.Split(lines[1], ",")
	if len(dataColumns) != 3 {
		t.Fatalf("unexpected data row width: %v", dataColumns)
	}
	if dataColumns[0] != "Row0" {
		t.Fatalf("unexpected row label: %s", dataColumns[0])
	}
	if dataColumns[1] != "" {
		t.Fatalf("expected missing header column to export as empty cell, got %q", dataColumns[1])
	}
	if dataColumns[2] != "row0-r1.fastq.gz" {
		t.Fatalf("expected existing column value to remain at its header position, got %v", dataColumns)
	}
}

// This test anchors current export behavior only.
// It records that a row-defined extra column is not surfaced when it is absent from headers.
func TestExportResultsCSV_CurrentBehaviorAnchor_ExtraRowColumnIsNotExported(t *testing.T) {
	dir := t.TempDir()
	result := map[int]map[string]string{
		0: {
			"R1":    "row0-r1.fastq.gz",
			"EXTRA": "row0-extra.fastq.gz",
		},
	}
	headers := []string{"R2", "R1"}

	if err := ExportResultsCSV(result, headers, dir); err != nil {
		t.Fatalf("ExportResultsCSV error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "fileblock.csv"))
	if err != nil {
		t.Fatalf("failed to read csv: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}

	headerColumns := strings.Split(lines[0], ",")
	if !reflect.DeepEqual(headerColumns, []string{"Row", "R2", "R1"}) {
		t.Fatalf("unexpected header row: %v", headerColumns)
	}

	dataColumns := strings.Split(lines[1], ",")
	if len(dataColumns) != 3 {
		t.Fatalf("expected export surface to contain only header-defined columns, got %v", dataColumns)
	}
	if dataColumns[0] != "Row0" {
		t.Fatalf("unexpected row label: %s", dataColumns[0])
	}
	if dataColumns[1] != "" {
		t.Fatalf("expected missing header-defined column to remain empty, got %q", dataColumns[1])
	}
	if dataColumns[2] != "row0-r1.fastq.gz" {
		t.Fatalf("expected header-defined column value to remain at its header position, got %v", dataColumns)
	}
	if strings.Contains(lines[1], "row0-extra.fastq.gz") {
		t.Fatalf("expected extra row column to be absent from CSV export surface, got %q", lines[1])
	}
}

// This is an investigation test only.
// It checks that the headers + rowMap boundary is already enough to observe missing and extra keys.
func TestDiagnosticsObservationBoundary_CanComputeMissingAndExtraKeysFromHeadersAndRowMap(t *testing.T) {
	headers := []string{"Row", "R1", "R2"}
	rowMap := map[string]string{
		"Row": "sample1",
		"R1":  "a.fastq",
		"X1":  "unexpected.fastq",
	}

	headerSet := make(map[string]struct{}, len(headers))
	for _, header := range headers {
		headerSet[header] = struct{}{}
	}

	missing := make([]string, 0)
	for _, header := range headers {
		if _, ok := rowMap[header]; !ok {
			missing = append(missing, header)
		}
	}

	extra := make([]string, 0)
	for key := range rowMap {
		if _, ok := headerSet[key]; !ok {
			extra = append(extra, key)
		}
	}

	sort.Strings(missing)
	sort.Strings(extra)

	if !reflect.DeepEqual(missing, []string{"R2"}) {
		t.Fatalf("unexpected missing keys: %v", missing)
	}
	if !reflect.DeepEqual(extra, []string{"X1"}) {
		t.Fatalf("unexpected extra keys: %v", extra)
	}
}

// This test verifies the extracted private helper boundary only.
// It does not introduce warning/report structure or change export semantics.
func TestCollectMissingAndExtraKeys(t *testing.T) {
	headers := []string{"Row", "R1", "R2"}
	rowMap := map[string]string{
		"Row": "sample1",
		"R1":  "a.fastq",
		"X1":  "unexpected.fastq",
	}

	missing, extra := collectMissingAndExtraKeys(headers, rowMap)

	if !reflect.DeepEqual(missing, []string{"R2"}) {
		t.Fatalf("unexpected missing keys: %v", missing)
	}
	if !reflect.DeepEqual(extra, []string{"X1"}) {
		t.Fatalf("unexpected extra keys: %v", extra)
	}
}

func TestLoadRuleSetFromFile(t *testing.T) {
	dir := t.TempDir()
	rs := RuleSet{Delimiter: []string{"_"}, Header: []string{"A"}, RowRules: RowRules{MatchParts: []int{0}}, ColumnRules: ColumnRules{MatchParts: []int{0}}}
	b, err := json.Marshal(rs)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "rule.json"), b, 0644); err != nil {
		t.Fatalf("write rule.json error: %v", err)
	}
	loaded, err := LoadRuleSetFromFile(dir)
	if err != nil {
		t.Fatalf("LoadRuleSetFromFile error: %v", err)
	}
	if loaded.Delimiter[0] != "_" || loaded.Header[0] != "A" {
		t.Errorf("loaded data mismatch: %+v", loaded)
	}
}
