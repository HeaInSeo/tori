package db

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/HeaInSeo/tori/protoio"
	pb "github.com/HeaInSeo/tori/protos/ichthys/v1"
)

// TDI-I4F acceptance tests T04–T14: frozen classification basis across acceptance,
// crash/reconcile, rule-only drift, firstRun laundering, and legacy migration. These
// live in package db so they can drive the unexported acceptance/basis primitives to
// reproduce crash windows.

// ruleJSONR2 is a semantically DIFFERENT rule than ruleJSON (header order swapped): it
// has a distinct classification-semantics revision AND produces distinct FileBlock
// ColumnHeaders, so "R1 basis was used" is observable in the projection.
const ruleJSONR2 = `{
	"version": "1",
	"delimiter": ["_", "."],
	"header": ["R2", "R1"],
	"rowRules": {"matchParts": [0, 1, 2, 4, 5, 6]},
	"columnRules": {"matchParts": [3]},
	"sizeRules": {"minSize": 0, "maxSize": 1000}
}`

func setRule(t *testing.T, dir, ruleText string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "rule.json"), []byte(ruleText), 0o600); err != nil {
		t.Fatalf("write rule.json: %v", err)
	}
}

func loadBlocks(t *testing.T, root string) map[string]*pb.FileBlock {
	t.Helper()
	dbk, err := protoio.LoadDataBlock(filepath.Join(root, "datablock.pb"))
	if err != nil {
		t.Fatalf("load datablock: %v", err)
	}
	m := make(map[string]*pb.FileBlock, len(dbk.GetBlocks()))
	for _, b := range dbk.GetBlocks() {
		m[b.GetBlockId()] = b
	}
	return m
}

func sameBlocks(a, b map[string]*pb.FileBlock) bool {
	if len(a) != len(b) {
		return false
	}
	for id, x := range a {
		y, ok := b[id]
		if !ok || !proto.Equal(x, y) {
			return false
		}
	}
	return true
}

func acceptedBasis(t *testing.T, ctx context.Context, db interface {
	sqlDBTX
}, acceptedVer int64, folder string) (folderBasis, bool) {
	t.Helper()
	b, ok, err := getSemantics(ctx, db, acceptedVer, folder)
	if err != nil {
		t.Fatalf("getSemantics: %v", err)
	}
	return b, ok
}

// I4F-T04: accept under R1, enter a pending recovery window, mutate on-disk rule to R2,
// reconcile — the recovered projection uses the frozen R1 basis, never implicit R2.
func TestI4F_T04_CrashReconcileStaysOnR1(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	dir := writeRuleFolder(t, root, "f1", pairFiles("A")...)
	db := newAcceptanceDB(t)
	acceptBaseline(t, db, root)

	r1blocks := loadBlocks(t, root)
	acceptedVer, _ := metaGetInt(ctx, db, metaKeyAcceptedVersion)
	r1, ok := acceptedBasis(t, ctx, db, acceptedVer, dir)
	if !ok {
		t.Fatal("expected an accepted R1 basis after baseline")
	}

	// Simulate a crash window: a pending target that pinned the R1 basis, DB already
	// advanced but projection not yet promoted to clean.
	if _, err := beginPendingWithBasis(ctx, db, []folderBasis{r1}); err != nil {
		t.Fatalf("beginPendingWithBasis: %v", err)
	}
	// On-disk rule now drifts to R2.
	setRule(t, dir, ruleJSONR2)

	res, err := SyncFolders(ctx, db, root, nil, acceptanceExclusions)
	if err != nil {
		t.Fatalf("SyncFolders: %v", err)
	}
	// The reconcile rebuilt the projection from the frozen R1 target basis; the on-disk
	// R2 then surfaces as drift (HOLD), never as an adopted acceptance.
	if res.Outcome != OutcomeReclassifyHold {
		t.Fatalf("expected reclassify-hold (R2 on disk), got %v (%s)", res.Outcome, res.Reason)
	}
	if got := loadBlocks(t, root); !sameBlocks(r1blocks, got) {
		t.Fatal("recovered projection differs from R1: on-disk R2 was used instead of the frozen basis")
	}
	newVer, _ := metaGetInt(ctx, db, metaKeyAcceptedVersion)
	if b, ok := acceptedBasis(t, ctx, db, newVer, dir); !ok || b.RevisionID != r1.RevisionID {
		t.Fatalf("accepted basis is not the frozen R1 revision (%v / %s)", ok, r1.RevisionID)
	}
}

// I4F-T05: accepted metadata references a revision whose exact frozen basis cannot be
// recovered → reconcile does not mark clean and does not fall back to the current rule.
func TestI4F_T05_UnavailableFrozenBasisFailsClosed(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeRuleFolder(t, root, "f1", pairFiles("A")...)
	db := newAcceptanceDB(t)
	acceptBaseline(t, db, root)

	// Lose the frozen basis entirely and force a pending recovery.
	if _, err := db.ExecContext(ctx, "DELETE FROM classification_semantics"); err != nil {
		t.Fatalf("wipe basis: %v", err)
	}
	if err := metaSet(ctx, db, metaKeyAcceptanceState, acceptancePending); err != nil {
		t.Fatalf("force pending: %v", err)
	}

	res, err := SyncFolders(ctx, db, root, nil, acceptanceExclusions)
	if err != nil {
		t.Fatalf("SyncFolders: %v", err)
	}
	if res.Outcome != OutcomeReclassifyHold {
		t.Fatalf("expected fail-closed reclassify-hold, got %v (%s)", res.Outcome, res.Reason)
	}
	// Must remain pending (not silently marked clean under a substituted rule).
	if state, sErr := readAcceptanceState(ctx, db); sErr != nil || state != acceptancePending {
		t.Fatalf("expected still-pending after fail-closed HOLD, got %q (err %v)", state, sErr)
	}
}

