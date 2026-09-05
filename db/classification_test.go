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
	if folderExistsInDB(t, db, dirB) {
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
