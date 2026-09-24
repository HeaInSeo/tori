package db

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TDI-I2B acceptance suite: acceptance and recovery CONSUME the exact source envelope.
// Every scenario is driven through SyncFolders — the real observation → acceptance →
// crash/reconcile caller — rather than through the basis helpers alone.

// acceptedPinForTest returns the source basis pinned for the accepted snapshot.
func acceptedPinForTest(t *testing.T, db *sql.DB) SnapshotSourceBasis {
	t.Helper()
	b, ok, err := GetAcceptedSourceBasis(context.Background(), db)
	if err != nil {
		t.Fatalf("GetAcceptedSourceBasis: %v", err)
	}
	if !ok {
		t.Fatal("accepted snapshot has no pinned source basis")
	}
	return b
}

func pinAtForTest(t *testing.T, db *sql.DB, version int64) (SnapshotSourceBasis, bool) {
	t.Helper()
	b, ok, err := GetSourceBasisAt(context.Background(), db, version)
	if err != nil {
		t.Fatalf("GetSourceBasisAt(v%d): %v", version, err)
	}
	return b, ok
}

func acceptedVersionForTest(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	v, err := metaGetInt(context.Background(), db, metaKeyAcceptedVersion)
	if err != nil {
		t.Fatalf("accepted_version: %v", err)
	}
	return v
}

func envelopeForTest(t *testing.T, db *sql.DB) SourceEnvelope {
	t.Helper()
	env, ok, err := GetSourceEnvelope(context.Background(), db)
	if err != nil || !ok {
		t.Fatalf("GetSourceEnvelope: ok=%v err=%v", ok, err)
	}
	return env
}

func syncForTest(t *testing.T, db *sql.DB, root string, folderEx, fileEx []string, opts ...SyncOption) SyncResult {
	t.Helper()
	res, err := SyncFolders(context.Background(), db, root, folderEx, fileEx, opts...)
	if err != nil {
		t.Fatalf("SyncFolders: %v", err)
	}
	return res
}

// I2B-T01: an accepted snapshot pins the exact SourceID + SourceRevision + endpoint the
// observation ran under, at the same version as its I4F classification basis.
func TestI2B_AcceptedSnapshotPinsExactSourceRevision(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	conn := newAcceptanceDB(t)
	writeRuleFolder(t, root, "runA", pairFiles("A")...)
	acceptBaseline(t, conn, root)

	env := envelopeForTest(t, conn)
	pin := acceptedPinForTest(t, conn)
	if pin.SourceID != env.SourceID || pin.RevisionID != env.CurrentRevisionID || pin.EndpointID != env.CurrentEndpointID {
		t.Fatalf("accepted pin %+v does not name the envelope it was observed under %+v", pin, env)
	}
	if pin.Origin != sourceBasisNative {
		t.Errorf("pin origin = %q, want %q", pin.Origin, sourceBasisNative)
	}
	if n, err := countSemanticsAtVersion(ctx, conn, pin.Version); err != nil || n == 0 {
		t.Errorf("classification basis at the pinned version v%d: n=%d err=%v (source and classification basis must share the version)",
			pin.Version, n, err)
	}

	// The observation reports the basis it ran under.
	res := syncForTest(t, conn, root, nil, acceptanceExclusions)
	if res.Outcome != OutcomeUnchanged {
		t.Fatalf("settled sync = %s (%s), want unchanged", res.Outcome, res.Reason)
	}
	if !res.Source.sameSource(pin) || res.Source.EndpointID != pin.EndpointID {
		t.Errorf("SyncResult.Source = %+v, want the pinned basis %+v", res.Source, pin)
	}
}