// I4F-T06: a rule-only edit (no file inventory change) must not read as ordinary
// "unchanged" and must not adopt R2; the accepted R1 state is preserved.
func TestI4F_T06_RuleOnlyDriftIsNotUnchanged(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	dir := writeRuleFolder(t, root, "f1", pairFiles("A")...)
	db := newAcceptanceDB(t)
	acceptBaseline(t, db, root)

	r1blocks := loadBlocks(t, root)
	acceptedVer, _ := metaGetInt(ctx, db, metaKeyAcceptedVersion)
	r1, _ := acceptedBasis(t, ctx, db, acceptedVer, dir)

	setRule(t, dir, ruleJSONR2) // only the rule changes; files unchanged

	res, err := SyncFolders(ctx, db, root, nil, acceptanceExclusions)
	if err != nil {
		t.Fatalf("SyncFolders: %v", err)
	}
	if res.Outcome == OutcomeUnchanged {
		t.Fatal("rule-only drift was reported as ordinary unchanged")
	}
	if res.Outcome != OutcomeReclassifyHold {
		t.Fatalf("expected reclassify-hold, got %v (%s)", res.Outcome, res.Reason)
	}
	if b, _ := acceptedBasis(t, ctx, db, acceptedVer, dir); b.RevisionID != r1.RevisionID {
		t.Fatal("accepted basis was mutated to R2")
	}
	if got := loadBlocks(t, root); !sameBlocks(r1blocks, got) {
		t.Fatal("accepted R1 projection was rewritten under R2")
	}
}

// I4F-T07: for a new folder whose rule is missing/invalid, the authoritative DB target
// does not advance and the previous accepted DB + projection remain intact.
func TestI4F_T07_RulePreflightBeforeDBAdvance(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeRuleFolder(t, root, "a", pairFiles("A")...)
	db := newAcceptanceDB(t)
	acceptBaseline(t, db, root)
	r1blocks := loadBlocks(t, root)

	// Add a new folder with files but NO rule.json (invalid/absent rule basis).
	dirB := filepath.Join(root, "b")
	if err := os.MkdirAll(dirB, 0o750); err != nil {
		t.Fatalf("mkdir b: %v", err)
	}
	for _, f := range pairFiles("B") {
		if err := os.WriteFile(filepath.Join(dirB, f), []byte("x"), 0o600); err != nil {
			t.Fatalf("write %s: %v", f, err)
		}
	}

	res, err := SyncFolders(ctx, db, root, nil, acceptanceExclusions)
	if err != nil {
		t.Fatalf("SyncFolders: %v", err)
	}
	if res.Outcome != OutcomeDegradedHold {
		t.Fatalf("expected rule preflight degraded-hold, got %v (%s)", res.Outcome, res.Reason)
	}
	if folderExistsInDB(t, db, "b") {
		t.Fatal("new folder with a missing rule was advanced into the accepted DB")
	}
	if got := loadBlocks(t, root); !sameBlocks(r1blocks, got) {
		t.Fatal("previous accepted projection was mutated during a preflight HOLD")
	}
}

// I4F-T08: two accepted folders with distinct rule snapshots recover with their own
// pinned semantics; no global-latest rule substitution.
func TestI4F_T08_PerFolderSemanticsRemainExact(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	dirA := writeRuleFolder(t, root, "a", pairFiles("A")...)
	dirB := writeRuleFolder(t, root, "b", pairFiles("B")...)
	setRule(t, dirB, ruleJSONR2) // B uses a distinct rule snapshot

	db := newAcceptanceDB(t)
	acceptBaseline(t, db, root)

	acceptedVer, _ := metaGetInt(ctx, db, metaKeyAcceptedVersion)
	ba, okA := acceptedBasis(t, ctx, db, acceptedVer, dirA)
	bb, okB := acceptedBasis(t, ctx, db, acceptedVer, dirB)
	if !okA || !okB {
		t.Fatalf("expected pinned bases for both folders (a=%v b=%v)", okA, okB)
	}
	if ba.RevisionID == bb.RevisionID {
		t.Fatal("distinct per-folder rules collapsed to one revision")
	}
}

// I4F-T09: a crash after pending+target-basis creation cannot yield a target acceptance
// whose exact rule snapshot is unknown; restart reconstructs from the target-pinned rule.
func TestI4F_T09_PendingTargetPinsRuleBasis(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	dir := writeRuleFolder(t, root, "f1", pairFiles("A")...)
	db := newAcceptanceDB(t)
	acceptBaseline(t, db, root)

	acceptedVer, _ := metaGetInt(ctx, db, metaKeyAcceptedVersion)
	r1, _ := acceptedBasis(t, ctx, db, acceptedVer, dir)

	// Crash window: pending with a pinned target basis, nothing promoted to clean.
	target, err := beginPendingWithBasis(ctx, db, []folderBasis{r1})
	if err != nil {
		t.Fatalf("beginPendingWithBasis: %v", err)
	}
	// The target basis for the pending version must be complete and resolvable.
	if n, cErr := countSemanticsAtVersion(ctx, db, target); cErr != nil || n == 0 {
		t.Fatalf("pending target basis is not durably pinned (n=%d err=%v)", n, cErr)
	}
	if state, _ := readAcceptanceState(ctx, db); state != acceptancePending {
		t.Fatalf("expected pending state, got %q", state)
	}
	// Restart: reconcile reconstructs from the target-pinned rule and converges to clean.
	res, err := SyncFolders(ctx, db, root, nil, acceptanceExclusions)
	if err != nil {
		t.Fatalf("SyncFolders: %v", err)
	}
	if res.Outcome != OutcomeAcceptedUpdate || !res.Reconcile {
		t.Fatalf("expected reconciled accepted-update, got %v reconcile=%v (%s)", res.Outcome, res.Reconcile, res.Reason)
	}
	if state, _ := readAcceptanceState(ctx, db); state != acceptanceClean {
		t.Fatalf("expected clean after reconcile, got %q", state)
	}
}

// I4F-T10: existing I1/I10 acceptance safety retained — baseline accept settles, and an
// unreadable source still HOLDs with the prior snapshot intact.
func TestI4F_T10_ExistingAcceptanceSafetyRetained(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeRuleFolder(t, root, "f1", pairFiles("A")...)
	db := newAcceptanceDB(t)
	acceptBaseline(t, db, root) // accept + settle to unchanged (asserts inside)

	r1blocks := loadBlocks(t, root)
	// Make the root unobservable → degraded HOLD, prior projection retained.
	if err := os.Chmod(root, 0o000); err != nil {
		t.Skipf("cannot chmod root to force observation failure: %v", err)
	}
	res, err := SyncFolders(ctx, db, root, nil, acceptanceExclusions)
	_ = os.Chmod(root, 0o750) //nolint:gosec // G302: restoring the test temp dir's own perms (a directory needs the execute bit); test-controlled path
	if err != nil {
		t.Fatalf("SyncFolders: %v", err)
	}
	if res.Outcome != OutcomeDegradedHold {
		t.Fatalf("expected degraded-hold on unreadable root, got %v (%s)", res.Outcome, res.Reason)
	}
	if got := loadBlocks(t, root); !sameBlocks(r1blocks, got) {
		t.Fatal("prior accepted projection changed during a degraded HOLD")
	}
}

