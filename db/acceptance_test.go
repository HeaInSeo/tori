package db

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// This suite proves the TDI-I1+I10 observation acceptance boundary. Tests live in
// package db so they can drive the unexported acceptance/reconcile primitives
// (beginPending / UpdateDB / reconcileIfPending / metaGet ...) that simulate a
// crash between phases, in addition to the public SyncFolders entry point.

var acceptanceExclusions = []string{"*.json", "invalid_files", "*.csv", "*.pb"}

// ruleJSON is a minimal pair-end rule set. Fixtures below always provide complete
// R1/R2 pairs so no invalid rows (and therefore no tracked side-files) are emitted;
// grouping correctness is not under test here, only that a projection is generated
// from the accepted inventory.
const ruleJSON = `{
	"version": "1",
	"delimiter": ["_", "."],
	"header": ["R1", "R2"],
	"rowRules": {"matchParts": [0, 1, 2, 4, 5, 6]},
	"columnRules": {"matchParts": [3]},
	"sizeRules": {"minSize": 0, "maxSize": 1000}
}`

// pairFiles returns a complete, rule-valid R1/R2 pair for the given sample.
func pairFiles(sample string) []string {
	return []string{
		sample + "_S1_L001_R1_001.fastq.gz",
		sample + "_S1_L001_R2_001.fastq.gz",
	}
}

// writeRuleFolder creates <root>/<name> containing a rule.json plus the given files.
func writeRuleFolder(t *testing.T, root, name string, files ...string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "rule.json"), []byte(ruleJSON), 0o600); err != nil {
		t.Fatalf("write rule.json: %v", err)
	}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o600); err != nil {
			t.Fatalf("write %s: %v", f, err)
		}
	}
	return dir
}

// newAcceptanceDB opens a file-backed sqlite DB in its own temp dir (separate from
// the source root, mirroring production where the DB lives at cwd and the source is
// a mount).
func newAcceptanceDB(t *testing.T) *sql.DB {
	t.Helper()
	dbDir := t.TempDir()
	conn, err := ConnectDB("sqlite3", filepath.Join(dbDir, "file_monitor.db"), true)
	if err != nil {
		t.Fatalf("ConnectDB: %v", err)
	}
	t.Cleanup(func() {
		if cErr := conn.Close(); cErr != nil {
			t.Logf("close db: %v", cErr)
		}
	})
	if err := InitializeDatabase(conn); err != nil {
		t.Fatalf("InitializeDatabase: %v", err)
	}
	return conn
}

// acceptBaseline establishes a fully accepted snapshot: witness recorded + marker
// on disk, DB populated, datablock.pb generated, acceptance clean.
func acceptBaseline(t *testing.T, db *sql.DB, root string) {
	t.Helper()
	ctx := context.Background()
	if err := SaveFolders(ctx, db, root, nil, acceptanceExclusions); err != nil {
		t.Fatalf("SaveFolders: %v", err)
	}
	res, err := SyncFolders(ctx, db, root, nil, acceptanceExclusions)
	if err != nil {
		t.Fatalf("baseline SyncFolders: %v", err)
	}
	if res.Outcome != OutcomeAcceptedUpdate {
		t.Fatalf("baseline expected accepted-update, got %v (%s)", res.Outcome, res.Reason)
	}
	if _, err := os.Stat(filepath.Join(root, "datablock.pb")); err != nil {
		t.Fatalf("baseline datablock missing: %v", err)
	}
	// The boundary must reach a quiescent, fully-accepted state after one accept.
	res, err = SyncFolders(ctx, db, root, nil, acceptanceExclusions)
	if err != nil {
		t.Fatalf("baseline settle SyncFolders: %v", err)
	}
	if res.Outcome != OutcomeUnchanged {
		t.Fatalf("baseline did not settle to unchanged, got %v (%s)", res.Outcome, res.Reason)
	}
}

func readFileBytes(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path) //nolint:gosec // test-only read of a path this test just created
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return b
}

func countFolders(t *testing.T, db *sql.DB) int {
	t.Helper()
	folders, err := GetFoldersFromDB(db)
	if err != nil {
		t.Fatalf("GetFoldersFromDB: %v", err)
	}
	return len(folders)
}

func countFiles(t *testing.T, db *sql.DB) int {
	t.Helper()
	files, err := GetFilesFromDB(db)
	if err != nil {
		t.Fatalf("GetFilesFromDB: %v", err)
	}
	return len(files)
}

func folderExistsInDB(t *testing.T, db *sql.DB, base string) bool {
	t.Helper()
	folders, err := GetFoldersFromDB(db)
	if err != nil {
		t.Fatalf("GetFoldersFromDB: %v", err)
	}
	for _, f := range folders {
		if filepath.Base(f.Path) == base {
			return true
		}
	}
	return false
}

func fileExistsInDB(t *testing.T, db *sql.DB, folderPath, name string) bool {
	t.Helper()
	files, err := GetFilesByPathFromDB(db, folderPath)
	if err != nil {
		t.Fatalf("GetFilesByPathFromDB: %v", err)
	}
	for _, f := range files {
		if f.Name == name {
			return true
		}
	}
	return false
}

func fileSizeInDB(t *testing.T, db *sql.DB, folderPath, name string) int64 {
	t.Helper()
	files, err := GetFilesByPathFromDB(db, folderPath)
	if err != nil {
		t.Fatalf("GetFilesByPathFromDB: %v", err)
	}
	for _, f := range files {
		if f.Name == name {
			return f.Size
		}
	}
	t.Fatalf("file %s not found in DB folder %s", name, folderPath)
	return 0
}

