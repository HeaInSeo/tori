//go:build linux

package db

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// This suite proves TDI-I5P-1 (local POSIX stat evidence + SUSPECT/HOLD comparator). The
// legacy comparator (size + 1-second mtime string) classifies every rewrite below as
// "unchanged"; each case here must instead be modified, SUSPECT/HOLD or fail closed.

func evidenceRows(t *testing.T, db *sql.DB) map[string]string {
	t.Helper()
	ctx := context.Background()
	if err := ensureStatEvidenceTable(ctx, db); err != nil {
		t.Fatalf("ensure evidence table: %v", err)
	}
	rows, err := db.QueryContext(ctx, `SELECT f.path, e.name, e.evidence_class, e.content_proof
		FROM file_stat_evidence e JOIN folders f ON f.id = e.folder_id`)
	if err != nil {
		t.Fatalf("query evidence: %v", err)
	}
	defer func() { _ = rows.Close() }()
	out := make(map[string]string)
	for rows.Next() {
		var path, name, class, proof string
		if err := rows.Scan(&path, &name, &class, &proof); err != nil {
			t.Fatalf("scan evidence: %v", err)
		}
		if proof != contentProofNone {
			t.Fatalf("evidence %s/%s content_proof=%q, want %q", path, name, proof, contentProofNone)
		}
		out[filepath.Join(path, name)] = class
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate evidence: %v", err)
	}
	return out
}

func acceptedVersion(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	v, err := metaGetInt(context.Background(), db, metaKeyAcceptedVersion)
	if err != nil {
		t.Fatalf("accepted_version: %v", err)
	}
	return v
}

func statOf(t *testing.T, path string) (os.FileInfo, *syscall.Stat_t) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		t.Fatalf("no Stat_t for %s", path)
	}
	return info, st
}

// rewritePreservingMtime rewrites path in place with same-size content and restores the
// exact original mtime (ns), retrying until the kernel's ctime actually moved.
func rewritePreservingMtime(t *testing.T, path string, content []byte) {
	t.Helper()
	info, st := statOf(t, path)
	origMtime := info.ModTime()
	origCtime := st.Ctim.Nano()
	for i := 0; i < 100; i++ {
		time.Sleep(20 * time.Millisecond)
		if err := os.WriteFile(path, content, 0o600); err != nil {
			t.Fatalf("rewrite %s: %v", path, err)
		}
		if err := os.Chtimes(path, origMtime, origMtime); err != nil {
			t.Fatalf("chtimes %s: %v", path, err)
		}
		newInfo, newSt := statOf(t, path)
		if newInfo.Size() != info.Size() || !newInfo.ModTime().Equal(origMtime) || newSt.Ino != st.Ino {
			t.Fatalf("precondition: size/mtime/inode must be preserved")
		}
		if newSt.Ctim.Nano() != origCtime {
			return
		}
	}
	t.Skip("ctime did not advance on this filesystem; cannot build a SUSPECT fixture")
}

// I5P-T01: same size, same second (here: identical mtime_ns) rewrite → SUSPECT/HOLD, never
// "unchanged". The legacy comparator returns OutcomeUnchanged for this fixture.
func TestI5P_T01_SameSizeRewriteIsSuspectHold(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	dir := writeRuleFolder(t, root, "set_a", pairFiles("sample1")...)
	db := newAcceptanceDB(t)
	acceptBaseline(t, db, root)

	verBefore := acceptedVersion(t, db)
	dbBytesBefore := readFileBytes(t, filepath.Join(root, "datablock.pb"))
	target := filepath.Join(dir, pairFiles("sample1")[0])
	rewritePreservingMtime(t, target, []byte("y"))

	for run := 0; run < 2; run++ {
		res, err := SyncFolders(ctx, db, root, nil, acceptanceExclusions)
		if err != nil {
			t.Fatalf("SyncFolders: %v", err)
		}
		if res.Outcome == OutcomeUnchanged {
			t.Fatalf("run %d: same-size rewrite reported unchanged", run)
		}
		if res.Outcome != OutcomeDegradedHold || !strings.Contains(res.Reason, "SUSPECT") {
			t.Fatalf("run %d: expected degraded-hold SUSPECT, got %v (%s)", run, res.Outcome, res.Reason)
		}
	}
	if got := acceptedVersion(t, db); got != verBefore {
		t.Fatalf("accepted version advanced under SUSPECT: %d -> %d", verBefore, got)
	}
	if got := readFileBytes(t, filepath.Join(root, "datablock.pb")); string(got) != string(dbBytesBefore) {
		t.Fatalf("projection overwritten under SUSPECT")
	}
}