// I4F-T11: Track A compatibility retained — the frozen-basis projection equals the
// legacy disk-rule projection for the same rule.
func TestI4F_T11_TrackACompatibilityRetained(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeRuleFolder(t, root, "f1", pairFiles("A")...)
	db := newAcceptanceDB(t)
	res, err := SyncFolders(ctx, db, root, nil, acceptanceExclusions)
	if err != nil || res.Outcome != OutcomeAcceptedUpdate {
		t.Fatalf("bootstrap accept failed: %v (%v)", err, res.Outcome)
	}
	// Second run settles to unchanged: the frozen basis reproduces the same projection.
	res, err = SyncFolders(ctx, db, root, nil, acceptanceExclusions)
	if err != nil || res.Outcome != OutcomeUnchanged {
		t.Fatalf("expected unchanged settle, got %v (%v)", res.Outcome, err)
	}
}

// I4F-T12: after a clean accepted R1, change only rule.json to R2 AND remove
// datablock.pb. Sync must preserve accepted R1 (rebuild from the accepted basis),
// must not accept R2, and must not silently bump/clean a new version.
func TestI4F_T12_MissingDatablockCannotLaunderR2(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	dir := writeRuleFolder(t, root, "f1", pairFiles("A")...)
	db := newAcceptanceDB(t)
	acceptBaseline(t, db, root)

	r1blocks := loadBlocks(t, root)
	acceptedVer, _ := metaGetInt(ctx, db, metaKeyAcceptedVersion)
	r1, _ := acceptedBasis(t, ctx, db, acceptedVer, dir)

	setRule(t, dir, ruleJSONR2)
	if err := os.Remove(filepath.Join(root, "datablock.pb")); err != nil {
		t.Fatalf("remove datablock: %v", err)
	}

	res, err := SyncFolders(ctx, db, root, nil, acceptanceExclusions)
	if err != nil {
		t.Fatalf("SyncFolders: %v", err)
	}
	if res.Outcome == OutcomeUnchanged {
		t.Fatal("missing datablock + R2 was laundered into a clean unchanged/bump")
	}
	if res.Outcome != OutcomeReclassifyHold {
		t.Fatalf("expected reclassify-hold, got %v (%s)", res.Outcome, res.Reason)
	}
	// R2 not adopted; projection restored to R1 from the accepted frozen basis.
	if b, _ := acceptedBasis(t, ctx, db, acceptedVer, dir); b.RevisionID != r1.RevisionID {
		t.Fatal("accepted basis was changed to R2")
	}
	if got := loadBlocks(t, root); !sameBlocks(r1blocks, got) {
		t.Fatal("restored projection is not the accepted R1 projection")
	}
}

// I4F-T13: crash at the target/accepted basis transition never loses exact target
// semantics: recovery rebuilds from the complete target basis, with no disk-rule
// substitution even when the on-disk rule has drifted.
func TestI4F_T13_TargetAcceptedTransitionCrashAtomic(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	dir := writeRuleFolder(t, root, "f1", pairFiles("A")...)
	db := newAcceptanceDB(t)
	acceptBaseline(t, db, root)

	r1blocks := loadBlocks(t, root)
	acceptedVer, _ := metaGetInt(ctx, db, metaKeyAcceptedVersion)
	r1, _ := acceptedBasis(t, ctx, db, acceptedVer, dir)

	// Crash between pending+target-basis and clean; then a new file arrives AND the
	// on-disk rule drifts to R2 before restart.
	if _, err := beginPendingWithBasis(ctx, db, []folderBasis{r1}); err != nil {
		t.Fatalf("beginPendingWithBasis: %v", err)
	}
	setRule(t, dir, ruleJSONR2)
	// New data also arrives during the crash window; it must not be accepted under the
	// drifted rule.
	if err := os.WriteFile(filepath.Join(dir, "C_S1_L001_R1_001.fastq.gz"), []byte("x"), 0o600); err != nil {
		t.Fatalf("write new file: %v", err)
	}

	res, err := SyncFolders(ctx, db, root, nil, acceptanceExclusions)
	if err != nil {
		t.Fatalf("SyncFolders: %v", err)
	}
	// Recovery uses the pinned target R1 basis for the projection; on-disk R2 is not
	// substituted (it surfaces as drift HOLD after the R1 rebuild).
	if got := loadBlocks(t, root); !sameBlocks(r1blocks, got) {
		t.Fatal("crash recovery substituted the on-disk R2 rule for the pinned target basis")
	}
	if res.Outcome == OutcomeUnchanged {
		t.Fatal("crash recovery silently accepted under drifted rule")
	}
}

// TestI4F_LegacyDatablockDeletedHolds (adversarial regression): a prior accepted
// snapshot (accepted_version >= 1) with no pinned basis whose datablock.pb has been
// deleted must NOT be treated as a bootstrap and silently re-accepted under the current
// on-disk rules — reproduction is unverifiable, so it HOLDs.
func TestI4F_LegacyDatablockDeletedHolds(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	dir := writeRuleFolder(t, root, "f1", pairFiles("A")...)
	db := newAcceptanceDB(t)
	acceptBaseline(t, db, root) // accepted_version == 1, basis pinned, datablock present

	// Simulate a legacy accepted snapshot whose projection was lost: drop the pinned
	// basis and the datablock, keep the accepted DB rows + accepted_version + clean.
	if _, err := db.ExecContext(ctx, "DELETE FROM classification_semantics"); err != nil {
		t.Fatalf("wipe basis: %v", err)
	}
	if err := os.Remove(filepath.Join(root, "datablock.pb")); err != nil {
		t.Fatalf("remove datablock: %v", err)
	}

	res, err := SyncFolders(ctx, db, root, nil, acceptanceExclusions)
	if err != nil {
		t.Fatalf("SyncFolders: %v", err)
	}
	if res.Outcome != OutcomeReclassifyHold {
		t.Fatalf("expected HOLD for legacy snapshot with lost projection, got %v (%s)", res.Outcome, res.Reason)
	}
	acceptedVer, _ := metaGetInt(ctx, db, metaKeyAcceptedVersion)
	if _, ok := acceptedBasis(t, ctx, db, acceptedVer, dir); ok {
		t.Fatal("adopted a basis for a legacy snapshot whose projection could not be verified")
	}
	if !folderExistsInDB(t, db, "f1") {
		t.Fatal("legacy accepted inventory was wiped instead of held")
	}
}