// --- I1: acceptance boundary ---------------------------------------------------

// I1-T01: scope confirmed + COMPLETE + genuine removal → removal accepted.
func TestI1_T01_GenuineRemovalAccepted(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeRuleFolder(t, root, "set_a", pairFiles("sample1")...)
	writeRuleFolder(t, root, "set_b", pairFiles("sample2")...)
	db := newAcceptanceDB(t)
	acceptBaseline(t, db, root)

	if !folderExistsInDB(t, db, "set_b") {
		t.Fatalf("precondition: set_b should be tracked")
	}
	// Genuine removal of a whole folder from the confirmed source.
	if err := os.RemoveAll(filepath.Join(root, "set_b")); err != nil {
		t.Fatalf("remove set_b: %v", err)
	}

	res, err := SyncFolders(ctx, db, root, nil, acceptanceExclusions)
	if err != nil {
		t.Fatalf("SyncFolders: %v", err)
	}
	if res.Outcome != OutcomeAcceptedUpdate {
		t.Fatalf("expected accepted-update, got %v (%s)", res.Outcome, res.Reason)
	}
	if folderExistsInDB(t, db, "set_b") {
		t.Fatalf("genuine removal was not accepted: set_b still in DB")
	}
	if !folderExistsInDB(t, db, "set_a") {
		t.Fatalf("removal over-applied: set_a lost")
	}
}

// I1-T02: observation unavailable/error → previous DB + projection retained.
func TestI1_T02_ObservationErrorRetainsState(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeRuleFolder(t, root, "set_a", pairFiles("sample1")...)
	db := newAcceptanceDB(t)
	acceptBaseline(t, db, root)

	foldersBefore := countFolders(t, db)
	dbBytesBefore := readFileBytes(t, filepath.Join(root, "datablock.pb"))

	// A source root that cannot be observed at all (e.g. detached so the path is gone).
	gone := root + "_gone"
	res, err := SyncFolders(ctx, db, gone, nil, acceptanceExclusions)
	if err != nil {
		t.Fatalf("SyncFolders should HOLD, not error: %v", err)
	}
	if res.Outcome != OutcomeDegradedHold || res.Scope != ScopeUnknown {
		t.Fatalf("expected degraded-hold/UNKNOWN, got %v/%v (%s)", res.Outcome, res.Scope, res.Reason)
	}
	if got := countFolders(t, db); got != foldersBefore {
		t.Fatalf("DB mutated on observation error: folders %d -> %d", foldersBefore, got)
	}
	if got := readFileBytes(t, filepath.Join(root, "datablock.pb")); string(got) != string(dbBytesBefore) {
		t.Fatalf("projection overwritten on observation error")
	}
}