// I2B-T02: the current config moves to R2 without any data change. The R1 snapshot must
// not be silently treated as R2 ("unchanged"): R2 is accepted as a NEW version with its
// own pin, and the R1 version's pin and revision row stay byte-identical.
func TestI2B_ConfigR2DoesNotRewriteAcceptedR1(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	conn := newAcceptanceDB(t)
	writeRuleFolder(t, root, "runA", pairFiles("A")...)
	acceptBaseline(t, conn, root)

	r1 := acceptedPinForTest(t, conn)
	r1Canonical, ok, err := SourceRevisionCanonical(ctx, conn, r1.SourceID, r1.RevisionID)
	if err != nil || !ok {
		t.Fatalf("R1 canonical: ok=%v err=%v", ok, err)
	}

	// R2: a new file-exclusion pattern that matches nothing on disk (pure meaning change).
	r2Ex := append(append([]string{}, acceptanceExclusions...), "*.bam")
	res := syncForTest(t, conn, root, nil, r2Ex)
	if res.Outcome != OutcomeAcceptedUpdate {
		t.Fatalf("R2 sync = %s (%s), want accepted-update (a revision change is not \"unchanged\")", res.Outcome, res.Reason)
	}

	r2 := acceptedPinForTest(t, conn)
	if r2.Version <= r1.Version {
		t.Fatalf("R2 did not become a new version: v%d → v%d", r1.Version, r2.Version)
	}
	if r2.SourceID != r1.SourceID || r2.RevisionID == r1.RevisionID {
		t.Fatalf("R2 pin %+v: want same SourceID %s and a new revision (R1 %s)", r2, r1.SourceID, r1.RevisionID)
	}
	if r2.RevisionID != envelopeForTest(t, conn).CurrentRevisionID {
		t.Errorf("R2 pin revision %s is not the current envelope revision", r2.RevisionID)
	}

	stillR1, ok := pinAtForTest(t, conn, r1.Version)
	if !ok || stillR1 != r1 {
		t.Fatalf("R1 version pin was rewritten: %+v → %+v (ok=%v)", r1, stillR1, ok)
	}
	if got, _, _ := SourceRevisionCanonical(ctx, conn, r1.SourceID, r1.RevisionID); got != r1Canonical {
		t.Fatalf("R1 revision row mutated:\n old: %s\n new: %s", r1Canonical, got)
	}

	// And it settles: the next R2 run is genuinely unchanged.
	if res := syncForTest(t, conn, root, nil, r2Ex); res.Outcome != OutcomeUnchanged {
		t.Fatalf("settle under R2 = %s (%s), want unchanged", res.Outcome, res.Reason)
	}
}

// I2B-T03: a proven endpoint relocation continues the same SourceRevision. The accepted
// version after the move names the same SourceID and revision and the new endpoint; the
// pre-move version keeps the old endpoint.
func TestI2B_ProvenRelocationKeepsSourceRevision(t *testing.T) {
	oldRoot := t.TempDir()
	conn := newAcceptanceDB(t)
	writeRuleFolder(t, oldRoot, "runA", pairFiles("A")...)
	acceptBaseline(t, conn, oldRoot)
	before := acceptedPinForTest(t, conn)

	newRoot := filepath.Join(t.TempDir(), "relocated")
	if err := os.Rename(oldRoot, newRoot); err != nil {
		t.Fatalf("relocate: %v", err)
	}
	res := syncForTest(t, conn, newRoot, nil, acceptanceExclusions)
	if res.Scope != ScopeConfirmed || res.Outcome != OutcomeAcceptedUpdate {
		t.Fatalf("relocated sync = %s scope=%s (%s), want accepted-update under a CONFIRMED witness",
			res.Outcome, res.Scope, res.Reason)
	}

	after := acceptedPinForTest(t, conn)
	if !after.sameSource(before) {
		t.Fatalf("relocation changed the pinned source meaning: %+v → %+v", before, after)
	}
	if after.EndpointID == before.EndpointID {
		t.Fatalf("relocated acceptance still pins the old endpoint %s", before.EndpointID)
	}
	if old, ok := pinAtForTest(t, conn, before.Version); !ok || old != before {
		t.Fatalf("pre-move version pin changed: %+v → %+v", before, old)
	}
}

// I2B-T03b: a credential rotation alone is an endpoint event. It continues the accepted
// SourceRevision and needs no new snapshot version.
func TestI2B_CredentialRotationContinuesAcceptedRevision(t *testing.T) {
	root := t.TempDir()
	conn := newAcceptanceDB(t)
	writeRuleFolder(t, root, "runA", pairFiles("A")...)
	acceptBaseline(t, conn, root)
	before := acceptedPinForTest(t, conn)

	rotated := "cred-ref-rotated"
	res := syncForTest(t, conn, root, nil, acceptanceExclusions, WithAccessCredentialRef(rotated))
	if res.Outcome != OutcomeUnchanged {
		t.Fatalf("rotation-only sync = %s (%s), want unchanged", res.Outcome, res.Reason)
	}
	if after := acceptedPinForTest(t, conn); after != before {
		t.Fatalf("rotation rewrote or re-versioned the accepted pin: %+v → %+v", before, after)
	}
	if env := envelopeForTest(t, conn); env.CurrentEndpointID == before.EndpointID {
		t.Fatalf("rotation was not recorded as an endpoint event")
	}
}

