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

func TestNormalizeRoleKey_UsesRuleSetMapWithoutChangingObservedKeySemantics(t *testing.T) {
	rs := RuleSet{
		RoleNormalization: map[string]string{
			"bam":     "BAM",
			"bam_bai": "BAI",
		},
	}

	got, found := NormalizeRoleKey("bam", rs)
	if !found || got != "BAM" {
		t.Fatalf("expected bam to normalize to BAM, got=%q found=%v", got, found)
	}

	got, found = NormalizeRoleKey("bam_bai", rs)
	if !found || got != "BAI" {
		t.Fatalf("expected bam_bai to normalize to BAI, got=%q found=%v", got, found)
	}

	got, found = NormalizeRoleKey("unknown", rs)
	if found || got != "" {
		t.Fatalf("expected unknown key to remain unresolved, got=%q found=%v", got, found)
	}
}

func TestNormalizeRoleKey_MissingMapLeavesObservedKeyUnresolved(t *testing.T) {
	got, found := NormalizeRoleKey("R1", RuleSet{})
	if found || got != "" {
		t.Fatalf("expected missing normalization map to leave key unresolved, got=%q found=%v", got, found)
	}
}

func TestBuildRoleNormalizationPreview_DoesNotRewriteRowMap(t *testing.T) {
	rs := RuleSet{
		RoleNormalization: map[string]string{
			"bam":     "BAM",
			"bam_bai": "BAI",
		},
	}
	rowMap := map[string]string{
		"bam_bai": "NA12878_chr21_1x.bam.bai",
		"bam":     "NA12878_chr21_1x.bam",
		"debug":   "debug.txt",
	}

	got := BuildRoleNormalizationPreview(rowMap, rs)
	want := []RoleNormalizationPreviewEntry{
		{
			ObservedKey:    "bam",
			NormalizedRole: "BAM",
			Resolved:       true,
			FileName:       "NA12878_chr21_1x.bam",
		},
		{
			ObservedKey:    "bam_bai",
			NormalizedRole: "BAI",
			Resolved:       true,
			FileName:       "NA12878_chr21_1x.bam.bai",
		},
		{
			ObservedKey:    "debug",
			NormalizedRole: "",
			Resolved:       false,
			FileName:       "debug.txt",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected normalization preview: got=%#v want=%#v", got, want)
	}
	if rowMap["bam"] != "NA12878_chr21_1x.bam" || rowMap["bam_bai"] != "NA12878_chr21_1x.bam.bai" {
		t.Fatalf("expected observed-key row map to remain unchanged: %#v", rowMap)
	}
}

func TestBuildRowPreview_ContainsObservedRolesAndNormalizationPreview(t *testing.T) {
	rs := RuleSet{
		RoleNormalization: map[string]string{
			"bam":     "BAM",
			"bam_bai": "BAI",
		},
	}
	rowMap := map[string]string{
		"bam_bai": "NA12878_chr21_1x.bam.bai",
		"bam":     "NA12878_chr21_1x.bam",
	}

	got := BuildRowPreview(7, rowMap, rs)
	if got.RowIndex != 7 {
		t.Fatalf("unexpected row index: %d", got.RowIndex)
	}
	if !reflect.DeepEqual(got.ObservedRoles, []string{"bam", "bam_bai"}) {
		t.Fatalf("unexpected observed roles: %v", got.ObservedRoles)
	}
	if len(got.RoleNormalization) != 2 {
		t.Fatalf("expected 2 normalization entries, got %d", len(got.RoleNormalization))
	}
	if got.RoleNormalization[0].ObservedKey != "bam" || got.RoleNormalization[0].NormalizedRole != "BAM" {
		t.Fatalf("unexpected first normalization entry: %#v", got.RoleNormalization[0])
	}
	if got.RoleNormalization[1].ObservedKey != "bam_bai" || got.RoleNormalization[1].NormalizedRole != "BAI" {
		t.Fatalf("unexpected second normalization entry: %#v", got.RoleNormalization[1])
	}
	if _, ok := rowMap["BAM"]; ok {
		t.Fatalf("expected row preview not to rewrite row map: %#v", rowMap)
	}
}

func TestBuildResolverPreview_DeterministicRowOrdering(t *testing.T) {
	rs := RuleSet{
		RoleNormalization: map[string]string{
			"bam":     "BAM",
			"bam_bai": "BAI",
		},
	}
	resultMap := map[int]map[string]string{
		2: {
			"bam": "second.bam",
		},
		0: {
			"bam_bai": "first.bam.bai",
			"bam":     "first.bam",
		},
	}

	got := BuildResolverPreview(resultMap, rs)
	if got.SourceFileCount != 3 {
		t.Fatalf("unexpected source file count: %d", got.SourceFileCount)
	}
	if got.RowCount != 2 {
		t.Fatalf("unexpected row count: %d", got.RowCount)
	}
	if got.ObservedRoleCount != 3 {
		t.Fatalf("unexpected observed role count: %d", got.ObservedRoleCount)
	}
	if got.UnresolvedRoleCount != 0 {
		t.Fatalf("unexpected unresolved role count: %d", got.UnresolvedRoleCount)
	}
	if len(got.Rows) != 2 {
		t.Fatalf("expected 2 row previews, got %d", len(got.Rows))
	}
	if got.Rows[0].RowIndex != 0 || got.Rows[1].RowIndex != 2 {
		t.Fatalf("unexpected row preview ordering: %#v", got.Rows)
	}
	if got.Rows[0].RoleNormalization[0].NormalizedRole != "BAM" {
		t.Fatalf("unexpected first row normalization: %#v", got.Rows[0].RoleNormalization)
	}
	if got.Rows[1].RoleNormalization[0].ObservedKey != "bam" || got.Rows[1].RoleNormalization[0].NormalizedRole != "BAM" {
		t.Fatalf("unexpected second row normalization: %#v", got.Rows[1].RoleNormalization)
	}
}

func TestGenerateResolverPreview_BuildsPreviewFromCurrentGrouping(t *testing.T) {
	files := []string{
		"NA12878_chr21_1x.bam",
		"NA12878_chr21_1x.bam.bai",
	}
	rs := RuleSet{
		Delimiter:   []string{"_", "."},
		Header:      []string{"bam", "bam_bai"},
		RowRules:    RowRules{MatchParts: []int{0, 1, 2}},
		ColumnRules: ColumnRules{MatchParts: []int{3, 4}},
		RoleNormalization: map[string]string{
			"bam":     "BAM",
			"bam_bai": "BAI",
		},
	}

	got, err := GenerateResolverPreview(files, rs)
	if err != nil {
		t.Fatalf("GenerateResolverPreview error: %v", err)
	}
	if got.RowCount != 1 || len(got.Rows) != 1 {
		t.Fatalf("expected one preview row, got %#v", got)
	}
	if got.SourceFileCount != 2 {
		t.Fatalf("unexpected source file count: %d", got.SourceFileCount)
	}
	if got.ObservedRoleCount != 2 {
		t.Fatalf("unexpected observed role count: %d", got.ObservedRoleCount)
	}
	if got.UnresolvedRoleCount != 0 {
		t.Fatalf("unexpected unresolved role count: %d", got.UnresolvedRoleCount)
	}
	row := got.Rows[0]
	if !reflect.DeepEqual(row.ObservedRoles, []string{"bam", "bam_bai"}) {
		t.Fatalf("unexpected observed roles: %v", row.ObservedRoles)
	}
	if row.RoleNormalization[0].NormalizedRole != "BAM" || row.RoleNormalization[1].NormalizedRole != "BAI" {
		t.Fatalf("unexpected normalization preview: %#v", row.RoleNormalization)
	}
}

func TestGenerateResolverPreview_CountsUnresolvedRoles(t *testing.T) {
	files := []string{
		"NA12878_chr21_1x.bam",
		"NA12878_chr21_1x.bam.bai",
		"NA12878_chr21_1x.debug",
	}
	rs := RuleSet{
		Delimiter:   []string{"_", "."},
		Header:      []string{"bam", "bam_bai"},
		RowRules:    RowRules{MatchParts: []int{0, 1, 2}},
		ColumnRules: ColumnRules{MatchParts: []int{3, 4}},
		RoleNormalization: map[string]string{
			"bam":     "BAM",
			"bam_bai": "BAI",
		},
	}

	got, err := GenerateResolverPreview(files, rs)
	if err != nil {
		t.Fatalf("GenerateResolverPreview error: %v", err)
	}
	if got.SourceFileCount != 3 {
		t.Fatalf("unexpected source file count: %d", got.SourceFileCount)
	}
	if got.RowCount != 1 {
		t.Fatalf("unexpected row count: %d", got.RowCount)
	}
	if got.ObservedRoleCount != 3 {
		t.Fatalf("unexpected observed role count: %d", got.ObservedRoleCount)
	}
	if got.UnresolvedRoleCount != 1 {
		t.Fatalf("unexpected unresolved role count: %d", got.UnresolvedRoleCount)
	}
}

func TestGenerateResolverPreview_PreservesDuplicateCollisionError(t *testing.T) {
	files := []string{
		"sample1_S1_L001_R1_001.fastq.gz",
		"sample1__S1_L001_R1_001.fastq.gz",
	}
	rs := RuleSet{
		Delimiter:   []string{"_", "."},
		RowRules:    RowRules{MatchParts: []int{0, 1, 2, 4, 5, 6}},
		ColumnRules: ColumnRules{MatchParts: []int{3}},
	}

	_, err := GenerateResolverPreview(files, rs)
	if err == nil {
		t.Fatalf("expected duplicate collision error")
	}
	var dupErr *DuplicateCollisionError
	if !errors.As(err, &dupErr) {
		t.Fatalf("expected DuplicateCollisionError, got %T", err)
	}
	if len(dupErr.Entries) != 1 || dupErr.Entries[0].RoleKey != "R1" {
		t.Fatalf("unexpected duplicate error entries: %#v", dupErr.Entries)
	}
}

func TestGenerateResolverPreviewFromDir_ReadOnlyPreviewPath(t *testing.T) {
	dir := t.TempDir()
	rs := RuleSet{
		Delimiter:   []string{"_", "."},
		Header:      []string{"bam", "bam_bai"},
		RowRules:    RowRules{MatchParts: []int{0, 1, 2}},
		ColumnRules: ColumnRules{MatchParts: []int{3, 4}},
		RoleNormalization: map[string]string{
			"bam":     "BAM",
			"bam_bai": "BAI",
		},
	}
	b, err := json.Marshal(rs)
	if err != nil {
		t.Fatalf("marshal rule set: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "rule.json"), b, 0600); err != nil {
		t.Fatalf("write rule.json: %v", err)
	}
	for _, name := range []string{
		"NA12878_chr21_1x.bam",
		"NA12878_chr21_1x.bam.bai",
		"fileblock.csv",
		"ignored.pb",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("fixture"), 0600); err != nil {
			t.Fatalf("write fixture %s: %v", name, err)
		}
	}

	got, err := GenerateResolverPreviewFromDir(dir)
	if err != nil {
		t.Fatalf("GenerateResolverPreviewFromDir error: %v", err)
	}
	if got.SourceFileCount != 2 {
		t.Fatalf("expected excluded generated outputs to be ignored, source count=%d", got.SourceFileCount)
	}
	if got.RowCount != 1 || got.ObservedRoleCount != 2 || got.UnresolvedRoleCount != 0 {
		t.Fatalf("unexpected preview summary: %#v", got)
	}

	matches, err := filepath.Glob(filepath.Join(dir, "invalid_files_*.txt"))
	if err != nil {
		t.Fatalf("glob invalid reports: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("expected preview path not to write invalid reports, got %v", matches)
	}
}

func TestGenerateResolverPreviewFromDir_PreservesDuplicateCollisionError(t *testing.T) {
	dir := t.TempDir()
	rs := RuleSet{
		Delimiter:   []string{"_", "."},
		RowRules:    RowRules{MatchParts: []int{0, 1, 2, 4, 5, 6}},
		ColumnRules: ColumnRules{MatchParts: []int{3}},
	}
	b, err := json.Marshal(rs)
	if err != nil {
		t.Fatalf("marshal rule set: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "rule.json"), b, 0600); err != nil {
		t.Fatalf("write rule.json: %v", err)
	}
	for _, name := range []string{
		"sample1_S1_L001_R1_001.fastq.gz",
		"sample1__S1_L001_R1_001.fastq.gz",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("fixture"), 0600); err != nil {
			t.Fatalf("write fixture %s: %v", name, err)
		}
	}

	_, err = GenerateResolverPreviewFromDir(dir)
	if err == nil {
		t.Fatalf("expected duplicate collision error")
	}
	var dupErr *DuplicateCollisionError
	if !errors.As(err, &dupErr) {
		t.Fatalf("expected DuplicateCollisionError, got %T", err)
	}
	if len(dupErr.Entries) != 1 || dupErr.Entries[0].RoleKey != "R1" {
		t.Fatalf("unexpected duplicate entries: %#v", dupErr.Entries)
	}
}

func TestBuildSchemaValidationPreview_PairedEndValidHasNoMissingRequiredRole(t *testing.T) {
	files := []string{
		"sample1_S1_L001_R1_001.fastq.gz",
		"sample1_S1_L001_R2_001.fastq.gz",
	}
	rs := RuleSet{
		Delimiter:   []string{"_", "."},
		Header:      []string{"R1", "R2"},
		RowRules:    RowRules{MatchParts: []int{0, 1, 2, 4, 5, 6}},
		ColumnRules: ColumnRules{MatchParts: []int{3}},
	}

	resolverPreview, err := GenerateResolverPreview(files, rs)
	if err != nil {
		t.Fatalf("GenerateResolverPreview error: %v", err)
	}
	got := BuildSchemaValidationPreview(resolverPreview, rs)

	if got.MissingRequiredRoleCount != 0 {
		t.Fatalf("expected no missing required role candidates, got %#v", got)
	}
	if got.UnresolvedObservedRoleCount != 2 {
		t.Fatalf("expected R1/R2 to remain unresolved observations, got %#v", got)
	}
	if got.EntryCount != 2 || got.Entries[0].ReasonCode != "unresolved_observed_role" || got.Entries[1].ReasonCode != "unresolved_observed_role" {
		t.Fatalf("unexpected validation preview entries: %#v", got.Entries)
	}
}

func TestBuildSchemaValidationPreview_PairedEndMissingRoleSeparatesUnresolvedFromMissing(t *testing.T) {
	files := []string{
		"sample1_S1_L001_R1_001.fastq.gz",
	}
	rs := RuleSet{
		Delimiter:   []string{"_", "."},
		Header:      []string{"R1", "R2"},
		RowRules:    RowRules{MatchParts: []int{0, 1, 2, 4, 5, 6}},
		ColumnRules: ColumnRules{MatchParts: []int{3}},
	}

	resolverPreview, err := GenerateResolverPreview(files, rs)
	if err != nil {
		t.Fatalf("GenerateResolverPreview error: %v", err)
	}
	got := BuildSchemaValidationPreview(resolverPreview, rs)

	if got.UnresolvedObservedRoleCount != 1 {
		t.Fatalf("expected R1 to remain unresolved observation, got %#v", got)
	}
	if got.MissingRequiredRoleCount != 1 {
		t.Fatalf("expected one missing required role candidate, got %#v", got)
	}
	if got.EntryCount != 2 {
		t.Fatalf("expected two validation preview entries, got %#v", got)
	}
	if got.Entries[0].ReasonCode != "unresolved_observed_role" || got.Entries[0].ObservedKey != "R1" {
		t.Fatalf("expected unresolved R1 entry first, got %#v", got.Entries)
	}
	if got.Entries[1].ReasonCode != "missing_required_role" || got.Entries[1].Role != "R2" {
		t.Fatalf("expected missing R2 entry second, got %#v", got.Entries)
	}
}

func TestBuildSchemaValidationPreview_ExtraObservedRoleIsSeparateObservation(t *testing.T) {
	files := []string{
		"sample1_S1_L001_R1_001.fastq.gz",
		"sample1_S1_L001_R2_001.fastq.gz",
		"sample1_S1_L001_RX_001.fastq.gz",
	}
	rs := RuleSet{
		Delimiter:   []string{"_", "."},
		Header:      []string{"R1", "R2"},
		RowRules:    RowRules{MatchParts: []int{0, 1, 2, 4, 5, 6}},
		ColumnRules: ColumnRules{MatchParts: []int{3}},
		RoleNormalization: map[string]string{
			"R1": "R1",
			"R2": "R2",
		},
	}

	resolverPreview, err := GenerateResolverPreview(files, rs)
	if err != nil {
		t.Fatalf("GenerateResolverPreview error: %v", err)
	}
	got := BuildSchemaValidationPreview(resolverPreview, rs)

	if got.MissingRequiredRoleCount != 0 {
		t.Fatalf("expected no missing required role candidates, got %#v", got)
	}
	if got.UnresolvedObservedRoleCount != 1 {
		t.Fatalf("expected RX to remain unresolved observation, got %#v", got)
	}
	if got.ExtraObservedRoleCount != 1 {
		t.Fatalf("expected one extra observed role candidate, got %#v", got)
	}
	if got.EntryCount != 2 {
		t.Fatalf("expected unresolved and extra entries, got %#v", got)
	}
	if got.Entries[1].ReasonCode != "extra_observed_role" || got.Entries[1].ObservedKey != "RX" {
		t.Fatalf("expected extra RX entry, got %#v", got.Entries)
	}
}

func TestBuildTypedRoleValidationPreview_BAMBAICompleteHasNoMissingRequiredRole(t *testing.T) {
	files := []string{
		"NA12878_chr21_1x.bam",
		"NA12878_chr21_1x.bam.bai",
	}
	rs := RuleSet{
		Delimiter:   []string{"_", "."},
		Header:      []string{"bam", "bam_bai"},
		RowRules:    RowRules{MatchParts: []int{0, 1, 2}},
		ColumnRules: ColumnRules{MatchParts: []int{3, 4}},
		RoleNormalization: map[string]string{
			"bam":     "BAM",
			"bam_bai": "BAI",
		},
	}

	resolverPreview, err := GenerateResolverPreview(files, rs)
	if err != nil {
		t.Fatalf("GenerateResolverPreview error: %v", err)
	}
	got := BuildTypedRoleValidationPreview(resolverPreview, []string{"BAM", "BAI"})

	if got.EntryCount != 0 {
		t.Fatalf("expected complete BAM/BAI typed role preview to have no entries, got %#v", got)
	}
	if got.MissingRequiredRoleCount != 0 || got.UnresolvedObservedRoleCount != 0 || got.ExtraObservedRoleCount != 0 {
		t.Fatalf("unexpected typed role validation summary: %#v", got)
	}
}

func TestBuildTypedRoleValidationPreview_BAMOnlyReportsMissingBAI(t *testing.T) {
	files := []string{
		"NA12878_chr21_1x.bam",
	}
	rs := RuleSet{
		Delimiter:   []string{"_", "."},
		Header:      []string{"bam", "bam_bai"},
		RowRules:    RowRules{MatchParts: []int{0, 1, 2}},
		ColumnRules: ColumnRules{MatchParts: []int{3, 4}},
		RoleNormalization: map[string]string{
			"bam":     "BAM",
			"bam_bai": "BAI",
		},
	}

	resolverPreview, err := GenerateResolverPreview(files, rs)
	if err != nil {
		t.Fatalf("GenerateResolverPreview error: %v", err)
	}
	got := BuildTypedRoleValidationPreview(resolverPreview, []string{"BAM", "BAI"})

	if got.MissingRequiredRoleCount != 1 {
		t.Fatalf("expected one typed missing required role candidate, got %#v", got)
	}
	if got.EntryCount != 1 {
		t.Fatalf("expected one typed role validation entry, got %#v", got)
	}
	entry := got.Entries[0]
	if entry.ReasonCode != "missing_required_role" || entry.Role != "BAI" {
		t.Fatalf("expected missing BAI typed role entry, got %#v", entry)
	}
}

func TestBuildTypedRoleValidationPreview_BAIOnlyReportsMissingBAM(t *testing.T) {
	files := []string{
		"NA12878_chr21_1x.bam.bai",
	}
	rs := RuleSet{
		Delimiter:   []string{"_", "."},
		Header:      []string{"bam", "bam_bai"},
		RowRules:    RowRules{MatchParts: []int{0, 1, 2}},
		ColumnRules: ColumnRules{MatchParts: []int{3, 4}},
		RoleNormalization: map[string]string{
			"bam":     "BAM",
			"bam_bai": "BAI",
		},
	}

	resolverPreview, err := GenerateResolverPreview(files, rs)
	if err != nil {
		t.Fatalf("GenerateResolverPreview error: %v", err)
	}
	got := BuildTypedRoleValidationPreview(resolverPreview, []string{"BAM", "BAI"})

	if got.MissingRequiredRoleCount != 1 {
		t.Fatalf("expected one typed missing required role candidate, got %#v", got)
	}
	if got.EntryCount != 1 {
		t.Fatalf("expected one typed role validation entry, got %#v", got)
	}
	entry := got.Entries[0]
	if entry.ReasonCode != "missing_required_role" || entry.Role != "BAM" {
		t.Fatalf("expected missing BAM typed role entry, got %#v", entry)
	}
}

func TestBuildTypedRoleValidationPreview_StaysSeparateFromObservedKeySchemaPreview(t *testing.T) {
	files := []string{
		"NA12878_chr21_1x.bam",
	}
	rs := RuleSet{
		Delimiter:   []string{"_", "."},
		Header:      []string{"bam", "bam_bai"},
		RowRules:    RowRules{MatchParts: []int{0, 1, 2}},
		ColumnRules: ColumnRules{MatchParts: []int{3, 4}},
		RoleNormalization: map[string]string{
			"bam":     "BAM",
			"bam_bai": "BAI",
		},
	}

	resolverPreview, err := GenerateResolverPreview(files, rs)
	if err != nil {
		t.Fatalf("GenerateResolverPreview error: %v", err)
	}
	observedKeyPreview := BuildSchemaValidationPreview(resolverPreview, rs)
	typedRolePreview := BuildTypedRoleValidationPreview(resolverPreview, []string{"BAM", "BAI"})

	if observedKeyPreview.Entries[0].Role != "bam_bai" {
		t.Fatalf("expected observed-key preview to report missing bam_bai, got %#v", observedKeyPreview.Entries)
	}
	if typedRolePreview.Entries[0].Role != "BAI" {
		t.Fatalf("expected typed-role preview to report missing BAI, got %#v", typedRolePreview.Entries)
	}
}

func TestBuildPrimaryIndexPairingPreview_BAIOnlyReportsUnpairedIndex(t *testing.T) {
	files := []string{
		"NA12878_chr21_1x.bam.bai",
	}
	rs := RuleSet{
		Delimiter:   []string{"_", "."},
		Header:      []string{"bam", "bam_bai"},
		RowRules:    RowRules{MatchParts: []int{0, 1, 2}},
		ColumnRules: ColumnRules{MatchParts: []int{3, 4}},
		RoleNormalization: map[string]string{
			"bam":     "BAM",
			"bam_bai": "BAI",
		},
	}

	resolverPreview, err := GenerateResolverPreview(files, rs)
	if err != nil {
		t.Fatalf("GenerateResolverPreview error: %v", err)
	}
	got := BuildPrimaryIndexPairingPreview(resolverPreview, "BAM", "BAI")

	if got.EntryCount != 1 {
		t.Fatalf("expected one pairing preview entry, got %#v", got)
	}
	entry := got.Entries[0]
	if entry.ReasonCode != "unpaired_index_role" || entry.Role != "BAI" {
		t.Fatalf("expected unpaired BAI index entry, got %#v", entry)
	}
	if entry.ObservedKey != "bam_bai" || entry.NormalizedRole != "BAI" || entry.FileName == "" {
		t.Fatalf("expected index source context to be preserved, got %#v", entry)
	}
}

func TestBuildPrimaryIndexPairingPreview_BAMOnlyHasNoPairingEntry(t *testing.T) {
	files := []string{
		"NA12878_chr21_1x.bam",
	}
	rs := RuleSet{
		Delimiter:   []string{"_", "."},
		Header:      []string{"bam", "bam_bai"},
		RowRules:    RowRules{MatchParts: []int{0, 1, 2}},
		ColumnRules: ColumnRules{MatchParts: []int{3, 4}},
		RoleNormalization: map[string]string{
			"bam":     "BAM",
			"bam_bai": "BAI",
		},
	}

	resolverPreview, err := GenerateResolverPreview(files, rs)
	if err != nil {
		t.Fatalf("GenerateResolverPreview error: %v", err)
	}
	got := BuildPrimaryIndexPairingPreview(resolverPreview, "BAM", "BAI")

	if got.EntryCount != 0 {
		t.Fatalf("expected no pairing preview entries for BAM-only row, got %#v", got)
	}
}

func TestBuildPrimaryIndexPairingPreview_BAMBAICompleteHasNoPairingEntry(t *testing.T) {
	files := []string{
		"NA12878_chr21_1x.bam",
		"NA12878_chr21_1x.bam.bai",
	}
	rs := RuleSet{
		Delimiter:   []string{"_", "."},
		Header:      []string{"bam", "bam_bai"},
		RowRules:    RowRules{MatchParts: []int{0, 1, 2}},
		ColumnRules: ColumnRules{MatchParts: []int{3, 4}},
		RoleNormalization: map[string]string{
			"bam":     "BAM",
			"bam_bai": "BAI",
		},
	}

	resolverPreview, err := GenerateResolverPreview(files, rs)
	if err != nil {
		t.Fatalf("GenerateResolverPreview error: %v", err)
	}
	got := BuildPrimaryIndexPairingPreview(resolverPreview, "BAM", "BAI")

	if got.EntryCount != 0 {
		t.Fatalf("expected no pairing preview entries for complete BAM/BAI row, got %#v", got)
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

func TestIsValidRuleSet_NegativeIndex(t *testing.T) {
	rs := RuleSet{
		RowRules:    RowRules{MatchParts: []int{-1, 0}},
		ColumnRules: ColumnRules{MatchParts: []int{1}},
	}
	if IsValidRuleSet(rs) {
		t.Errorf("expected rule set to be invalid due to negative index")
	}
}

func TestFilterGroups_Deterministic(t *testing.T) {
	resultMap := map[int]map[string]string{
		0: {"R1": "f0-r1", "R2": "f0-r2"},
		1: {"R1": "f1-r1"},
		2: {"R1": "f2-r1", "R2": "f2-r2"},
		3: {"R1": "f3-r1"},
	}
	for range 20 {
		valid, invalid := FilterGroups(resultMap, 2)
		if len(valid) != 2 || len(invalid) != 2 {
			t.Fatalf("unexpected counts: valid=%d invalid=%d", len(valid), len(invalid))
		}
		if valid[0]["R1"] != "f0-r1" || valid[1]["R1"] != "f2-r1" {
			t.Fatalf("non-deterministic valid ordering: %v", valid)
		}
	}
}

func TestFilterGroupsByHeaders_ExactMatch(t *testing.T) {
	resultMap := map[int]map[string]string{
		0: {"R1": "f0-r1", "R2": "f0-r2"},
		1: {"R1": "f1-r1", "R3": "f1-r3"},
		2: {"R1": "f2-r1", "R2": "f2-r2"},
	}
	headers := []string{"R1", "R2"}
	valid, invalid := FilterGroupsByHeaders(resultMap, headers)
	if len(valid) != 2 {
		t.Fatalf("expected 2 valid, got %d", len(valid))
	}
	if len(invalid) != 1 {
		t.Fatalf("expected 1 invalid, got %d", len(invalid))
	}
	if valid[0]["R1"] != "f0-r1" || valid[1]["R1"] != "f2-r1" {
		t.Fatalf("unexpected valid ordering: %v", valid)
	}
	if invalid[0]["R3"] != "f1-r3" {
		t.Fatalf("unexpected invalid row: %v", invalid)
	}
}

func TestListFilesExclude(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "keep.txt"), []byte(""), 0600); err != nil {
		t.Fatalf("write keep.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "skip.json"), []byte(""), 0600); err != nil {
		t.Fatalf("write skip.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "invalid_files"), []byte(""), 0600); err != nil {
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
	data, err := os.ReadFile(filepath.Join(dir, "fileblock.csv")) //nolint:gosec // dir is t.TempDir()-scoped, not external input
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

	data, err := os.ReadFile(filepath.Join(dir, "fileblock.csv")) //nolint:gosec // dir is t.TempDir()-scoped, not external input
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

	data, err := os.ReadFile(filepath.Join(dir, "fileblock.csv")) //nolint:gosec // dir is t.TempDir()-scoped, not external input
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

	data, err := os.ReadFile(filepath.Join(dir, "fileblock.csv")) //nolint:gosec // dir is t.TempDir()-scoped, not external input
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

// This is a narrow synthetic proof only.
// It shows that the pipeline-facing missing/extra observational contract does not contradict
// the current permissive export surface for a single row case.
func TestPipelineFacingBindingProof_SingleSyntheticCase(t *testing.T) {
	dir := t.TempDir()
	headers := []string{"sample_id", "bam", "bai"}
	rowMap := map[string]string{
		"sample_id":  "S1",
		"bam":        "",
		"debug_note": "tmp",
	}

	missing, extra := collectMissingAndExtraKeys(headers, rowMap)
	if !reflect.DeepEqual(missing, []string{"bam", "bai"}) {
		t.Fatalf("unexpected missing keys: %v", missing)
	}
	if !reflect.DeepEqual(extra, []string{"debug_note"}) {
		t.Fatalf("unexpected extra keys: %v", extra)
	}

	result := map[int]map[string]string{0: rowMap}
	if err := ExportResultsCSV(result, headers, dir); err != nil {
		t.Fatalf("ExportResultsCSV error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "fileblock.csv")) //nolint:gosec // dir is t.TempDir()-scoped, not external input
	if err != nil {
		t.Fatalf("failed to read csv: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}

	dataColumns := strings.Split(lines[1], ",")
	if !reflect.DeepEqual(dataColumns, []string{"Row0", "S1", "", ""}) {
		t.Fatalf("unexpected export row: %v", dataColumns)
	}
	if strings.Contains(lines[1], "tmp") {
		t.Fatalf("expected extra key to remain absent from export surface, got %q", lines[1])
	}
}

// This test verifies the extracted private helper boundary only.
// It keeps current permissive export semantics fixed without adding diagnostics surfaces.
func TestCollectMissingAndExtraKeys_CurrentSemantics(t *testing.T) {
	headers := []string{"R2", "R1", "R3"}
	rowMap := map[string]string{
		"R1":    "a.fastq",
		"R2":    "",
		"X2":    "z.fastq",
		"EXTRA": "unexpected.fastq",
	}

	missing, extra := collectMissingAndExtraKeys(headers, rowMap)

	if !reflect.DeepEqual(missing, []string{"R2", "R3"}) {
		t.Fatalf("unexpected missing keys: %v", missing)
	}
	if !reflect.DeepEqual(extra, []string{"EXTRA", "X2"}) {
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
	if err := os.WriteFile(filepath.Join(dir, "rule.json"), b, 0600); err != nil {
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

func TestLoadRuleSetFromFile_LoadsOptionalRoleNormalization(t *testing.T) {
	dir := t.TempDir()
	rs := RuleSet{
		Delimiter:   []string{"_", "."},
		Header:      []string{"bam", "bam_bai"},
		RowRules:    RowRules{MatchParts: []int{0, 1, 2}},
		ColumnRules: ColumnRules{MatchParts: []int{3, 4}},
		RoleNormalization: map[string]string{
			"bam":     "BAM",
			"bam_bai": "BAI",
		},
	}
	b, err := json.Marshal(rs)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "rule.json"), b, 0600); err != nil {
		t.Fatalf("write rule.json error: %v", err)
	}

	loaded, err := LoadRuleSetFromFile(dir)
	if err != nil {
		t.Fatalf("LoadRuleSetFromFile error: %v", err)
	}

	if !reflect.DeepEqual(loaded.RoleNormalization, rs.RoleNormalization) {
		t.Fatalf("unexpected role normalization map: got=%v want=%v", loaded.RoleNormalization, rs.RoleNormalization)
	}
	normalized, found := NormalizeRoleKey("bam_bai", loaded)
	if !found || normalized != "BAI" {
		t.Fatalf("expected loaded bam_bai normalization to BAI, got=%q found=%v", normalized, found)
	}
}