// I1-T03: scope confirmed + PARTIAL → NO mutation and NO projection overwrite.
func TestI1_T03_PartialCoverageHolds(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("cannot simulate an unreadable folder as root")
	}
	ctx := context.Background()
	root := t.TempDir()
	writeRuleFolder(t, root, "set_a", pairFiles("sample1")...)
	db := newAcceptanceDB(t)
	acceptBaseline(t, db, root)

	foldersBefore := countFolders(t, db)
	filesBefore := countFiles(t, db)
	dbBytesBefore := readFileBytes(t, filepath.Join(root, "datablock.pb"))

	// Stage a genuine addition so we can prove it is NOT applied under PARTIAL.
	writeRuleFolder(t, root, "set_c", pairFiles("sample3")...)
	// Make another folder unreadable -> coverage PARTIAL.
	setB := writeRuleFolder(t, root, "set_b", pairFiles("sample2")...)
	if err := os.Chmod(setB, 0o000); err != nil {
		t.Fatalf("chmod set_b: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(setB, 0o700) }) //nolint:gosec // restore dir rwx so t.TempDir cleanup can traverse and remove it

	res, err := SyncFolders(ctx, db, root, nil, acceptanceExclusions)
	if err != nil {
		t.Fatalf("SyncFolders should HOLD, not error: %v", err)
	}
	if res.Outcome != OutcomeDegradedHold || res.Coverage != CoveragePartial || res.Scope != ScopeConfirmed {
		t.Fatalf("expected degraded-hold/CONFIRMED/PARTIAL, got %v/%v/%v (%s)",
			res.Outcome, res.Scope, res.Coverage, res.Reason)
	}
	if got := countFolders(t, db); got != foldersBefore {
		t.Fatalf("DB folders mutated under PARTIAL: %d -> %d", foldersBefore, got)
	}
	if got := countFiles(t, db); got != filesBefore {
		t.Fatalf("DB files mutated under PARTIAL: %d -> %d", filesBefore, got)
	}
	if folderExistsInDB(t, db, "set_c") {
		t.Fatalf("staged addition leaked into inventory under PARTIAL")
	}
	if got := readFileBytes(t, filepath.Join(root, "datablock.pb")); string(got) != string(dbBytesBefore) {
		t.Fatalf("projection overwritten under PARTIAL")
	}
}

// I1-T04: remote/shared readable mountpoint without continuity proof → no
// authoritative mutation even when empty.
func TestI1_T04_EmptyReadableMountHolds(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeRuleFolder(t, root, "set_a", pairFiles("sample1")...)
	db := newAcceptanceDB(t)
	acceptBaseline(t, db, root)

	foldersBefore := countFolders(t, db)
	dbBytesBefore := readFileBytes(t, filepath.Join(root, "datablock.pb"))

	// A different, empty but readable directory (detached mount) with no witness.
	emptyMount := t.TempDir()
	res, err := SyncFolders(ctx, db, emptyMount, nil, acceptanceExclusions)
	if err != nil {
		t.Fatalf("SyncFolders should HOLD, not error: %v", err)
	}
	if res.Outcome != OutcomeDegradedHold || res.Scope != ScopeUnknown {
		t.Fatalf("expected degraded-hold/UNKNOWN, got %v/%v (%s)", res.Outcome, res.Scope, res.Reason)
	}
	if got := countFolders(t, db); got != foldersBefore || foldersBefore == 0 {
		t.Fatalf("accepted inventory wiped by empty mount: folders %d -> %d", foldersBefore, got)
	}
	if got := readFileBytes(t, filepath.Join(root, "datablock.pb")); string(got) != string(dbBytesBefore) {
		t.Fatalf("projection overwritten by empty mount")
	}
}

// I1-T05: as T04 but unrelated local files visible → they do not pollute inventory.
func TestI1_T05_UnrelatedFilesDoNotPollute(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeRuleFolder(t, root, "set_a", pairFiles("sample1")...)
	db := newAcceptanceDB(t)
	acceptBaseline(t, db, root)

	foldersBefore := countFolders(t, db)

	// Wrong mount that is readable and non-empty but carries no witness.
	wrongMount := t.TempDir()
	writeRuleFolder(t, wrongMount, "junk_folder", pairFiles("unrelated")...)

	res, err := SyncFolders(ctx, db, wrongMount, nil, acceptanceExclusions)
	if err != nil {
		t.Fatalf("SyncFolders should HOLD, not error: %v", err)
	}
	if res.Outcome != OutcomeDegradedHold || res.Scope != ScopeUnknown {
		t.Fatalf("expected degraded-hold/UNKNOWN, got %v/%v (%s)", res.Outcome, res.Scope, res.Reason)
	}
	if got := countFolders(t, db); got != foldersBefore {
		t.Fatalf("unrelated files polluted inventory: folders %d -> %d", foldersBefore, got)
	}
	if folderExistsInDB(t, db, "junk_folder") {
		t.Fatalf("junk_folder leaked into accepted inventory")
	}
}

// I1-T06: wrong replacement mount/source (carries a foreign witness token) → no
// authoritative mutation.
func TestI1_T06_WrongReplacementMountHolds(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeRuleFolder(t, root, "set_a", pairFiles("sample1")...)
	db := newAcceptanceDB(t)
	acceptBaseline(t, db, root)

	foldersBefore := countFolders(t, db)

	wrongMount := t.TempDir()
	writeRuleFolder(t, wrongMount, "set_a", pairFiles("sample1")...)
	// A foreign witness token (does not match our recorded token).
	if err := os.WriteFile(filepath.Join(wrongMount, witnessPrefix+"deadbeefdeadbeefdeadbeefdeadbeef"), []byte{}, 0o600); err != nil {
		t.Fatalf("write foreign witness: %v", err)
	}

	res, err := SyncFolders(ctx, db, wrongMount, nil, acceptanceExclusions)
	if err != nil {
		t.Fatalf("SyncFolders should HOLD, not error: %v", err)
	}
	if res.Outcome != OutcomeDegradedHold || res.Scope != ScopeUnknown {
		t.Fatalf("expected degraded-hold/UNKNOWN, got %v/%v (%s)", res.Outcome, res.Scope, res.Reason)
	}
	if got := countFolders(t, db); got != foldersBefore {
		t.Fatalf("wrong mount mutated inventory: folders %d -> %d", foldersBefore, got)
	}
}

// I1-T07: confirmed+complete normal modify → legacy projection updates.
func TestI1_T07_NormalModifyUpdatesProjection(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	setA := writeRuleFolder(t, root, "set_a", pairFiles("sample1")...)
	db := newAcceptanceDB(t)
	acceptBaseline(t, db, root)

	r1 := "sample1_S1_L001_R1_001.fastq.gz"
	sizeBefore := fileSizeInDB(t, db, setA, r1)
	before := readFileBytes(t, filepath.Join(root, "datablock.pb"))

	// Modify an existing tracked file (size change) — the pair stays valid.
	if err := os.WriteFile(filepath.Join(setA, r1), []byte("xxxxxxxx"), 0o600); err != nil {
		t.Fatalf("modify R1: %v", err)
	}

	res, err := SyncFolders(ctx, db, root, nil, acceptanceExclusions)
	if err != nil {
		t.Fatalf("SyncFolders: %v", err)
	}
	if res.Outcome != OutcomeAcceptedUpdate {
		t.Fatalf("expected accepted-update, got %v (%s)", res.Outcome, res.Reason)
	}
	if got := fileSizeInDB(t, db, setA, r1); got == sizeBefore {
		t.Fatalf("modify not accepted into DB (size unchanged at %d)", got)
	}
	if after := readFileBytes(t, filepath.Join(root, "datablock.pb")); string(after) == string(before) {
		t.Fatalf("projection was not updated on modify")
	}
}

// I1-T08: status distinguishes unchanged / accepted-complete-update / degraded-HOLD.
func TestI1_T08_StatusDistinguishesOutcomes(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	setA := writeRuleFolder(t, root, "set_a", pairFiles("sample1")...)
	db := newAcceptanceDB(t)

	// accepted-complete-update (first generation) + settle to unchanged.
	acceptBaseline(t, db, root)

	// unchanged
	res, err := SyncFolders(ctx, db, root, nil, acceptanceExclusions)
	if err != nil {
		t.Fatalf("SyncFolders(unchanged): %v", err)
	}
	if res.Outcome != OutcomeUnchanged {
		t.Fatalf("expected unchanged, got %v", res.Outcome)
	}

	// accepted-complete-update (a real change)
	if err := os.WriteFile(filepath.Join(setA, "sample1_S1_L001_R1_001.fastq.gz"), []byte("xxxx"), 0o600); err != nil {
		t.Fatalf("write change: %v", err)
	}
	res, err = SyncFolders(ctx, db, root, nil, acceptanceExclusions)
	if err != nil {
		t.Fatalf("SyncFolders(update): %v", err)
	}
	if res.Outcome != OutcomeAcceptedUpdate {
		t.Fatalf("expected accepted-update, got %v", res.Outcome)
	}

	// degraded-HOLD
	res, err = SyncFolders(ctx, db, t.TempDir(), nil, acceptanceExclusions)
	if err != nil {
		t.Fatalf("SyncFolders(hold): %v", err)
	}
	if res.Outcome != OutcomeDegradedHold {
		t.Fatalf("expected degraded-hold, got %v", res.Outcome)
	}
}

// --- I10: crash-recoverable acceptance -----------------------------------------

// I10-T01: DB advances then projection generation fails (crash) → restart detects
// incomplete acceptance and rebuilds; stale projection is not silently accepted.
func TestI10_T01_DBAdvancedProjectionStaleReconciles(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	setA := writeRuleFolder(t, root, "set_a", pairFiles("sample1")...)
	db := newAcceptanceDB(t)
	acceptBaseline(t, db, root)

	staleProjection := readFileBytes(t, filepath.Join(root, "datablock.pb"))

	// A genuine change on disk: a whole new valid pair.
	for _, f := range pairFiles("sample2") {
		if err := os.WriteFile(filepath.Join(setA, f), []byte("x"), 0o600); err != nil {
			t.Fatalf("write change: %v", err)
		}
	}
	_, fDiff, fChange, err := DiffFolders(db, root, nil, acceptanceExclusions)
	if err != nil {
		t.Fatalf("DiffFolders: %v", err)
	}

	// Simulate a crash: mark pending + advance the DB, but never write the projection.
	if _, err := beginPending(ctx, db); err != nil {
		t.Fatalf("beginPending: %v", err)
	}
	if err := UpdateDB(ctx, db, fDiff, fChange); err != nil {
		t.Fatalf("UpdateDB: %v", err)
	}

	// Forbidden intermediate state: DB=S2, projection=S1(stale), next diff empty.
	if state, _ := readAcceptanceState(ctx, db); state != acceptancePending {
		t.Fatalf("precondition: expected pending, got %q", state)
	}
	if !fileExistsInDB(t, db, setA, "sample2_S1_L001_R1_001.fastq.gz") {
		t.Fatalf("precondition: DB should have advanced")
	}
	if cur := readFileBytes(t, filepath.Join(root, "datablock.pb")); string(cur) != string(staleProjection) {
		t.Fatalf("precondition: projection should still be stale")
	}

	// Restart / re-entry.
	res, err := SyncFolders(ctx, db, root, nil, acceptanceExclusions)
	if err != nil {
		t.Fatalf("SyncFolders(restart): %v", err)
	}
	if res.Outcome == OutcomeUnchanged {
		t.Fatalf("stale projection was silently accepted as unchanged")
	}
	if res.Outcome != OutcomeAcceptedUpdate || !res.Reconcile {
		t.Fatalf("expected reconciled accepted-update, got %v reconcile=%v", res.Outcome, res.Reconcile)
	}
	if state, _ := readAcceptanceState(ctx, db); state != acceptanceClean {
		t.Fatalf("acceptance not marked clean after reconcile: %q", state)
	}
	if cur := readFileBytes(t, filepath.Join(root, "datablock.pb")); string(cur) == string(staleProjection) {
		t.Fatalf("projection was not rebuilt from the accepted DB")
	}
}

// I10-T02: failure during multi-row DB application → partial mutation cannot become
// an accepted snapshot; next run reconciles safely.
func TestI10_T02_PartialMutationRolledBack(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	setA := writeRuleFolder(t, root, "set_a", pairFiles("sample1")...)
	db := newAcceptanceDB(t)
	acceptBaseline(t, db, root)

	filesBefore := countFiles(t, db)
	folderID := folderIDForTest(t, db, setA)

	// A multi-row batch where a later row fails: the whole tx must roll back so the
	// earlier "added" row cannot survive as a partial, accepted mutation.
	changes := []FileChange{
		{ChangeType: "added", FolderID: folderID, Name: "would_rollback.dat", DiskSize: 1, CreatedTime: "2026-01-01 00:00:00"},
		{ChangeType: "bogus", FolderID: folderID, Name: "boom.dat"},
	}
	if _, err := beginPending(ctx, db); err != nil {
		t.Fatalf("beginPending: %v", err)
	}
	if err := UpdateDB(ctx, db, nil, changes); err == nil {
		t.Fatalf("expected UpdateDB to fail on the bogus row")
	}

	if fileExistsInDB(t, db, setA, "would_rollback.dat") {
		t.Fatalf("partial mutation survived a failed multi-row application")
	}
	if got := countFiles(t, db); got != filesBefore {
		t.Fatalf("file count changed after rolled-back application: %d -> %d", filesBefore, got)
	}

	// Next run reconciles safely: pending marker is honored, projection rebuilt from
	// the (unchanged) accepted DB, and the snapshot converges to clean.
	res, err := SyncFolders(ctx, db, root, nil, acceptanceExclusions)
	if err != nil {
		t.Fatalf("SyncFolders(recover): %v", err)
	}
	if res.Outcome == OutcomeDegradedHold {
		t.Fatalf("unexpected HOLD after rolled-back mutation: %s", res.Reason)
	}
	if state, _ := readAcceptanceState(ctx, db); state != acceptanceClean {
		t.Fatalf("did not converge to clean: %q", state)
	}
}

// I10-T03: projection already correct but the acceptance marker is incomplete →
// reconcile is idempotent, converges, and has no duplicate semantic effects.
func TestI10_T03_ReconcileIdempotentWhenProjectionCorrect(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeRuleFolder(t, root, "set_a", pairFiles("sample1")...)
	db := newAcceptanceDB(t)
	acceptBaseline(t, db, root)

	foldersBefore := countFolders(t, db)
	filesBefore := countFiles(t, db)

	// Simulate a crash that left only the marker set, though the projection already
	// matches the accepted DB.
	if err := metaSet(ctx, db, metaKeyAcceptanceState, acceptancePending); err != nil {
		t.Fatalf("metaSet pending: %v", err)
	}

	res, err := SyncFolders(ctx, db, root, nil, acceptanceExclusions)
	if err != nil {
		t.Fatalf("SyncFolders(reconcile): %v", err)
	}
	if !res.Reconcile {
		t.Fatalf("expected a reconcile to occur")
	}
	if state, _ := readAcceptanceState(ctx, db); state != acceptanceClean {
		t.Fatalf("reconcile did not converge to clean: %q", state)
	}
	if countFolders(t, db) != foldersBefore || countFiles(t, db) != filesBefore {
		t.Fatalf("reconcile produced duplicate semantic effects (folder/file counts changed)")
	}

	// Idempotent: a subsequent run is a plain unchanged with no further reconcile.
	res2, err := SyncFolders(ctx, db, root, nil, acceptanceExclusions)
	if err != nil {
		t.Fatalf("SyncFolders(second): %v", err)
	}
	if res2.Outcome != OutcomeUnchanged || res2.Reconcile {
		t.Fatalf("expected plain unchanged on second run, got %v reconcile=%v", res2.Outcome, res2.Reconcile)
	}
}

// I10-T04: ordinary no-change after a fully accepted snapshot → unchanged without an
// unnecessary projection rewrite.
func TestI10_T04_NoChangeNoRewrite(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeRuleFolder(t, root, "set_a", pairFiles("sample1")...)
	db := newAcceptanceDB(t)
	acceptBaseline(t, db, root)

	dbPath := filepath.Join(root, "datablock.pb")
	infoBefore, err := os.Stat(dbPath)
	if err != nil {
		t.Fatalf("stat before: %v", err)
	}

	res, err := SyncFolders(ctx, db, root, nil, acceptanceExclusions)
	if err != nil {
		t.Fatalf("SyncFolders: %v", err)
	}
	if res.Outcome != OutcomeUnchanged {
		t.Fatalf("expected unchanged, got %v", res.Outcome)
	}
	infoAfter, err := os.Stat(dbPath)
	if err != nil {
		t.Fatalf("stat after: %v", err)
	}
	if !infoAfter.ModTime().Equal(infoBefore.ModTime()) {
		t.Fatalf("projection was rewritten on a no-change run (mtime changed)")
	}
}

// I10-T05: a DB folder vanishes from disk during the pending window → reconcile
// must still converge (not hard-error and wedge into permanent pending), and the
// removal is then accepted authoritatively.
func TestI10_T05_ReconcileConvergesUnderDiskDrift(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeRuleFolder(t, root, "set_a", pairFiles("sample1")...)
	writeRuleFolder(t, root, "set_b", pairFiles("sample2")...)
	db := newAcceptanceDB(t)
	acceptBaseline(t, db, root)

	// Enter the pending window, then drift: a tracked folder disappears from disk.
	if _, err := beginPending(ctx, db); err != nil {
		t.Fatalf("beginPending: %v", err)
	}
	if err := os.RemoveAll(filepath.Join(root, "set_b")); err != nil {
		t.Fatalf("remove set_b: %v", err)
	}

	res, err := SyncFolders(ctx, db, root, nil, acceptanceExclusions)
	if err != nil {
		t.Fatalf("SyncFolders must converge under drift, got error: %v", err)
	}
	if res.Outcome == OutcomeDegradedHold {
		t.Fatalf("unexpected HOLD under confirmed+complete drift: %s", res.Reason)
	}
	if state, _ := readAcceptanceState(ctx, db); state != acceptanceClean {
		t.Fatalf("did not converge to clean under drift: %q", state)
	}
	if folderExistsInDB(t, db, "set_b") {
		t.Fatalf("vanished folder not pruned from accepted inventory")
	}
	if !folderExistsInDB(t, db, "set_a") {
		t.Fatalf("drift over-applied: set_a lost")
	}
	// Fully settled: a follow-up run is a plain unchanged.
	res, err = SyncFolders(ctx, db, root, nil, acceptanceExclusions)
	if err != nil {
		t.Fatalf("SyncFolders(settle): %v", err)
	}
	if res.Outcome != OutcomeUnchanged {
		t.Fatalf("expected unchanged after drift converged, got %v", res.Outcome)
	}
}

// TestWitnessBootstrapAdoptsOrphanMarker covers the crash window where a prior
// bootstrap wrote the marker file but crashed before recording the token. Restart
// must ADOPT the orphan marker, not write a second one (which would later read as
// ambiguous and wedge the boundary into a permanent HOLD).
func TestWitnessBootstrapAdoptsOrphanMarker(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeRuleFolder(t, root, "set_a", pairFiles("sample1")...)
	// Simulate the orphan marker from an interrupted bootstrap (no DB record yet).
	orphan := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if err := os.WriteFile(filepath.Join(root, witnessPrefix+orphan), []byte{}, 0o600); err != nil {
		t.Fatalf("write orphan marker: %v", err)
	}
	db := newAcceptanceDB(t)

	if err := SaveFolders(ctx, db, root, nil, acceptanceExclusions); err != nil {
		t.Fatalf("SaveFolders: %v", err)
	}
	res, err := SyncFolders(ctx, db, root, nil, acceptanceExclusions)
	if err != nil {
		t.Fatalf("SyncFolders: %v", err)
	}
	if res.Outcome != OutcomeAcceptedUpdate {
		t.Fatalf("expected accepted-update on bootstrap, got %v (%s)", res.Outcome, res.Reason)
	}

	// Exactly one marker, and it is the adopted orphan token.
	tok, ambiguous, err := readWitnessToken(root)
	if err != nil {
		t.Fatalf("readWitnessToken: %v", err)
	}
	if ambiguous {
		t.Fatalf("a second marker was written; source is now ambiguous")
	}
	if tok != orphan {
		t.Fatalf("orphan marker not adopted: got %q want %q", tok, orphan)
	}

	// Continuity now holds: a subsequent run settles to unchanged (not a HOLD).
	res, err = SyncFolders(ctx, db, root, nil, acceptanceExclusions)
	if err != nil {
		t.Fatalf("SyncFolders(second): %v", err)
	}
	if res.Outcome != OutcomeUnchanged {
		t.Fatalf("expected unchanged after adopted bootstrap, got %v (%s)", res.Outcome, res.Reason)
	}
}

// folderIDForTest resolves a folder id from the accepted DB rows.
func folderIDForTest(t *testing.T, db *sql.DB, path string) int64 {
	t.Helper()
	folders, err := GetFoldersFromDB(db)
	if err != nil {
		t.Fatalf("GetFoldersFromDB: %v", err)
	}
	for _, f := range folders {
		if f.Path == path {
			return f.ID
		}
	}
	t.Fatalf("folder %s not found in DB", path)
	return 0
}

// --- I1: legacy DB adoption (no recorded witness) ------------------------------
//
// Regression tests for the bootstrap-conflation defect: "no witness recorded" is
// NOT "nothing accepted to protect". A pre-feature / legacy DB holds accepted
// inventory with recorded=="". Bootstrap must be gated on an EMPTY accepted
// inventory, and a legacy DB may only be adopted when the current source still
// carries every accepted folder — otherwise a readable-but-wrong/empty mount would
// be adopted and wipe the accepted inventory.

// simulateLegacyDB removes the recorded continuity witness and its on-disk marker,
// leaving the accepted inventory in place — i.e. exactly the state of a DB that was
// populated before this feature existed.
func simulateLegacyDB(t *testing.T, db *sql.DB, root string) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(),
		"DELETE FROM snapshot_meta WHERE key = ?", metaKeySourceWitness); err != nil {
		t.Fatalf("clear witness meta: %v", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("readdir root: %v", err)
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), witnessPrefix) {
			if err := os.Remove(filepath.Join(root, e.Name())); err != nil {
				t.Fatalf("remove marker: %v", err)
			}
		}
	}
}