// TestI4F_DriftIgnoresExcludedFolders (adversarial regression): an accepted folder that
// is newly excluded from scope must not wedge the run on a drift HOLD even if its rule
// also changed; it is pruned by the normal diff/accept path.
func TestI4F_DriftIgnoresExcludedFolders(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeRuleFolder(t, root, "a", pairFiles("A")...)
	dirB := writeRuleFolder(t, root, "b", pairFiles("B")...)
	db := newAcceptanceDB(t)
	acceptBaseline(t, db, root) // accept both a and b

	// b's rule drifts AND b is newly excluded from scope.
	setRule(t, dirB, ruleJSONR2)
	res, err := SyncFolders(ctx, db, root, []string{"b"}, acceptanceExclusions)
	if err != nil {
		t.Fatalf("SyncFolders: %v", err)
	}
	if res.Outcome == OutcomeReclassifyHold {
		t.Fatalf("drift on an out-of-scope (excluded) folder wedged the run: %s", res.Reason)
	}
	if folderExistsInDB(t, db, "b") {
		t.Fatal("excluded folder b was not pruned from the accepted DB")
	}
	if !folderExistsInDB(t, db, "a") {
		t.Fatal("in-scope folder a was wrongly dropped")
	}
}

// TestI4F_ExcludedFolderCrashWindowPrunesNotOrphans (adversarial regression, round 2):
// crash after beginPendingWithBasis with an in-scope-only target basis, while a folder
// being excluded is still in the DB, must not promote an incomplete target that leaves
// the excluded folder basis-less. Recovery skips the out-of-scope folder (like a removed
// one), defers the clean mark, and the diff/accept path prunes it — converging without a
// wedge or an orphaned basis-less folder.
func TestI4F_ExcludedFolderCrashWindowPrunesNotOrphans(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	dirA := writeRuleFolder(t, root, "a", pairFiles("A")...)
	writeRuleFolder(t, root, "b", pairFiles("B")...)
	db := newAcceptanceDB(t)
	acceptBaseline(t, db, root)

	acceptedVer, _ := metaGetInt(ctx, db, metaKeyAcceptedVersion)
	aBasis, ok := acceptedBasis(t, ctx, db, acceptedVer, dirA)
	if !ok {
		t.Fatal("expected a's accepted basis after baseline")
	}
	// Crash window: pending target pins only the in-scope folder (a); b is being excluded
	// but still sits in the DB.
	if _, err := beginPendingWithBasis(ctx, db, []folderBasis{aBasis}); err != nil {
		t.Fatalf("beginPendingWithBasis: %v", err)
	}

	res, err := SyncFolders(ctx, db, root, []string{"b"}, acceptanceExclusions)
	if err != nil {
		t.Fatalf("SyncFolders: %v", err)
	}
	if res.Outcome == OutcomeReclassifyHold {
		t.Fatalf("excluded folder in crash window wedged recovery: %s", res.Reason)
	}
	if folderExistsInDB(t, db, "b") {
		t.Fatal("excluded folder b was left in the accepted DB (orphaned)")
	}
	if !folderExistsInDB(t, db, "a") {
		t.Fatal("in-scope folder a was dropped during recovery")
	}
	newVer, _ := metaGetInt(ctx, db, metaKeyAcceptedVersion)
	if _, ok := acceptedBasis(t, ctx, db, newVer, dirA); !ok {
		t.Fatal("a has no basis at the promoted accepted version")
	}
	if state, _ := readAcceptanceState(ctx, db); state != acceptanceClean {
		t.Fatalf("expected clean convergence, got %q", state)
	}
}

// TestI4F_MigrationIgnoresExcludedFolder (adversarial regression, round 2): legacy
// migration must not HOLD because an OUT-OF-SCOPE (excluded) folder's rule changed; it
// reproduces/adopts only the in-scope folders and lets the diff prune the excluded one.
func TestI4F_MigrationIgnoresExcludedFolder(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeRuleFolder(t, root, "a", pairFiles("A")...)
	dirB := writeRuleFolder(t, root, "b", pairFiles("B")...)
	db := newAcceptanceDB(t)
	acceptBaseline(t, db, root)

	// Legacy: drop the pinned basis (datablock + rows remain).
	if _, err := db.ExecContext(ctx, "DELETE FROM classification_semantics"); err != nil {
		t.Fatalf("wipe basis: %v", err)
	}
	// b (to be excluded) drifts; a stays R1.
	setRule(t, dirB, ruleJSONR2)

	res, err := SyncFolders(ctx, db, root, []string{"b"}, acceptanceExclusions)
	if err != nil {
		t.Fatalf("SyncFolders: %v", err)
	}
	if res.Outcome == OutcomeReclassifyHold {
		t.Fatalf("legacy migration wrongly HELD on an out-of-scope drifted folder: %s", res.Reason)
	}
	if folderExistsInDB(t, db, "b") {
		t.Fatal("excluded folder b was not pruned")
	}
	if !folderExistsInDB(t, db, "a") {
		t.Fatal("in-scope folder a was dropped")
	}
}

// TestI4F_InScopeBasisLessFolderHolds (adversarial regression, round 4): an in-scope
// accepted folder that has no pinned accepted basis (e.g. a prior scoped migration/accept
// left it basis-less and it is now back in scope) must fail closed as a reclassify-hold,
// not be silently skipped by drift detection (which would let a later update pin an
// arbitrary rule for it).
func TestI4F_InScopeBasisLessFolderHolds(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeRuleFolder(t, root, "a", pairFiles("A")...)
	dirB := writeRuleFolder(t, root, "b", pairFiles("B")...)
	db := newAcceptanceDB(t)
	acceptBaseline(t, db, root)

	acceptedVer, _ := metaGetInt(ctx, db, metaKeyAcceptedVersion)
	// Drop ONLY b's pinned basis, leaving b in the DB and in scope (no exclusion).
	if _, err := db.ExecContext(ctx, "DELETE FROM classification_semantics WHERE folder_path = ?", dirB); err != nil {
		t.Fatalf("drop b basis: %v", err)
	}

	res, err := SyncFolders(ctx, db, root, nil, acceptanceExclusions)
	if err != nil {
		t.Fatalf("SyncFolders: %v", err)
	}
	if res.Outcome != OutcomeReclassifyHold {
		t.Fatalf("expected reclassify-hold for an in-scope basis-less folder, got %v (%s)", res.Outcome, res.Reason)
	}
	if _, ok := acceptedBasis(t, ctx, db, acceptedVer, dirB); ok {
		t.Fatal("basis-less folder was silently re-pinned instead of held")
	}
}