// I5P-T02: size change, or a sub-second mtime_ns change within the same second → modified.
func TestI5P_T02_SizeOrMtimeNsChangeIsModified(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	dir := writeRuleFolder(t, root, "set_a", pairFiles("sample1")...)
	db := newAcceptanceDB(t)
	acceptBaseline(t, db, root)
	names := pairFiles("sample1")

	// (a) size change (existing behavior).
	if err := os.WriteFile(filepath.Join(dir, names[0]), []byte("xx"), 0o600); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	res, err := SyncFolders(ctx, db, root, nil, acceptanceExclusions)
	if err != nil || res.Outcome != OutcomeAcceptedUpdate {
		t.Fatalf("size change: expected accepted-update, got %v (%s) err=%v", res.Outcome, res.Reason, err)
	}
	if got := fileSizeInDB(t, db, dir, names[0]); got != 2 {
		t.Fatalf("size change not accepted: size=%d", got)
	}

	// (b) same size, same second, different mtime_ns: the 1s string is identical.
	path := filepath.Join(dir, names[1])
	info, _ := statOf(t, path)
	sec := info.ModTime().Truncate(time.Second)
	ns := 100 * time.Millisecond
	if info.ModTime().Sub(sec) < 500*time.Millisecond {
		ns = 700 * time.Millisecond
	}
	newMtime := sec.Add(ns)
	if err := os.Chtimes(path, newMtime, newMtime); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	if newMtime.Format("2006-01-02 15:04:05") != info.ModTime().Format("2006-01-02 15:04:05") {
		t.Fatalf("precondition: legacy 1s mtime string must be unchanged")
	}
	res, err = SyncFolders(ctx, db, root, nil, acceptanceExclusions)
	if err != nil || res.Outcome != OutcomeAcceptedUpdate {
		t.Fatalf("mtime_ns change: expected accepted-update, got %v (%s) err=%v", res.Outcome, res.Reason, err)
	}
	res, err = SyncFolders(ctx, db, root, nil, acceptanceExclusions)
	if err != nil || res.Outcome != OutcomeUnchanged {
		t.Fatalf("settle: expected unchanged, got %v (%s) err=%v", res.Outcome, res.Reason, err)
	}
}

// I5P-T03: inode replaced (rename-into-place, same size and mtime) → modified. It is not
// recorded as completion evidence: the class stays STAT_TUPLE with content_proof NONE.
func TestI5P_T03_InodeReplacedIsModified(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	dir := writeRuleFolder(t, root, "set_a", pairFiles("sample1")...)
	db := newAcceptanceDB(t)
	acceptBaseline(t, db, root)

	target := filepath.Join(dir, pairFiles("sample1")[0])
	info, st := statOf(t, target)
	staged := filepath.Join(root, "staged.tmp") // top-level files are never enumerated
	if err := os.WriteFile(staged, []byte("y"), 0o600); err != nil {
		t.Fatalf("stage: %v", err)
	}
	if err := os.Chtimes(staged, info.ModTime(), info.ModTime()); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	if err := os.Rename(staged, target); err != nil {
		t.Fatalf("rename: %v", err)
	}
	newInfo, newSt := statOf(t, target)
	if newInfo.Size() != info.Size() || !newInfo.ModTime().Equal(info.ModTime()) || newSt.Ino == st.Ino {
		t.Fatalf("precondition: same size+mtime, different inode")
	}

	res, err := SyncFolders(ctx, db, root, nil, acceptanceExclusions)
	if err != nil || res.Outcome != OutcomeAcceptedUpdate {
		t.Fatalf("expected accepted-update, got %v (%s) err=%v", res.Outcome, res.Reason, err)
	}
	if class := evidenceRows(t, db)[target]; class != EvidenceStatTuple.String() {
		t.Fatalf("evidence class=%q, want %s", class, EvidenceStatTuple)
	}
	res, err = SyncFolders(ctx, db, root, nil, acceptanceExclusions)
	if err != nil || res.Outcome != OutcomeUnchanged {
		t.Fatalf("settle: expected unchanged, got %v (%s) err=%v", res.Outcome, res.Reason, err)
	}
}