func recordedWitnessForTest(t *testing.T, db *sql.DB) string {
	t.Helper()
	v, _, err := metaGet(context.Background(), db, metaKeySourceWitness)
	if err != nil {
		t.Fatalf("metaGet witness: %v", err)
	}
	return v
}

func acceptanceStateForTest(t *testing.T, db *sql.DB) string {
	t.Helper()
	st, err := readAcceptanceState(context.Background(), db)
	if err != nil {
		t.Fatalf("readAcceptanceState: %v", err)
	}
	return st
}

func markerPresent(t *testing.T, root string) bool {
	t.Helper()
	tok, _, err := readWitnessToken(root)
	if err != nil {
		t.Fatalf("readWitnessToken: %v", err)
	}
	return tok != ""
}

// I1-T09: legacy DB (no recorded witness) + the SAME mount path emptied of its
// accepted content → HOLD, accepted inventory retained (NOT wiped/adopted).
// This is the bootstrap-conflation wipe the fix closes.
func TestI1_T09_LegacyDBNoWitnessWrongMountHolds(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeRuleFolder(t, root, "set_a", pairFiles("sample1")...)
	writeRuleFolder(t, root, "set_b", pairFiles("sample2")...)
	db := newAcceptanceDB(t)
	acceptBaseline(t, db, root)

	foldersBefore := countFolders(t, db)
	filesBefore := countFiles(t, db)
	dbBytesBefore := readFileBytes(t, filepath.Join(root, "datablock.pb"))

	// Legacy state: witness cleared, inventory intact.
	simulateLegacyDB(t, db, root)
	if recordedWitnessForTest(t, db) != "" {
		t.Fatalf("precondition: witness should be cleared")
	}
	// Wrong/empty mount at the SAME path: the accepted folders are gone but the
	// root stays readable (the realistic detached-then-remounted case).
	if err := os.RemoveAll(filepath.Join(root, "set_a")); err != nil {
		t.Fatalf("empty set_a: %v", err)
	}
	if err := os.RemoveAll(filepath.Join(root, "set_b")); err != nil {
		t.Fatalf("empty set_b: %v", err)
	}

	res, err := SyncFolders(ctx, db, root, nil, acceptanceExclusions)
	if err != nil {
		t.Fatalf("SyncFolders should HOLD, not error: %v", err)
	}
	if res.Outcome != OutcomeDegradedHold || res.Scope != ScopeUnknown {
		t.Fatalf("expected degraded-hold/UNKNOWN, got %v/%v (%s)", res.Outcome, res.Scope, res.Reason)
	}
	if got := countFolders(t, db); got != foldersBefore || foldersBefore == 0 {
		t.Fatalf("legacy DB inventory wiped by wrong mount: folders %d -> %d", foldersBefore, got)
	}
	if got := countFiles(t, db); got != filesBefore {
		t.Fatalf("legacy DB files wiped by wrong mount: files %d -> %d", filesBefore, got)
	}
	if got := readFileBytes(t, filepath.Join(root, "datablock.pb")); string(got) != string(dbBytesBefore) {
		t.Fatalf("projection overwritten by wrong mount adopted as bootstrap")
	}
	// No wrong mount may be adopted: the witness must remain unrecorded.
	if recordedWitnessForTest(t, db) != "" {
		t.Fatalf("wrong mount was adopted (witness backfilled) — must not happen")
	}
}