// TestI4F_LegacyMigrationAtV0DriftHolds (adversarial regression, round 4/P0): a pre-I1/I10
// snapshot never ran commitClean, so accepted_version stays 0 even though a real accepted
// projection (datablock.pb + rows) exists. migrateLegacyBasisIfNeeded adopts and pins a
// basis at version 0. A subsequent rule.json drift on that folder MUST still be detected
// and HOLD — detectSemanticsDrift must key on "a basis is pinned at acceptedVer", not on
// acceptedVer==0 (which would silently skip drift for exactly these legacy snapshots and
// reinterpret the accepted inventory under the drifted rule).
func TestI4F_LegacyMigrationAtV0DriftHolds(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	dir := writeRuleFolder(t, root, "f1", pairFiles("A")...)
	db := newAcceptanceDB(t)
	acceptBaseline(t, db, root) // v1, basis pinned, datablock present

	// Simulate a pre-I1/I10 legacy snapshot: accepted rows + datablock present, but no
	// pinned basis and accepted_version/target_version never advanced past 0.
	if _, err := db.ExecContext(ctx, "DELETE FROM classification_semantics"); err != nil {
		t.Fatalf("wipe basis: %v", err)
	}
	if err := metaSet(ctx, db, metaKeyAcceptedVersion, "0"); err != nil {
		t.Fatalf("reset accepted_version: %v", err)
	}
	if err := metaSet(ctx, db, metaKeyTargetVersion, "0"); err != nil {
		t.Fatalf("reset target_version: %v", err)
	}

	// First sync: migration adopts current (R1) rules as the basis, pinned at v0.
	res, err := SyncFolders(ctx, db, root, nil, acceptanceExclusions)
	if err != nil {
		t.Fatalf("SyncFolders (adopt): %v", err)
	}
	if res.Outcome == OutcomeReclassifyHold {
		t.Fatalf("legacy migration wrongly HELD despite reproducing projection: %s", res.Reason)
	}
	if _, ok := acceptedBasis(t, ctx, db, 0, dir); !ok {
		t.Fatal("migration did not pin a basis at v0")
	}

	// The on-disk rule now drifts R1->R2. This must be a reclassify-hold even though
	// accepted_version is still 0 (the exact hole the acceptedVer==0 guard would reopen).
	setRule(t, dir, ruleJSONR2)
	res, err = SyncFolders(ctx, db, root, nil, acceptanceExclusions)
	if err != nil {
		t.Fatalf("SyncFolders (drift): %v", err)
	}
	if res.Outcome != OutcomeReclassifyHold {
		t.Fatalf("expected reclassify-hold on rule drift over a v0 migration basis, got %v (%s)", res.Outcome, res.Reason)
	}
}

// TestI4F_UncoveredDiffFolderGuard (adversarial regression, round 3): the freeze→diff
// TOCTOU guard flags an added/modified folder whose basis was not frozen (it appeared
// after the freeze) and exempts removals and covered folders.
func TestI4F_UncoveredDiffFolderGuard(t *testing.T) {
	frozen := map[string]folderBasis{"/src/a": {Path: "/src/a"}}

	if got := uncoveredDiffFolder([]FolderDiff{{ChangeType: "added", Path: "/src/b"}}, nil, frozen); got != "/src/b" {
		t.Fatalf("added folder not in frozen must be flagged, got %q", got)
	}
	if got := uncoveredDiffFolder(nil, []FileChange{{ChangeType: "added", Path: "/src/c"}}, frozen); got != "/src/c" {
		t.Fatalf("file change in an unfrozen folder must be flagged, got %q", got)
	}
	if got := uncoveredDiffFolder(
		[]FolderDiff{{ChangeType: "removed", Path: "/src/gone"}},
		[]FileChange{{ChangeType: "removed", Path: "/src/gone"}}, frozen); got != "" {
		t.Fatalf("removals must be exempt, got %q", got)
	}
	if got := uncoveredDiffFolder(
		[]FolderDiff{{ChangeType: "modified", Path: "/src/a"}},
		[]FileChange{{ChangeType: "added", Path: "/src/a"}}, frozen); got != "" {
		t.Fatalf("covered (frozen) folder must not be flagged, got %q", got)
	}
}

// TestI4F_EmptyRootPendingReconciles (adversarial regression): a crash after
// beginPendingWithBasis during an EMPTY-root acceptance (zero folders → zero basis) must
// reconcile to a clean empty snapshot on restart, not HOLD forever on "zero bases".
func TestI4F_EmptyRootPendingReconciles(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir() // empty source root
	db := newAcceptanceDB(t)

	// Crash window: pending initial acceptance of an empty root pins no basis.
	if _, err := beginPendingWithBasis(ctx, db, nil); err != nil {
		t.Fatalf("beginPendingWithBasis: %v", err)
	}
	if state, _ := readAcceptanceState(ctx, db); state != acceptancePending {
		t.Fatalf("expected pending, got %q", state)
	}

	res, err := SyncFolders(ctx, db, root, nil, acceptanceExclusions)
	if err != nil {
		t.Fatalf("SyncFolders: %v", err)
	}
	if res.Outcome == OutcomeReclassifyHold {
		t.Fatalf("empty-root pending wrongly HELD instead of reconciling: %s", res.Reason)
	}
	if state, _ := readAcceptanceState(ctx, db); state != acceptanceClean {
		t.Fatalf("expected clean after empty reconcile, got %q", state)
	}
	if _, err := os.Stat(filepath.Join(root, "datablock.pb")); err != nil {
		t.Fatalf("empty reconcile did not regenerate a datablock: %v", err)
	}
}