// I5P-T04: no Sys() tuple (non-POSIX profile) → UNKNOWN, fail closed: no mutation, both for
// an accepted snapshot and for a bootstrap.
func TestI5P_T04_UnknownTupleFailsClosed(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeRuleFolder(t, root, "set_a", pairFiles("sample1")...)
	db := newAcceptanceDB(t)
	acceptBaseline(t, db, root)
	writeRuleFolder(t, root, "set_c", pairFiles("sample3")...)

	orig := statTupleOf
	statTupleOf = func(os.FileInfo) StatTuple { return StatTuple{} }
	t.Cleanup(func() { statTupleOf = orig })

	verBefore := acceptedVersion(t, db)
	res, err := SyncFolders(ctx, db, root, nil, acceptanceExclusions)
	if err != nil {
		t.Fatalf("SyncFolders: %v", err)
	}
	if res.Outcome != OutcomeDegradedHold || !strings.Contains(res.Reason, "UNKNOWN") {
		t.Fatalf("expected degraded-hold UNKNOWN, got %v (%s)", res.Outcome, res.Reason)
	}
	if folderExistsInDB(t, db, "set_c") || acceptedVersion(t, db) != verBefore {
		t.Fatalf("UNKNOWN observation advanced the accepted inventory")
	}

	// Bootstrap on a platform without the tuple is held as well.
	root2 := t.TempDir()
	writeRuleFolder(t, root2, "set_a", pairFiles("sample1")...)
	db2 := newAcceptanceDB(t)
	res, err = SyncFolders(ctx, db2, root2, nil, acceptanceExclusions)
	if err != nil {
		t.Fatalf("bootstrap SyncFolders: %v", err)
	}
	if res.Outcome != OutcomeDegradedHold || countFolders(t, db2) != 0 {
		t.Fatalf("bootstrap: expected degraded-hold with no inventory, got %v (%s)", res.Outcome, res.Reason)
	}

	if got := statTupleFromSys(fakeInfo{}); got.Known {
		t.Fatalf("FileInfo without Stat_t must yield an UNKNOWN tuple")
	}
}

type fakeInfo struct{ os.FileInfo }

func (fakeInfo) Sys() any    { return nil }
func (fakeInfo) Size() int64 { return 1 }

// I5P-T05: first scan after upgrade. Legacy accepted rows have no recorded tuple: the scan
// records HINT_ONLY tuples only and HOLDs accepted-projection advance (a staged addition is
// not accepted, the accepted rows are not invalidated). The next scan compares normally.
func TestI5P_T05_FirstUpgradeScanRecordsHintOnlyAndHolds(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	dir := writeRuleFolder(t, root, "set_a", pairFiles("sample1")...)
	db := newAcceptanceDB(t)
	acceptBaseline(t, db, root)

	// Simulate a pre-I5P DB: accepted rows exist, no evidence recorded.
	if _, err := db.ExecContext(ctx, "DELETE FROM file_stat_evidence"); err != nil {
		t.Fatalf("clear evidence: %v", err)
	}
	filesBefore := countFiles(t, db)
	verBefore := acceptedVersion(t, db)
	dbBytesBefore := readFileBytes(t, filepath.Join(root, "datablock.pb"))
	writeRuleFolder(t, root, "set_c", pairFiles("sample3")...)

	res, err := SyncFolders(ctx, db, root, nil, acceptanceExclusions)
	if err != nil {
		t.Fatalf("SyncFolders: %v", err)
	}
	if res.Outcome != OutcomeDegradedHold || !strings.Contains(res.Reason, "HINT_ONLY") {
		t.Fatalf("expected degraded-hold HINT_ONLY, got %v (%s)", res.Outcome, res.Reason)
	}
	ev := evidenceRows(t, db)
	if len(ev) != filesBefore {
		t.Fatalf("HINT_ONLY rows=%d, want one per legacy accepted file (%d)", len(ev), filesBefore)
	}
	for _, name := range pairFiles("sample1") {
		if class := ev[filepath.Join(dir, name)]; class != EvidenceHintOnly.String() {
			t.Fatalf("%s class=%q, want HINT_ONLY", name, class)
		}
	}
	if countFiles(t, db) != filesBefore || folderExistsInDB(t, db, "set_c") || acceptedVersion(t, db) != verBefore {
		t.Fatalf("first upgrade scan advanced the accepted inventory")
	}
	if got := readFileBytes(t, filepath.Join(root, "datablock.pb")); string(got) != string(dbBytesBefore) {
		t.Fatalf("projection overwritten on first upgrade scan")
	}

	res, err = SyncFolders(ctx, db, root, nil, acceptanceExclusions)
	if err != nil || res.Outcome != OutcomeAcceptedUpdate || !folderExistsInDB(t, db, "set_c") {
		t.Fatalf("second scan: expected accepted-update adding set_c, got %v (%s) err=%v", res.Outcome, res.Reason, err)
	}
	if class := evidenceRows(t, db)[filepath.Join(dir, pairFiles("sample1")[0])]; class != EvidenceHintOnly.String() {
		t.Fatalf("unchanged legacy row must keep HINT_ONLY, got %q", class)
	}
	res, err = SyncFolders(ctx, db, root, nil, acceptanceExclusions)
	if err != nil || res.Outcome != OutcomeUnchanged {
		t.Fatalf("settle: expected unchanged, got %v (%s) err=%v", res.Outcome, res.Reason, err)
	}
}