// I1-T10: legacy DB (no recorded witness) + the same, intact source → validated
// adoption: proceeds, backfills the continuity witness, and does not wipe.
func TestI1_T10_LegacyDBNoWitnessMatchingSourceAdopts(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeRuleFolder(t, root, "set_a", pairFiles("sample1")...)
	writeRuleFolder(t, root, "set_b", pairFiles("sample2")...)
	db := newAcceptanceDB(t)
	acceptBaseline(t, db, root)

	foldersBefore := countFolders(t, db)

	simulateLegacyDB(t, db, root)
	if markerPresent(t, root) {
		t.Fatalf("precondition: marker should be removed")
	}

	res, err := SyncFolders(ctx, db, root, nil, acceptanceExclusions)
	if err != nil {
		t.Fatalf("SyncFolders: %v", err)
	}
	if res.Outcome == OutcomeDegradedHold {
		t.Fatalf("matching legacy source should be adopted, got HOLD (%s)", res.Reason)
	}
	if got := countFolders(t, db); got != foldersBefore {
		t.Fatalf("adoption altered inventory: folders %d -> %d", foldersBefore, got)
	}
	// The continuity witness must now be backfilled and durable on disk, so a later
	// wrong mount is protected.
	if recordedWitnessForTest(t, db) == "" {
		t.Fatalf("witness was not backfilled on legacy adoption")
	}
	if !markerPresent(t, root) {
		t.Fatalf("witness marker not materialized on legacy adoption")
	}
}