// I4F-T14: a legacy pre-I4F clean snapshot adopts current rules only when they
// reproduce the accepted projection; a mismatch fails closed (HOLD).
func TestI4F_T14_LegacyMigrationEvidence(t *testing.T) {
	t.Run("reproduces-adopts", func(t *testing.T) {
		ctx := context.Background()
		root := t.TempDir()
		dir := writeRuleFolder(t, root, "f1", pairFiles("A")...)
		db := newAcceptanceDB(t)
		acceptBaseline(t, db, root)
		// Simulate a legacy pre-I4F DB: accepted rows + datablock, but no frozen basis.
		if _, err := db.ExecContext(ctx, "DELETE FROM classification_semantics"); err != nil {
			t.Fatalf("wipe basis: %v", err)
		}
		// Disk rule is unchanged (still R1) → reproduces → migration adopts.
		res, err := SyncFolders(ctx, db, root, nil, acceptanceExclusions)
		if err != nil {
			t.Fatalf("SyncFolders: %v", err)
		}
		if res.Outcome == OutcomeReclassifyHold {
			t.Fatalf("legacy migration wrongly HELD despite reproducing projection: %s", res.Reason)
		}
		acceptedVer, _ := metaGetInt(ctx, db, metaKeyAcceptedVersion)
		b, ok := acceptedBasis(t, ctx, db, acceptedVer, dir)
		if !ok || b.Origin != originMigrationAdopted {
			t.Fatalf("expected migration-adopted basis, got ok=%v origin=%q", ok, b.Origin)
		}
	})

	t.Run("mismatch-holds", func(t *testing.T) {
		ctx := context.Background()
		root := t.TempDir()
		dir := writeRuleFolder(t, root, "f1", pairFiles("A")...)
		db := newAcceptanceDB(t)
		acceptBaseline(t, db, root)
		if _, err := db.ExecContext(ctx, "DELETE FROM classification_semantics"); err != nil {
			t.Fatalf("wipe basis: %v", err)
		}
		// Disk rule now differs (R2) so the current rules no longer reproduce the
		// accepted R1 projection → migration must HOLD, not adopt.
		setRule(t, dir, ruleJSONR2)
		res, err := SyncFolders(ctx, db, root, nil, acceptanceExclusions)
		if err != nil {
			t.Fatalf("SyncFolders: %v", err)
		}
		if res.Outcome != OutcomeReclassifyHold {
			t.Fatalf("expected legacy migration HOLD on mismatch, got %v (%s)", res.Outcome, res.Reason)
		}
		acceptedVer, _ := metaGetInt(ctx, db, metaKeyAcceptedVersion)
		if _, ok := acceptedBasis(t, ctx, db, acceptedVer, dir); ok {
			t.Fatal("mismatched legacy migration adopted a basis instead of holding")
		}
	})
}

// I4F-T15 (v0.3 F1): a pre-v0.3 legacy DB (inventory + no acceptance provenance) whose
// projection is missing must HOLD, never bootstrap-adopt the current on-disk rules. This is
// the exact silent-reinterpretation window Option B would have shipped: without durable
// provenance the state is indistinguishable from a fresh seed.
func TestI4F_T15_LegacyV0MissingProjectionHolds(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	dir := writeRuleFolder(t, root, "f1", pairFiles("A")...)
	db := newAcceptanceDB(t)
	acceptBaseline(t, db, root)
	acceptedVer, _ := metaGetInt(ctx, db, metaKeyAcceptedVersion)

	// Simulate a genuine pre-v0.3 legacy DB: keep the accepted rows + accepted_version, but
	// remove the pinned basis, the durable provenance, and the projection (datablock.pb).
	if _, err := db.ExecContext(ctx, "DELETE FROM classification_semantics"); err != nil {
		t.Fatalf("wipe basis: %v", err)
	}
	if _, err := db.ExecContext(ctx, "DELETE FROM snapshot_meta WHERE key = ?", metaKeyAcceptanceProvenance); err != nil {
		t.Fatalf("clear provenance: %v", err)
	}
	if err := os.Remove(filepath.Join(root, "datablock.pb")); err != nil {
		t.Fatalf("remove datablock: %v", err)
	}
	// A drifted disk rule makes the danger concrete, but even an unchanged rule must HOLD.
	setRule(t, dir, ruleJSONR2)

	res, err := SyncFolders(ctx, db, root, nil, acceptanceExclusions)
	if err != nil {
		t.Fatalf("SyncFolders: %v", err)
	}
	if res.Outcome != OutcomeReclassifyHold {
		t.Fatalf("expected reclassify-hold for legacy-v0 + missing projection, got %v (%s)", res.Outcome, res.Reason)
	}
	// It was classified UNKNOWN_LEGACY (fail-closed), never SEED_ONLY, and adopted nothing.
	if p, ok, _ := getProvenance(ctx, db); !ok || p != provenanceUnknownLegacy {
		t.Fatalf("expected provenance UNKNOWN_LEGACY, got ok=%v p=%q", ok, p)
	}
	if _, ok := acceptedBasis(t, ctx, db, acceptedVer, dir); ok {
		t.Fatal("legacy-v0 snapshot adopted a basis instead of holding")
	}
	if !folderExistsInDB(t, db, "f1") {
		t.Fatal("legacy inventory was wiped instead of held")
	}
}

// I4F-T16 (v0.3 F1): a genuinely fresh seed (SaveFolders on an empty DB) records SEED_ONLY
// durably, and its first legitimate acceptance still succeeds and transitions to ACCEPTED.
func TestI4F_T16_FreshSeedRecordsSeedOnlyAndAccepts(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeRuleFolder(t, root, "f1", pairFiles("A")...)
	db := newAcceptanceDB(t)

	if err := SaveFolders(ctx, db, root, nil, acceptanceExclusions); err != nil {
		t.Fatalf("SaveFolders: %v", err)
	}
	if p, ok, _ := getProvenance(ctx, db); !ok || p != provenanceSeedOnly {
		t.Fatalf("fresh seed expected provenance SEED_ONLY, got ok=%v p=%q", ok, p)
	}

	res, err := SyncFolders(ctx, db, root, nil, acceptanceExclusions)
	if err != nil {
		t.Fatalf("SyncFolders: %v", err)
	}
	if res.Outcome != OutcomeAcceptedUpdate {
		t.Fatalf("first acceptance of a fresh seed must succeed, got %v (%s)", res.Outcome, res.Reason)
	}
	if p, ok, _ := getProvenance(ctx, db); !ok || p != provenanceAccepted {
		t.Fatalf("after first accept expected provenance ACCEPTED, got ok=%v p=%q", ok, p)
	}
}