// I5P-T06: PARTIAL coverage → zero advance, including no evidence write.
func TestI5P_T06_PartialCoverageZeroAdvance(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("cannot simulate an unreadable folder as root")
	}
	ctx := context.Background()
	root := t.TempDir()
	writeRuleFolder(t, root, "set_a", pairFiles("sample1")...)
	db := newAcceptanceDB(t)
	acceptBaseline(t, db, root)
	if _, err := db.ExecContext(ctx, "DELETE FROM file_stat_evidence"); err != nil {
		t.Fatalf("clear evidence: %v", err)
	}
	setB := writeRuleFolder(t, root, "set_b", pairFiles("sample2")...)
	if err := os.Chmod(setB, 0o000); err != nil {
		t.Fatalf("chmod set_b: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(setB, 0o700) }) //nolint:gosec // restore dir rwx so t.TempDir cleanup can traverse and remove it

	res, err := SyncFolders(ctx, db, root, nil, acceptanceExclusions)
	if err != nil {
		t.Fatalf("SyncFolders: %v", err)
	}
	if res.Outcome != OutcomeDegradedHold || res.Coverage != CoveragePartial {
		t.Fatalf("expected degraded-hold/PARTIAL, got %v/%v (%s)", res.Outcome, res.Coverage, res.Reason)
	}
	if n := len(evidenceRows(t, db)); n != 0 {
		t.Fatalf("PARTIAL observation wrote %d evidence row(s)", n)
	}
}

// I5P-T07: tuple equality is metadata only. Every evidence row carries content_proof NONE
// (checked in evidenceRows) and no row claims more than STAT_TUPLE.
func TestI5P_T07_TupleIsNeverContentProof(t *testing.T) {
	root := t.TempDir()
	writeRuleFolder(t, root, "set_a", pairFiles("sample1")...)
	writeRuleFolder(t, root, "set_b", pairFiles("sample2")...)
	db := newAcceptanceDB(t)
	acceptBaseline(t, db, root)
	ev := evidenceRows(t, db)
	if len(ev) != countFiles(t, db) {
		t.Fatalf("evidence rows=%d, files=%d", len(ev), countFiles(t, db))
	}
	for k, class := range ev {
		if class != EvidenceStatTuple.String() {
			t.Fatalf("%s class=%q, want STAT_TUPLE", k, class)
		}
	}
}

// I5P-T08: removals drop evidence with the row (no stale tuple for a re-added file).
func TestI5P_T08_RemovalDropsEvidence(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	dir := writeRuleFolder(t, root, "set_a", pairFiles("sample1")...)
	writeRuleFolder(t, root, "set_b", pairFiles("sample2")...)
	db := newAcceptanceDB(t)
	acceptBaseline(t, db, root)

	if err := os.RemoveAll(filepath.Join(root, "set_b")); err != nil {
		t.Fatalf("remove set_b: %v", err)
	}
	for _, name := range pairFiles("sample1") {
		if err := os.Remove(filepath.Join(dir, name)); err != nil {
			t.Fatalf("remove %s: %v", name, err)
		}
	}
	res, err := SyncFolders(ctx, db, root, nil, acceptanceExclusions)
	if err != nil || res.Outcome != OutcomeAcceptedUpdate {
		t.Fatalf("expected accepted-update, got %v (%s) err=%v", res.Outcome, res.Reason, err)
	}
	if n := len(evidenceRows(t, db)); n != 0 {
		t.Fatalf("stale evidence rows after removal: %d", n)
	}
	var total int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM file_stat_evidence").Scan(&total); err != nil {
		t.Fatalf("count: %v", err)
	}
	if total != 0 {
		t.Fatalf("orphan evidence rows (folder removed): %d", total)
	}
}