// I10-T06: removing the last tracked folder drives the accepted inventory to empty.
// The projection must be regenerable from an empty DB (empty DataBlock), so
// acceptance converges to clean instead of wedging in a permanent pending state.
func TestI10_T06_RemoveLastFolderConvergesNotWedged(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeRuleFolder(t, root, "set_only", pairFiles("sample1")...)
	db := newAcceptanceDB(t)
	acceptBaseline(t, db, root)

	if countFolders(t, db) != 1 {
		t.Fatalf("precondition: expected exactly 1 tracked folder")
	}
	// Genuine removal of the only folder from the confirmed source.
	if err := os.RemoveAll(filepath.Join(root, "set_only")); err != nil {
		t.Fatalf("remove set_only: %v", err)
	}

	res, err := SyncFolders(ctx, db, root, nil, acceptanceExclusions)
	if err != nil {
		t.Fatalf("SyncFolders wedged on empty-inventory projection: %v", err)
	}
	if res.Outcome != OutcomeAcceptedUpdate {
		t.Fatalf("expected accepted-update, got %v (%s)", res.Outcome, res.Reason)
	}
	if got := countFolders(t, db); got != 0 {
		t.Fatalf("removal not accepted: %d folders remain", got)
	}
	if st := acceptanceStateForTest(t, db); st != acceptanceClean {
		t.Fatalf("acceptance wedged: state=%q, want clean", st)
	}
	if _, err := os.Stat(filepath.Join(root, "datablock.pb")); err != nil {
		t.Fatalf("empty projection not written: %v", err)
	}

	// Re-entry must converge to a quiescent unchanged state, not re-wedge.
	res, err = SyncFolders(ctx, db, root, nil, acceptanceExclusions)
	if err != nil {
		t.Fatalf("second SyncFolders errored: %v", err)
	}
	if res.Outcome != OutcomeUnchanged {
		t.Fatalf("empty accepted snapshot did not settle to unchanged, got %v (%s)", res.Outcome, res.Reason)
	}
	if st := acceptanceStateForTest(t, db); st != acceptanceClean {
		t.Fatalf("state not clean after settle: %q", st)
	}
}