// I4F-T17 (v0.3 F1): a clean acceptance records ACCEPTED atomically with the clean/accepted
// transition, so there is no reachable state where the snapshot is clean+accepted yet still
// marked seed-only.
func TestI4F_T17_CleanAcceptanceRecordsAcceptedAtomically(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeRuleFolder(t, root, "f1", pairFiles("A")...)
	db := newAcceptanceDB(t)
	acceptBaseline(t, db, root)

	state, err := readAcceptanceState(ctx, db)
	if err != nil {
		t.Fatalf("readAcceptanceState: %v", err)
	}
	av, _ := metaGetInt(ctx, db, metaKeyAcceptedVersion)
	p, ok, _ := getProvenance(ctx, db)
	if state != acceptanceClean || av < 1 {
		t.Fatalf("expected clean accepted_version>=1, got state=%q av=%d", state, av)
	}
	if !ok || p != provenanceAccepted {
		t.Fatalf("clean+accepted must imply provenance ACCEPTED (atomic), got ok=%v p=%q", ok, p)
	}
}

// I4F-T18 (v0.3 F2): a scoped pending crash that leaves an in-scope accepted folder with no
// target and no accepted basis must surface a RECOVERABLE reclassification HOLD, not a
// permanent ErrFrozenBasisUnavailable hard-error wedge; a retry stays a recoverable HOLD.
func TestI4F_T18_ScopedPendingBasisLessFolderRecoverableHold(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	dirA := writeRuleFolder(t, root, "a", pairFiles("A")...)
	dirB := writeRuleFolder(t, root, "b", pairFiles("B")...)
	db := newAcceptanceDB(t)
	acceptBaseline(t, db, root)
	acceptedVer, _ := metaGetInt(ctx, db, metaKeyAcceptedVersion)

	// Crash mid-scoped-migration: a pending target that pinned only A; B (still in scope)
	// has neither a target basis nor an accepted basis.
	aBasis, ok := acceptedBasis(t, ctx, db, acceptedVer, dirA)
	if !ok {
		t.Fatal("expected a's accepted basis after baseline")
	}
	if _, err := beginPendingWithBasis(ctx, db, []folderBasis{aBasis}); err != nil {
		t.Fatalf("beginPendingWithBasis: %v", err)
	}
	if _, err := db.ExecContext(ctx, "DELETE FROM classification_semantics WHERE folder_path = ?", dirB); err != nil {
		t.Fatalf("drop b basis: %v", err)
	}

	res, err := SyncFolders(ctx, db, root, nil, acceptanceExclusions)
	if err != nil {
		t.Fatalf("SyncFolders must not hard-error: %v", err)
	}
	if res.Outcome != OutcomeReclassifyHold {
		t.Fatalf("expected recoverable reclassify-hold, got %v (%s)", res.Outcome, res.Reason)
	}
	// No permanent wedge: a retry is still a recoverable HOLD, not a raw error.
	res2, err2 := SyncFolders(ctx, db, root, nil, acceptanceExclusions)
	if err2 != nil {
		t.Fatalf("retry must not hard-error (no permanent wedge): %v", err2)
	}
	if res2.Outcome != OutcomeReclassifyHold {
		t.Fatalf("retry expected recoverable reclassify-hold, got %v (%s)", res2.Outcome, res2.Reason)
	}
}

// I4F-T19 (v0.3 F3): an in-scope accepted folder whose on-disk rule cannot be frozen
// (missing/invalid) AND has no accepted basis must HOLD (unverifiable), not be skipped as
// out-of-scope, and not fall through to ordinary "unchanged" or a raw projection error.
func TestI4F_T19_InScopeUnfreezableRuleNoBasisHolds(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeRuleFolder(t, root, "a", pairFiles("A")...)
	dirB := writeRuleFolder(t, root, "b", pairFiles("B")...)
	db := newAcceptanceDB(t)
	acceptBaseline(t, db, root)

	// B loses its accepted basis AND its on-disk rule becomes unparseable → freeze fails, so
	// B is in scope but absent from `frozen` with no accepted basis to fall back on.
	if _, err := db.ExecContext(ctx, "DELETE FROM classification_semantics WHERE folder_path = ?", dirB); err != nil {
		t.Fatalf("drop b basis: %v", err)
	}
	setRule(t, dirB, "{ this is not valid rule json")

	res, err := SyncFolders(ctx, db, root, nil, acceptanceExclusions)
	if err != nil {
		t.Fatalf("SyncFolders must not raw-error: %v", err)
	}
	if res.Outcome != OutcomeReclassifyHold {
		t.Fatalf("expected reclassify-hold for in-scope unfreezable rule + no basis, got %v (%s)", res.Outcome, res.Reason)
	}
}

// TestI4F_InterruptedSeedIsResumable (adversarial regression, round 6): SaveFolders records
// SEED_ONLY BEFORE the first seed row, so an interrupted seed can only leave
// (provenance=SEED_ONLY, rows, no datablock) — which must remain resumable (bootstrap +
// accept), never wedge into UNKNOWN_LEGACY/HOLD. Before the fix, provenance was written
// AFTER the rows, so a crash mid-seed left (rows, no provenance) which the next sync
// classified UNKNOWN_LEGACY and HELD, unrepairable by a re-seed (wasEmpty then false).
func TestI4F_InterruptedSeedIsResumable(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	dir := writeRuleFolder(t, root, "f1", pairFiles("A")...)
	db := newAcceptanceDB(t)

	// Reproduce the state an interrupted seed now leaves: the SEED_ONLY marker is durable
	// (written first) and folder rows are present, but no projection was produced yet.
	if err := setProvenanceTx(ctx, db, provenanceSeedOnly); err != nil {
		t.Fatalf("seed provenance: %v", err)
	}
	if err := StoreFilesFolderInfo(ctx, db, dir, acceptanceExclusions); err != nil {
		t.Fatalf("seed rows: %v", err)
	}

	res, err := SyncFolders(ctx, db, root, nil, acceptanceExclusions)
	if err != nil {
		t.Fatalf("SyncFolders: %v", err)
	}
	if res.Outcome != OutcomeAcceptedUpdate {
		t.Fatalf("interrupted seed must resume and accept, got %v (%s)", res.Outcome, res.Reason)
	}
	if p, ok, _ := getProvenance(ctx, db); !ok || p != provenanceAccepted {
		t.Fatalf("expected ACCEPTED after resume, got ok=%v p=%q", ok, p)
	}
}