// I2B-T04: a replaced/unknown witness stays a degraded HOLD; the accepted pin and version
// are untouched.
func TestI2B_WitnessMismatchHoldsWithoutTouchingPin(t *testing.T) {
	root := t.TempDir()
	conn := newAcceptanceDB(t)
	writeRuleFolder(t, root, "runA", pairFiles("A")...)
	acceptBaseline(t, conn, root)
	before := acceptedPinForTest(t, conn)

	tok := recordedWitnessForTest(t, conn)
	if err := os.Rename(filepath.Join(root, witnessPrefix+tok), filepath.Join(root, witnessPrefix+"0000replaced")); err != nil {
		t.Fatalf("replace witness: %v", err)
	}
	res := syncForTest(t, conn, root, nil, append(append([]string{}, acceptanceExclusions...), "*.bam"))
	if res.Outcome != OutcomeDegradedHold || res.Scope != ScopeUnknown {
		t.Fatalf("witness mismatch = %s scope=%s, want degraded-hold/UNKNOWN", res.Outcome, res.Scope)
	}
	if after := acceptedPinForTest(t, conn); after != before {
		t.Fatalf("HOLD changed the accepted pin: %+v → %+v", before, after)
	}
	if v := acceptedVersionForTest(t, conn); v != before.Version {
		t.Fatalf("HOLD advanced accepted_version v%d → v%d", before.Version, v)
	}
}

// I2B-T05: crash/reconcile preserves BOTH the source revision and the classification
// basis. An acceptance under R1 crashes after publishing but before the clean mark; the
// process restarts with config R2 that excludes the folder the R1 acceptance added.
// Recovery must complete the R1 acceptance under R1's scope (runB included, R1 pin, R1
// classification basis) and only then accept R2 as the next version.
func TestI2B_CrashReconcileUsesPinnedRevisionScope(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	conn := newAcceptanceDB(t)
	writeRuleFolder(t, root, "runA", pairFiles("A")...)
	acceptBaseline(t, conn, root)
	r1 := acceptedPinForTest(t, conn)

	dirB := writeRuleFolder(t, root, "runB", pairFiles("B")...)
	crashAfterPublishForTest = func() bool { return true }
	t.Cleanup(func() { crashAfterPublishForTest = nil })
	res := syncForTest(t, conn, root, nil, acceptanceExclusions)
	crashAfterPublishForTest = nil
	if res.Outcome != OutcomeIncompletePending || acceptanceStateForTest(t, conn) != acceptancePending {
		t.Fatalf("crash window = %s state=%s, want incomplete-pending", res.Outcome, acceptanceStateForTest(t, conn))
	}
	targetVer, err := metaGetInt(ctx, conn, metaKeyTargetVersion)
	if err != nil {
		t.Fatalf("target_version: %v", err)
	}
	if tp, ok := pinAtForTest(t, conn, targetVer); !ok || !tp.sameSource(r1) {
		t.Fatalf("pending target v%d source pin = %+v ok=%v, want R1 %+v (pinned with the pending transition)", targetVer, tp, ok, r1)
	}

	// Restart under R2: runB is now excluded by config.
	res = syncForTest(t, conn, root, []string{"runB"}, acceptanceExclusions)
	if res.Outcome != OutcomeAcceptedUpdate || !res.Reconcile {
		t.Fatalf("restart under R2 = %s reconcile=%v (%s), want accepted-update with reconcile", res.Outcome, res.Reconcile, res.Reason)
	}

	// The recovered version is the R1 acceptance, complete under R1's scope.
	recovered, ok := pinAtForTest(t, conn, targetVer)
	if !ok || !recovered.sameSource(r1) || recovered.Origin != sourceBasisNative {
		t.Fatalf("recovered v%d pin = %+v ok=%v, want native R1", targetVer, recovered, ok)
	}
	if _, ok, err := getSemantics(ctx, conn, targetVer, dirB); err != nil || !ok {
		t.Fatalf("recovered v%d lost runB's classification basis: ok=%v err=%v", targetVer, ok, err)
	}

	// R2 is then accepted as the next version; runB leaves the inventory only there.
	r2 := acceptedPinForTest(t, conn)
	if r2.Version != targetVer+1 || r2.RevisionID == r1.RevisionID {
		t.Fatalf("R2 pin = %+v, want v%d under a new revision", r2, targetVer+1)
	}
	if folderExistsInDB(t, conn, "runB") {
		t.Errorf("runB still accepted after the R2 acceptance")
	}
	if stillR1, _ := pinAtForTest(t, conn, targetVer); stillR1 != recovered {
		t.Errorf("R2 acceptance rewrote the recovered R1 pin: %+v → %+v", recovered, stillR1)
	}
}

