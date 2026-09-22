package db

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TDI-I12 P1 regression (PR #38, review 4047122864).
//
// A duplicate processed through the normal SyncFolders publication path makes
// GenerateFileBlockWithRuleSet write a timestamped invalid_files_<ts>.txt report into
// the SOURCE folder. The snapshot observer (GetCurrentFolderFileInfo) previously matched
// exclusions by exact name / "*.ext" only, and the shipped defaults carried the exact
// entry "invalid_files" — which predates the timestamped name. The report was therefore
// re-observed as source data on the next pass, so the boundary kept producing updates
// and never settled to unchanged.
//
// The fix teaches the observer the same "prefix*" pattern rules.ListFilesExclude already
// understands, and adds "invalid_files_*" to the default exclusion contract. These tests
// pin BOTH directions: the generated report is excluded, and nothing else is.

// defaultFilesExclusions mirrors the shipped config default (config/config.go,
// config/config.json). Kept literal here so a silent drift in either default breaks
// this test rather than the production convergence path.
var defaultFilesExclusions = []string{"*.json", "invalid_files", "invalid_files_*", "*.csv", "*.pb"}

// conflictPair returns two filenames that collapse onto ONE subject coordinate under
// the pair-end rule set, i.e. a duplicate_role_in_row conflict (same fixture shape as
// rules/conflict_isolation_test.go I12-T01).
func conflictPair(sample string) []string {
	return []string{
		sample + "_S1_L001_R1_001.fastq.gz",
		sample + "__S1_L001_R1_001.fastq.gz",
	}
}

// TestI12_P1_ConflictReportDoesNotReenterObservation drives the REAL SyncFolders
// publication path with a duplicate present and proves the generated conflict report
// does not become source data: the boundary converges to unchanged and the report never
// enters the accepted inventory.
func TestI12_P1_ConflictReportDoesNotReenterObservation(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()

	// One folder holding a conflicted subject AND a healthy sibling pair, so the run
	// produces both an invalid_files report and real accepted inventory.
	files := append(conflictPair("dup"), pairFiles("healthy")...)
	dir := writeRuleFolder(t, root, "mixed", files...)

	conn := newAcceptanceDB(t)
	if err := SaveFolders(ctx, conn, root, nil, defaultFilesExclusions); err != nil {
		t.Fatalf("SaveFolders: %v", err)
	}

	// Pass 1 accepts the inventory and generates the projection, which writes the report.
	res, err := SyncFolders(ctx, conn, root, nil, defaultFilesExclusions)
	if err != nil {
		t.Fatalf("SyncFolders(accept): %v", err)
	}
	if res.Outcome != OutcomeAcceptedUpdate {
		t.Fatalf("pass 1 expected accepted-update, got %v (%s)", res.Outcome, res.Reason)
	}

	reports := invalidReportNames(t, dir)
	if len(reports) == 0 {
		t.Fatalf("fixture did not exercise the defect: no invalid_files_<ts>.txt was generated in %s", dir)
	}

	// Pass 2 is the regression: the report is now on disk in the source folder. Before
	// the fix this returned an update (the report was observed as a new source file).
	res, err = SyncFolders(ctx, conn, root, nil, defaultFilesExclusions)
	if err != nil {
		t.Fatalf("SyncFolders(settle): %v", err)
	}
	if res.Outcome != OutcomeUnchanged {
		t.Fatalf("boundary did not converge with a conflict report present: got %v (%s); reports on disk: %v",
			res.Outcome, res.Reason, reports)
	}

	// Pass 3 proves convergence is stable, not a one-shot absorption of the report.
	res, err = SyncFolders(ctx, conn, root, nil, defaultFilesExclusions)
	if err != nil {
		t.Fatalf("SyncFolders(restable): %v", err)
	}
	if res.Outcome != OutcomeUnchanged {
		t.Fatalf("pass 3 expected unchanged, got %v (%s)", res.Outcome, res.Reason)
	}

	// The report must never have polluted the accepted catalog.
	accepted, err := GetFilesByPathFromDB(conn, dir)
	if err != nil {
		t.Fatalf("GetFilesByPathFromDB: %v", err)
	}
	var acceptedNames []string
	for _, f := range accepted {
		acceptedNames = append(acceptedNames, f.Name)
		if strings.HasPrefix(f.Name, "invalid_files") {
			t.Errorf("generated conflict report %q entered the accepted inventory", f.Name)
		}
	}

	// ...and the healthy sibling pair must still be accepted (the exclusion must not
	// have over-matched real source files).
	for _, want := range pairFiles("healthy") {
		if !containsName(acceptedNames, want) {
			t.Errorf("healthy source file %q missing from accepted inventory %v", want, acceptedNames)
		}
	}
}

// TestI12_P1_ObserverExclusionPatternBoundaries pins the observer's matching semantics
// directly, including the names that must NOT be excluded, so the prefix rule cannot be
// widened into swallowing real source files.
func TestI12_P1_ObserverExclusionPatternBoundaries(t *testing.T) {
	dir := t.TempDir()

	cases := []struct {
		name     string
		excluded bool
		why      string
	}{
		{"invalid_files_20260922120000.txt", true, "generated timestamped conflict report"},
		{"invalid_files_", true, "bare prefix is still a generated-report name"},
		{"invalid_files", true, "legacy exact-match entry"},
		{"rule.json", true, "*.json"},
		{"fileblock.csv", true, "*.csv"},
		{"mixedfiles.pb", true, "*.pb"},
		{"healthy_S1_L001_R1_001.fastq.gz", false, "ordinary source file"},
		{"healthy_S1_L001_R2_001.fastq.gz", false, "ordinary source file"},
		{"invalid_filesreport.txt", false, "prefix is invalid_files_ ; no underscore means not a report"},
		{"sample_invalid_files_1.fastq.gz", false, "prefix must match at the START of the name"},
	}

	for _, c := range cases {
		if err := os.WriteFile(filepath.Join(dir, c.name), []byte("x"), 0o600); err != nil {
			t.Fatalf("write %s: %v", c.name, err)
		}
	}

	_, observed, err := GetCurrentFolderFileInfo(dir, defaultFilesExclusions)
	if err != nil {
		t.Fatalf("GetCurrentFolderFileInfo: %v", err)
	}
	var names []string
	for _, f := range observed {
		names = append(names, f.Name)
	}

	for _, c := range cases {
		got := containsName(names, c.name)
		switch {
		case c.excluded && got:
			t.Errorf("%q should be excluded (%s) but was observed", c.name, c.why)
		case !c.excluded && !got:
			t.Errorf("%q should be observed (%s) but was excluded", c.name, c.why)
		}
	}
}

// invalidReportNames returns the generated invalid_files_<ts>.txt names in dir.
func invalidReportNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir %s: %v", dir, err)
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), "invalid_files_") {
			out = append(out, e.Name())
		}
	}
	return out
}

func containsName(names []string, want string) bool {
	for _, n := range names {
		if n == want {
			return true
		}
	}
	return false
}