// I1-T11: legacy DB (no recorded witness) synced against a DIFFERENT, empty root
// while the original accepted paths still exist elsewhere → HOLD, not adopted.
// Guards the containment check in firstMissingAcceptedFolder.
func TestI1_T11_LegacyDBDifferentRootHolds(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeRuleFolder(t, root, "set_a", pairFiles("sample1")...)
	db := newAcceptanceDB(t)
	acceptBaseline(t, db, root)

	foldersBefore := countFolders(t, db)
	simulateLegacyDB(t, db, root)

	// A different, empty but readable root; the original root (and its folders)
	// still exists on disk, so a naive "does the accepted path exist anywhere"
	// check would wrongly adopt this mount.
	otherRoot := t.TempDir()
	res, err := SyncFolders(ctx, db, otherRoot, nil, acceptanceExclusions)
	if err != nil {
		t.Fatalf("SyncFolders should HOLD, not error: %v", err)
	}
	if res.Outcome != OutcomeDegradedHold || res.Scope != ScopeUnknown {
		t.Fatalf("expected degraded-hold/UNKNOWN, got %v/%v (%s)", res.Outcome, res.Scope, res.Reason)
	}
	if got := countFolders(t, db); got != foldersBefore || foldersBefore == 0 {
		t.Fatalf("inventory altered by different-root adoption: %d -> %d", foldersBefore, got)
	}
	if recordedWitnessForTest(t, db) != "" {
		t.Fatalf("different root was adopted (witness backfilled) — must not happen")
	}
}