// I5P-T09: an incomplete pending reconcile (an accepted folder vanished) takes precedence
// over a stat-evidence HOLD: the projection was already rewritten, so the result must be
// incomplete-pending, not a degraded HOLD claiming the prior snapshot was retained.
func TestI5P_T09_IncompleteReconcileWithSuspectIsIncompletePending(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	dirA := writeRuleFolder(t, root, "a", pairFiles("A")...)
	dirB := writeRuleFolder(t, root, "b", pairFiles("B")...)
	db := newAcceptanceDB(t)
	acceptBaseline(t, db, root)
	acceptedVer := acceptedVersion(t, db)
	aBasis, okA := acceptedBasis(t, ctx, db, acceptedVer, dirA)
	bBasis, okB := acceptedBasis(t, ctx, db, acceptedVer, dirB)
	if !okA || !okB {
		t.Fatal("expected a and b accepted bases after baseline")
	}
	if _, err := beginPendingWithBasis(ctx, db, []folderBasis{aBasis, bBasis}); err != nil {
		t.Fatalf("beginPendingWithBasis: %v", err)
	}
	rewritePreservingMtime(t, filepath.Join(dirA, pairFiles("A")[0]), []byte("y"))
	if err := os.RemoveAll(dirB); err != nil {
		t.Fatalf("remove b dir: %v", err)
	}

	res, err := SyncFolders(ctx, db, root, nil, acceptanceExclusions)
	if err != nil {
		t.Fatalf("SyncFolders: %v", err)
	}
	if res.Outcome != OutcomeIncompletePending || !strings.Contains(res.Reason, "SUSPECT") {
		t.Fatalf("incomplete reconcile + SUSPECT must be incomplete-pending, got %v (%s)", res.Outcome, res.Reason)
	}
}

func TestClassifyExisting(t *testing.T) {
	base := StatTuple{Known: true, Size: 1, MtimeNs: 10, CtimeNs: 20, Inode: 30}
	stored := storedEvidence{Tuple: base}
	with := func(f func(*StatTuple)) StatTuple { s := base; f(&s); return s }
	cases := []struct {
		name      string
		disk      StatTuple
		hasStored bool
		modified  bool
		class     EvidenceClass
	}{
		{"unknown disk", StatTuple{}, true, false, EvidenceUnknown},
		{"legacy row", base, false, false, EvidenceHintOnly},
		{"equal", base, true, false, EvidenceStatTuple},
		{"size", with(func(s *StatTuple) { s.Size = 2 }), true, true, EvidenceStatTuple},
		{"mtime_ns", with(func(s *StatTuple) { s.MtimeNs = 11 }), true, true, EvidenceStatTuple},
		{"inode", with(func(s *StatTuple) { s.Inode = 31 }), true, true, EvidenceStatTuple},
		{"ctime only", with(func(s *StatTuple) { s.CtimeNs = 21 }), true, false, EvidenceSuspect},
	}
	for _, c := range cases {
		m, cl := classifyExisting(c.disk, stored, c.hasStored)
		if m != c.modified || cl != c.class {
			t.Errorf("%s: got (%v,%s), want (%v,%s)", c.name, m, cl, c.modified, c.class)
		}
	}
	if EvidenceClass(99).String() != "UNKNOWN" {
		t.Errorf("unrecognized class must render UNKNOWN")
	}
}