// I2B-T06: ACK loss — the acceptance committed but the caller never saw the result. The
// retry must converge to the same accepted version and pin, minting nothing new.
func TestI2B_AckLossRetryConvergesOnSamePin(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	conn := newAcceptanceDB(t)
	dir := writeRuleFolder(t, root, "runA", pairFiles("A")...)
	acceptBaseline(t, conn, root)

	if err := os.WriteFile(filepath.Join(dir, "A_S1_L002_R1_001.fastq.gz"), []byte("x"), 0o600); err != nil {
		t.Fatalf("add file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "A_S1_L002_R2_001.fastq.gz"), []byte("x"), 0o600); err != nil {
		t.Fatalf("add file: %v", err)
	}
	_ = syncForTest(t, conn, root, nil, acceptanceExclusions) // result "lost"
	committed := acceptedPinForTest(t, conn)

	res := syncForTest(t, conn, root, nil, acceptanceExclusions)
	if res.Outcome != OutcomeUnchanged {
		t.Fatalf("retry after lost ACK = %s (%s), want unchanged", res.Outcome, res.Reason)
	}
	if again := acceptedPinForTest(t, conn); again != committed {
		t.Fatalf("retry re-pinned: %+v → %+v", committed, again)
	}
	if n, _ := CountSourceRevisions(ctx, conn, committed.SourceID); n != 1 {
		t.Errorf("revision rows = %d, want 1", n)
	}
	if n, _ := CountSourceEndpoints(ctx, conn, committed.SourceID); n != 1 {
		t.Errorf("endpoint rows = %d, want 1", n)
	}
}

// I2B-T07: a pin whose authority cannot be established in the current envelope HOLDs —
// another SourceID (alias/union), or a revision the source never recorded.
func TestI2B_UnresolvedPinHolds(t *testing.T) {
	cases := map[string]string{
		"foreign source id":     "UPDATE snapshot_source_basis SET source_id = 'src-someoneelse'",
		"unrecorded revision":   "UPDATE snapshot_source_basis SET revision_id = 'deadbeef'",
		"unrecorded endpoint":   "UPDATE snapshot_source_basis SET endpoint_id = 'deadbeef'",
		"foreign pending target": "",
	}
	for name, tamper := range cases {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			conn := newAcceptanceDB(t)
			writeRuleFolder(t, root, "runA", pairFiles("A")...)
			acceptBaseline(t, conn, root)
			before := acceptedVersionForTest(t, conn)
			datablock := readFileBytes(t, filepath.Join(root, "datablock.pb"))

			if tamper == "" {
				// A pending target whose pin names another source.
				pin := acceptedPinForTest(t, conn)
				pin.SourceID = "src-someoneelse"
				if _, err := beginPendingWithBasis(ctx, conn, nil, pin); err != nil {
					t.Fatalf("beginPendingWithBasis: %v", err)
				}
			} else if _, err := conn.ExecContext(ctx, tamper); err != nil {
				t.Fatalf("tamper: %v", err)
			}

			res := syncForTest(t, conn, root, nil, acceptanceExclusions)
			if res.Outcome != OutcomeDegradedHold || !strings.Contains(res.Reason, "UNRESOLVED") {
				t.Fatalf("outcome = %s (%s), want degraded-hold on an unresolved source basis", res.Outcome, res.Reason)
			}
			if v := acceptedVersionForTest(t, conn); v != before {
				t.Errorf("HOLD advanced accepted_version v%d → v%d", before, v)
			}
			if got := readFileBytes(t, filepath.Join(root, "datablock.pb")); string(got) != string(datablock) {
				t.Errorf("HOLD rewrote the accepted projection")
			}
		})
	}
}

// I2B-T08: a pinned version is never rewritten, even by a direct re-pin.
func TestI2B_PinIsImmutable(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	conn := newAcceptanceDB(t)
	writeRuleFolder(t, root, "runA", pairFiles("A")...)
	acceptBaseline(t, conn, root)
	pin := acceptedPinForTest(t, conn)

	other := pin
	other.RevisionID = strings.Repeat("0", 64)
	if err := pinSourceBasisTx(ctx, conn, pin.Version, other); err != nil {
		t.Fatalf("re-pin: %v", err)
	}
	if got := acceptedPinForTest(t, conn); got != pin {
		t.Fatalf("pin rewritten in place: %+v → %+v", pin, got)
	}
}