// --- reconcile/projection correctness (codex P1 fixes) -------------------------

// Reconcile must NOT declare a snapshot clean while the rebuilt projection omits an
// accepted folder that vanished during the pending window. It leaves the state
// pending so the diff/accept path prunes the folder and commits a consistent clean
// projection; reconcile itself must not mutate the accepted DB.
func TestReconcile_IncompleteRebuildDefersClean(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeRuleFolder(t, root, "set_a", pairFiles("s1")...)
	writeRuleFolder(t, root, "set_b", pairFiles("s2")...)
	db := newAcceptanceDB(t)
	acceptBaseline(t, db, root)

	if _, err := beginPending(ctx, db); err != nil {
		t.Fatalf("beginPending: %v", err)
	}
	if err := os.RemoveAll(filepath.Join(root, "set_b")); err != nil {
		t.Fatalf("remove set_b: %v", err)
	}

	reconciled, err := reconcileIfPending(ctx, db, root)
	if err != nil {
		t.Fatalf("reconcileIfPending: %v", err)
	}
	if reconciled {
		t.Fatalf("reconcile reported clean while omitting an accepted folder")
	}
	if st := acceptanceStateForTest(t, db); st != acceptancePending {
		t.Fatalf("reconcile falsely left state %q; want still pending", st)
	}
	if !folderExistsInDB(t, db, "set_b") {
		t.Fatalf("reconcile must not prune DB rows; that is the diff/accept path's job")
	}

	// The full SyncFolders run resolves it authoritatively and converges to clean.
	res, err := SyncFolders(ctx, db, root, nil, acceptanceExclusions)
	if err != nil {
		t.Fatalf("SyncFolders: %v", err)
	}
	if res.Outcome != OutcomeAcceptedUpdate {
		t.Fatalf("want accepted-update, got %v (%s)", res.Outcome, res.Reason)
	}
	if st := acceptanceStateForTest(t, db); st != acceptanceClean {
		t.Fatalf("not clean after resolve: %q", st)
	}
	if folderExistsInDB(t, db, "set_b") {
		t.Fatalf("vanished folder not pruned after resolve")
	}
}

// The projection rebuilt from the accepted DB must be deterministically ordered
// (folders by path, file names ascending) so identical accepted inventory yields an
// identical projection ordering regardless of DB row order or acceptance history.
func TestReconcile_ProjectionOrderingDeterministic(t *testing.T) {
	root := t.TempDir()
	// Names chosen so on-disk/enumeration order differs from sorted order.
	writeRuleFolder(t, root, "zeta", pairFiles("s2")...)
	writeRuleFolder(t, root, "alpha", pairFiles("s1")...)
	db := newAcceptanceDB(t)
	acceptBaseline(t, db, root)

	rows, complete, err := buildFolderFilesFromDB(db)
	if err != nil {
		t.Fatalf("buildFolderFilesFromDB: %v", err)
	}
	if !complete {
		t.Fatalf("expected a complete rebuild")
	}
	if len(rows) != 2 {
		t.Fatalf("want 2 folders, got %d", len(rows))
	}
	if filepath.Base(rows[0][0]) != "alpha" || filepath.Base(rows[1][0]) != "zeta" {
		t.Fatalf("folders not sorted by path: %q then %q", rows[0][0], rows[1][0])
	}
	for _, row := range rows {
		names := row[1:]
		for i := 1; i < len(names); i++ {
			if names[i-1] > names[i] {
				t.Fatalf("file names not sorted in %s: %v", row[0], names)
			}
		}
	}
}