// TestI4F_AcceptWorkIncompleteLeavesPending (adversarial regression, round 7): if an
// accepted folder vanishes between diff observation and the projection rebuild, acceptWork
// must report complete=false and leave the snapshot PENDING (not promote to clean). Before
// the fix acceptWork returned a bare nil error, so SyncFolders reported an accepted update
// for an inconsistent, still-pending snapshot; it now propagates the incomplete signal so
// the caller HOLDs.
func TestI4F_AcceptWorkIncompleteLeavesPending(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	dirA := writeRuleFolder(t, root, "a", pairFiles("A")...)
	db := newAcceptanceDB(t)
	acceptBaseline(t, db, root)
	acceptedVer, _ := metaGetInt(ctx, db, metaKeyAcceptedVersion)
	aBasis, ok := acceptedBasis(t, ctx, db, acceptedVer, dirA)
	if !ok {
		t.Fatal("expected a's accepted basis after baseline")
	}

	// a is still an accepted DB row and in scope, but its directory is gone when acceptWork
	// rebuilds the projection (the vanish-after-diff race).
	if err := os.RemoveAll(dirA); err != nil {
		t.Fatalf("remove dir: %v", err)
	}
	complete, err := acceptWork(ctx, db, root, nil, nil, []folderBasis{aBasis}, map[string]struct{}{dirA: {}})
	if err != nil {
		t.Fatalf("acceptWork: %v", err)
	}
	if complete {
		t.Fatal("acceptWork must report incomplete when an accepted folder vanished before projection")
	}
	if st, _ := readAcceptanceState(ctx, db); st != acceptancePending {
		t.Fatalf("incomplete accept must leave the snapshot pending, got %q", st)
	}
}

// TestI4F_MigrationRejectsPhantomBlock (adversarial regression, round 7): legacy migration
// must reject a stored datablock.pb that contains a block mapping to no accepted DB folder.
// A one-way subset match would declare reproduction successful and adopt current rules,
// leaving the phantom block published indefinitely; exact reproduction requires no extras
// beyond accepted folders (in scope or being pruned).
func TestI4F_MigrationRejectsPhantomBlock(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	dir := writeRuleFolder(t, root, "f1", pairFiles("A")...)
	db := newAcceptanceDB(t)
	acceptBaseline(t, db, root)
	acceptedVer, _ := metaGetInt(ctx, db, metaKeyAcceptedVersion)

	// Legacy DB (no provenance, no pinned basis) whose stored projection has a phantom block.
	if _, err := db.ExecContext(ctx, "DELETE FROM classification_semantics"); err != nil {
		t.Fatalf("wipe basis: %v", err)
	}
	if _, err := db.ExecContext(ctx, "DELETE FROM snapshot_meta WHERE key = ?", metaKeyAcceptanceProvenance); err != nil {
		t.Fatalf("clear provenance: %v", err)
	}
	dbPath := filepath.Join(root, "datablock.pb")
	dbk, err := protoio.LoadDataBlock(dbPath)
	if err != nil {
		t.Fatalf("load datablock: %v", err)
	}
	dbk.Blocks = append(dbk.GetBlocks(), &pb.FileBlock{BlockId: filepath.Join(root, "ghost"), ColumnHeaders: []string{"x"}})
	if err := protoio.SaveMessage(dbPath, dbk, 0o600); err != nil {
		t.Fatalf("save datablock: %v", err)
	}

	res, err := SyncFolders(ctx, db, root, nil, acceptanceExclusions)
	if err != nil {
		t.Fatalf("SyncFolders: %v", err)
	}
	if res.Outcome != OutcomeReclassifyHold {
		t.Fatalf("expected reclassify-hold for a datablock with a phantom block, got %v (%s)", res.Outcome, res.Reason)
	}
	if _, ok := acceptedBasis(t, ctx, db, acceptedVer, dir); ok {
		t.Fatal("migration adopted a basis despite an unreproducible (phantom-block) projection")
	}
}

// TestI4F_MigrationRejectsDuplicateBlockID (adversarial regression, round 8): a legacy
// datablock.pb containing two blocks with the same accepted block id must fail reproduction
// (the map would otherwise collapse them, letting the last matching one pass while a stale
// duplicate stays published). Migration must HOLD, not adopt.
func TestI4F_MigrationRejectsDuplicateBlockID(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	dir := writeRuleFolder(t, root, "f1", pairFiles("A")...)
	db := newAcceptanceDB(t)
	acceptBaseline(t, db, root)
	acceptedVer, _ := metaGetInt(ctx, db, metaKeyAcceptedVersion)

	if _, err := db.ExecContext(ctx, "DELETE FROM classification_semantics"); err != nil {
		t.Fatalf("wipe basis: %v", err)
	}
	if _, err := db.ExecContext(ctx, "DELETE FROM snapshot_meta WHERE key = ?", metaKeyAcceptanceProvenance); err != nil {
		t.Fatalf("clear provenance: %v", err)
	}
	dbPath := filepath.Join(root, "datablock.pb")
	dbk, err := protoio.LoadDataBlock(dbPath)
	if err != nil {
		t.Fatalf("load datablock: %v", err)
	}
	// Duplicate f1's block (same block id) so the stored projection is not 1:1 with the DB.
	blocks := dbk.GetBlocks()
	if len(blocks) == 0 {
		t.Fatal("expected at least one stored block")
	}
	dbk.Blocks = append(dbk.GetBlocks(), blocks[0])
	if err := protoio.SaveMessage(dbPath, dbk, 0o600); err != nil {
		t.Fatalf("save datablock: %v", err)
	}

	res, err := SyncFolders(ctx, db, root, nil, acceptanceExclusions)
	if err != nil {
		t.Fatalf("SyncFolders: %v", err)
	}
	if res.Outcome != OutcomeReclassifyHold {
		t.Fatalf("expected reclassify-hold for a datablock with a duplicate block id, got %v (%s)", res.Outcome, res.Reason)
	}
	if _, ok := acceptedBasis(t, ctx, db, acceptedVer, dir); ok {
		t.Fatal("migration adopted a basis despite a duplicate-block (non-exact) projection")
	}
}