// I2B-T09: a pre-I2B accepted snapshot (no source pin). With no data diff the current
// revision provably reproduces it and is adopted as legacy-adopted. With a data diff it is
// left unpinned — nothing is claimed about it — and the new acceptance pins natively.
func TestI2B_LegacySnapshotAdoption(t *testing.T) {
	ctx := context.Background()
	t.Run("reproduced → adopted", func(t *testing.T) {
		root := t.TempDir()
		conn := newAcceptanceDB(t)
		writeRuleFolder(t, root, "runA", pairFiles("A")...)
		acceptBaseline(t, conn, root)
		v := acceptedVersionForTest(t, conn)
		if _, err := conn.ExecContext(ctx, "DELETE FROM snapshot_source_basis"); err != nil {
			t.Fatalf("simulate pre-I2B: %v", err)
		}

		res := syncForTest(t, conn, root, nil, acceptanceExclusions)
		if res.Outcome != OutcomeUnchanged {
			t.Fatalf("legacy settle = %s (%s), want unchanged", res.Outcome, res.Reason)
		}
		pin := acceptedPinForTest(t, conn)
		if pin.Version != v || pin.Origin != sourceBasisLegacyAdopted || pin.RevisionID != envelopeForTest(t, conn).CurrentRevisionID {
			t.Fatalf("legacy adoption pin = %+v, want legacy-adopted current revision at v%d", pin, v)
		}
	})
	t.Run("not reproduced → left unpinned", func(t *testing.T) {
		root := t.TempDir()
		conn := newAcceptanceDB(t)
		writeRuleFolder(t, root, "runA", pairFiles("A")...)
		acceptBaseline(t, conn, root)
		v := acceptedVersionForTest(t, conn)
		if _, err := conn.ExecContext(ctx, "DELETE FROM snapshot_source_basis"); err != nil {
			t.Fatalf("simulate pre-I2B: %v", err)
		}
		writeRuleFolder(t, root, "runB", pairFiles("B")...)

		res := syncForTest(t, conn, root, nil, acceptanceExclusions)
		if res.Outcome != OutcomeAcceptedUpdate {
			t.Fatalf("legacy + data change = %s (%s), want accepted-update", res.Outcome, res.Reason)
		}
		if _, ok := pinAtForTest(t, conn, v); ok {
			t.Errorf("pre-I2B version v%d was retroactively pinned despite a data change", v)
		}
		if pin := acceptedPinForTest(t, conn); pin.Version != v+1 || pin.Origin != sourceBasisNative {
			t.Errorf("new acceptance pin = %+v, want native at v%d", pin, v+1)
		}
	})
}

// I2B-T10: a classification-semantics drift HOLD keeps the accepted source pin; drift is
// not a source revision event and mints no version.
func TestI2B_ClassificationDriftKeepsSourcePin(t *testing.T) {
	root := t.TempDir()
	conn := newAcceptanceDB(t)
	dir := writeRuleFolder(t, root, "runA", pairFiles("A")...)
	acceptBaseline(t, conn, root)
	before := acceptedPinForTest(t, conn)

	setRule(t, dir, ruleJSONR2)
	res := syncForTest(t, conn, root, nil, acceptanceExclusions)
	if res.Outcome != OutcomeReclassifyHold {
		t.Fatalf("rule drift = %s (%s), want reclassify-hold", res.Outcome, res.Reason)
	}
	if after := acceptedPinForTest(t, conn); after != before {
		t.Fatalf("drift HOLD changed the source pin: %+v → %+v", before, after)
	}
}

// I2B-T11: pinning the source basis mints no Generation / publication / Auto-Run identity.
func TestI2B_NoGenerationIdentityMinted(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	conn := newAcceptanceDB(t)
	writeRuleFolder(t, root, "runA", pairFiles("A")...)
	acceptBaseline(t, conn, root)

	rows, err := conn.QueryContext(ctx, "SELECT name FROM sqlite_master WHERE type = 'table'")
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan: %v", err)
		}
		lower := strings.ToLower(name)
		for _, forbidden := range []string{"generation", "publication", "autorun", "auto_run"} {
			if strings.Contains(lower, forbidden) {
				t.Errorf("table %s mints a %s identity", name, forbidden)
			}
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
}
