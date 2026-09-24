package db

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TDI-I2A acceptance suite: the durable source envelope must keep three things apart
// that RootDir used to collapse into one — logical identity, observation meaning, and
// physical access route. Each test below is one of the packet's six required cases.
//
// Tests live in package db so they can assert the persisted rows directly (an envelope
// that is only correct in memory is not an envelope).

// ensureEnvelope is the common establish call, with an explicit endpoint.
func ensureEnvelope(t *testing.T, db *sql.DB, root, credRef string, folderEx, fileEx []string) SourceEnvelope {
	t.Helper()
	env, err := EnsureSourceEnvelope(context.Background(), db, SourceEnvelopeInput{
		RootDir:           root,
		CredentialRef:     credRef,
		FoldersExclusions: folderEx,
		FilesExclusions:   fileEx,
	})
	if err != nil {
		t.Fatalf("EnsureSourceEnvelope(root=%s cred=%q): %v", root, credRef, err)
	}
	return env
}

// I2A-T01: a path/endpoint change with the same proven source retains the SourceID.
//
// This is the failure the packet exists to prevent: a mount that moves must not read as
// a different source. The move is proven-same by carrying the continuity witness across,
// which is exactly what the I1 boundary already attests.
func TestI2A_EndpointRelocationRetainsSourceID(t *testing.T) {
	ctx := context.Background()
	oldRoot := t.TempDir()
	conn := newAcceptanceDB(t)

	writeRuleFolder(t, oldRoot, "runA", pairFiles("A")...)
	acceptBaseline(t, conn, oldRoot)

	before, ok, err := GetSourceEnvelope(ctx, conn)
	if err != nil || !ok {
		t.Fatalf("GetSourceEnvelope after baseline: ok=%v err=%v", ok, err)
	}
	if before.SourceID == "" || before.CurrentEndpointID == "" || before.CurrentRevisionID == "" {
		t.Fatalf("baseline envelope incomplete: %+v", before)
	}

	// An actual filesystem relocation: the same directory — contents, rule files and
	// continuity witness included — now lives at a different path. The move is then
	// driven through SyncFolders, so it is adopted only after observe() actually PROVES
	// continuity, exactly as in production.
	newRoot := filepath.Join(t.TempDir(), "relocated")
	if err := os.Rename(oldRoot, newRoot); err != nil {
		t.Fatalf("relocate %s → %s: %v", oldRoot, newRoot, err)
	}

	res, err := SyncFolders(ctx, conn, newRoot, nil, acceptanceExclusions)
	if err != nil {
		t.Fatalf("SyncFolders after relocation: %v", err)
	}
	if res.Scope != ScopeConfirmed {
		t.Fatalf("relocated source scope = %s, want CONFIRMED (witness was carried across): %s",
			res.Scope, res.Reason)
	}

	after, ok, err := GetSourceEnvelope(ctx, conn)
	if err != nil || !ok {
		t.Fatalf("GetSourceEnvelope after relocation: ok=%v err=%v", ok, err)
	}

	if after.SourceID != before.SourceID {
		t.Errorf("relocation changed SourceID: %s → %s (identity must not follow the path)",
			before.SourceID, after.SourceID)
	}
	if after.CurrentRevisionID != before.CurrentRevisionID {
		t.Errorf("relocation changed the semantic revision: %s → %s (an endpoint move is not a meaning change)",
			before.CurrentRevisionID, after.CurrentRevisionID)
	}
	if after.CurrentEndpointID == before.CurrentEndpointID {
		t.Errorf("relocation did not record a new endpoint (endpoint %s unchanged for %s → %s)",
			after.CurrentEndpointID, oldRoot, newRoot)
	}

	// The prior endpoint is retained as history, not overwritten.
	endpoints, err := CountSourceEndpoints(ctx, conn, after.SourceID)
	if err != nil {
		t.Fatalf("CountSourceEndpoints: %v", err)
	}
	if endpoints != 2 {
		t.Errorf("endpoint history = %d, want 2 (old endpoint must be retained)", endpoints)
	}
	revisions, err := CountSourceRevisions(ctx, conn, after.SourceID)
	if err != nil {
		t.Fatalf("CountSourceRevisions: %v", err)
	}
	if revisions != 1 {
		t.Errorf("revision count = %d, want 1 (relocation must not mint a revision)", revisions)
	}
}

// I2A-T01b is the negative half of T01: an UNPROVEN root must not be adopted as this
// source's endpoint. Without this, "the endpoint can move" would degrade into "any
// readable path can claim to be this source" — the exact I1 failure the witness exists
// to stop, re-introduced one layer up.
func TestI2A_UnprovenRootIsNotAdoptedAsEndpoint(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	conn := newAcceptanceDB(t)

	writeRuleFolder(t, root, "runA", pairFiles("A")...)
	acceptBaseline(t, conn, root)

	before, ok, err := GetSourceEnvelope(ctx, conn)
	if err != nil || !ok {
		t.Fatalf("GetSourceEnvelope after baseline: ok=%v err=%v", ok, err)
	}

	// A different, readable path that carries NO continuity witness.
	wrongRoot := t.TempDir()
	writeRuleFolder(t, wrongRoot, "runA", pairFiles("A")...)

	res, err := SyncFolders(ctx, conn, wrongRoot, nil, acceptanceExclusions)
	if err != nil {
		t.Fatalf("SyncFolders on unproven root: %v", err)
	}
	if res.Outcome != OutcomeDegradedHold {
		t.Fatalf("outcome = %s, want degraded-hold for an unproven root", res.Outcome)
	}

	after, ok, err := GetSourceEnvelope(ctx, conn)
	if err != nil || !ok {
		t.Fatalf("GetSourceEnvelope after HOLD: ok=%v err=%v", ok, err)
	}
	if after != before {
		t.Errorf("a HELD (unproven) root mutated the envelope:\n before: %+v\n after:  %+v", before, after)
	}
	endpoints, err := CountSourceEndpoints(ctx, conn, before.SourceID)
	if err != nil {
		t.Fatalf("CountSourceEndpoints: %v", err)
	}
	if endpoints != 1 {
		t.Errorf("endpoint count = %d, want 1 (the unproven root must not be recorded)", endpoints)
	}
}

// I2A-T02: a scope/exclusion semantic change creates a NEW SourceRevision and leaves the
// previous one byte-identical. Mutating a revision in place would silently reinterpret
// every snapshot already accepted under it.
func TestI2A_ScopeChangeCreatesNewRevisionWithoutMutatingOld(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	conn := newAcceptanceDB(t)

	first := ensureEnvelope(t, conn, root, "", nil, acceptanceExclusions)
	firstCanonical, ok, err := SourceRevisionCanonical(ctx, conn, first.SourceID, first.CurrentRevisionID)
	if err != nil || !ok {
		t.Fatalf("SourceRevisionCanonical(first): ok=%v err=%v", ok, err)
	}

	// Semantic change: a new file-exclusion pattern narrows the observation domain.
	widened := append(append([]string{}, acceptanceExclusions...), "*.bam")
	second := ensureEnvelope(t, conn, root, "", nil, widened)

	if second.SourceID != first.SourceID {
		t.Errorf("scope change altered SourceID: %s → %s (meaning changed, identity did not)",
			first.SourceID, second.SourceID)
	}
	if second.CurrentRevisionID == first.CurrentRevisionID {
		t.Fatalf("scope change did not mint a new revision (still %s)", first.CurrentRevisionID)
	}

	// The OLD revision must still exist, with its original canonical bytes.
	stillThere, ok, err := SourceRevisionCanonical(ctx, conn, first.SourceID, first.CurrentRevisionID)
	if err != nil {
		t.Fatalf("SourceRevisionCanonical(old): %v", err)
	}
	if !ok {
		t.Fatalf("old revision %s was removed by the scope change", first.CurrentRevisionID)
	}
	if stillThere != firstCanonical {
		t.Errorf("old revision %s was mutated in place:\n old: %s\n new: %s",
			first.CurrentRevisionID, firstCanonical, stillThere)
	}

	count, err := CountSourceRevisions(ctx, conn, first.SourceID)
	if err != nil {
		t.Fatalf("CountSourceRevisions: %v", err)
	}
	if count != 2 {
		t.Errorf("revision count = %d, want 2 (append, not overwrite)", count)
	}
}

// I2A-T02b pins the boundary the other way: reordering or repeating exclusion patterns is
// NOT a semantic change, because the lists are matched as sets. Without this, a cosmetic
// config edit would mint a revision and look like a reinterpretation.
func TestI2A_ExclusionReorderIsNotASemanticChange(t *testing.T) {
	root := t.TempDir()
	conn := newAcceptanceDB(t)

	first := ensureEnvelope(t, conn, root, "", []string{"tmp", "cache"}, []string{"*.json", "*.pb"})

	reordered := ensureEnvelope(t, conn, root, "",
		[]string{"cache", "tmp", "cache"}, // reordered + duplicated
		[]string{"*.pb", "*.json", ""},    // reordered + an empty (meaningless) pattern
	)

	if reordered.CurrentRevisionID != first.CurrentRevisionID {
		t.Errorf("reordering/duplicating exclusions minted a new revision: %s → %s",
			first.CurrentRevisionID, reordered.CurrentRevisionID)
	}
	count, err := CountSourceRevisions(context.Background(), conn, first.SourceID)
	if err != nil {
		t.Fatalf("CountSourceRevisions: %v", err)
	}
	if count != 1 {
		t.Errorf("revision count = %d, want 1", count)
	}
}

// I2A-T03: SourceID and the semantic revision are both stable across a credential-only
// change. Only the access endpoint moves.
func TestI2A_CredentialRotationKeepsSourceIDAndRevision(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	conn := newAcceptanceDB(t)

	before := ensureEnvelope(t, conn, root, "secret-ref-v1", nil, acceptanceExclusions)
	after := ensureEnvelope(t, conn, root, "secret-ref-v2", nil, acceptanceExclusions)

	if after.SourceID != before.SourceID {
		t.Errorf("credential rotation changed SourceID: %s → %s", before.SourceID, after.SourceID)
	}
	if after.CurrentRevisionID != before.CurrentRevisionID {
		t.Errorf("credential rotation changed the semantic revision: %s → %s (a credential carries no meaning)",
			before.CurrentRevisionID, after.CurrentRevisionID)
	}
	if after.CurrentEndpointID == before.CurrentEndpointID {
		t.Errorf("credential rotation was not recorded as an endpoint change (endpoint stayed %s)",
			after.CurrentEndpointID)
	}

	revisions, err := CountSourceRevisions(ctx, conn, after.SourceID)
	if err != nil {
		t.Fatalf("CountSourceRevisions: %v", err)
	}
	if revisions != 1 {
		t.Errorf("revision count = %d, want 1 (rotation must not mint a revision)", revisions)
	}
}

// I2A-T03c: GetSourceEnvelope must return a revision and an endpoint that were current
// at the same moment. EnsureSourceEnvelope moves both pointers in one transaction, so a
// read that interleaves with it must see both moves or neither — never the old revision
// with the new endpoint. The hook runs the writer between the two pointer reads, the
// only place a torn read can show. It waits long enough for an unblocked writer to
// commit. With one read snapshot, the writer's commit waits for the read to finish.
func TestI2A_GetSourceEnvelopeReadsOneSnapshot(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	conn := newAcceptanceDB(t)

	before := ensureEnvelope(t, conn, root, "ref-a", nil, acceptanceExclusions)

	// The concurrent change moves both pointers: a new scope and a rotated reference.
	rotated := "ref-b"
	widened := append(append([]string{}, acceptanceExclusions...), "*.bam")
	writerDone := make(chan error, 1)
	// Fire only for the first caller, which is the read under test. The writer passes
	// through this hook too, inside its own transaction, and must not be held up there:
	// that would keep it from committing and hide a torn read.
	var fired atomic.Bool
	testHookBetweenPointerReads = func() {
		if !fired.CompareAndSwap(false, true) {
			return
		}
		go func() {
			_, err := EnsureSourceEnvelope(ctx, conn, SourceEnvelopeInput{
				RootDir:         root,
				CredentialRef:   rotated,
				FilesExclusions: widened,
			})
			writerDone <- err
		}()
		select {
		case err := <-writerDone:
			writerDone <- err // committed while the read was open; keep the result
		case <-time.After(300 * time.Millisecond):
		}
	}
	t.Cleanup(func() { testHookBetweenPointerReads = nil })

	got, ok, err := GetSourceEnvelope(ctx, conn)
	if err != nil || !ok {
		t.Fatalf("GetSourceEnvelope during concurrent change: ok=%v err=%v", ok, err)
	}
	if werr := <-writerDone; werr != nil {
		t.Fatalf("concurrent EnsureSourceEnvelope: %v", werr)
	}
	testHookBetweenPointerReads = nil

	after, ok, err := GetSourceEnvelope(ctx, conn)
	if err != nil || !ok {
		t.Fatalf("GetSourceEnvelope after concurrent change: ok=%v err=%v", ok, err)
	}
	if after.CurrentRevisionID == before.CurrentRevisionID || after.CurrentEndpointID == before.CurrentEndpointID {
		t.Fatalf("setup: the concurrent change must move both pointers (before %s/%s, after %s/%s)",
			before.CurrentRevisionID, before.CurrentEndpointID, after.CurrentRevisionID, after.CurrentEndpointID)
	}

	sawBefore := got.CurrentRevisionID == before.CurrentRevisionID && got.CurrentEndpointID == before.CurrentEndpointID
	sawAfter := got.CurrentRevisionID == after.CurrentRevisionID && got.CurrentEndpointID == after.CurrentEndpointID
	if !sawBefore && !sawAfter {
		t.Errorf("torn read: revision %s with endpoint %s were never current together "+
			"(before %s/%s, after %s/%s)", got.CurrentRevisionID, got.CurrentEndpointID,
			before.CurrentRevisionID, before.CurrentEndpointID, after.CurrentRevisionID, after.CurrentEndpointID)
	}
}

// I2A-T03b is the production-path half of T03. T03 proves that rotation is endpoint-only
// when the caller hands EnsureSourceEnvelope the reference directly; that says nothing
// about whether the normal CLI/service sync path ever passes one. Without this test,
// `accessCredentialRef` could be rotated in config forever while CurrentEndpointID never
// moved — the documented endpoint contract would hold in the unit test and be false in
// production, which is the more dangerous of the two failures.
func TestI2A_SyncFoldersRecordsConfiguredCredentialRef(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	conn := newAcceptanceDB(t)

	writeRuleFolder(t, root, "runA", pairFiles("A")...)
	acceptBaseline(t, conn, root)

	before, ok, err := GetSourceEnvelope(ctx, conn)
	if err != nil || !ok {
		t.Fatalf("GetSourceEnvelope after baseline: ok=%v err=%v", ok, err)
	}

	// Held in a variable rather than inlined into the composite literal below: gosec
	// G101 flags a credential-shaped string literal assigned straight to a CredentialRef
	// field. The value is an opaque reference, never secret material — which is exactly
	// the distinction this packet persists.
	endpointRef := "rotated-ref-v2"

	// Rotate the CONFIGURED reference and drive the normal sync path, exactly as
	// DataBlockCliService.SyncFolders does.
	res, err := SyncFolders(ctx, conn, root, nil, acceptanceExclusions,
		WithAccessCredentialRef(endpointRef))
	if err != nil {
		t.Fatalf("SyncFolders with configured credential ref: %v", err)
	}
	if res.Scope != ScopeConfirmed {
		t.Fatalf("scope = %s, want CONFIRMED: %s", res.Scope, res.Reason)
	}

	after, ok, err := GetSourceEnvelope(ctx, conn)
	if err != nil || !ok {
		t.Fatalf("GetSourceEnvelope after rotation: ok=%v err=%v", ok, err)
	}

	// The endpoint actually persisted must be the one derived from the configured
	// reference — not merely "some new endpoint".
	_, wantEndpoint, err := SourceAccessEndpoint{RootDir: root, CredentialRef: endpointRef}.EndpointID()
	if err != nil {
		t.Fatalf("EndpointID: %v", err)
	}
	if after.CurrentEndpointID != wantEndpoint {
		t.Errorf("SyncFolders did not carry the configured credential ref into the endpoint: got %s, want %s",
			after.CurrentEndpointID, wantEndpoint)
	}
	if after.CurrentEndpointID == before.CurrentEndpointID {
		t.Errorf("rotating the configured credential ref left the endpoint unchanged (%s)",
			after.CurrentEndpointID)
	}

	// Rotation is still endpoint-only on this path.
	if after.SourceID != before.SourceID {
		t.Errorf("configured rotation changed SourceID: %s → %s", before.SourceID, after.SourceID)
	}
	if after.CurrentRevisionID != before.CurrentRevisionID {
		t.Errorf("configured rotation changed the semantic revision: %s → %s",
			before.CurrentRevisionID, after.CurrentRevisionID)
	}
	revisions, err := CountSourceRevisions(ctx, conn, after.SourceID)
	if err != nil {
		t.Fatalf("CountSourceRevisions: %v", err)
	}
	if revisions != 1 {
		t.Errorf("revision count = %d, want 1 (rotation must not mint a revision)", revisions)
	}
}

// I2A-T04: legacy RootDir bootstrap is durable and idempotent, and the migration
// provenance is explicit — the minted SourceID must NOT claim to reach back over
// inventory that already existed when it was minted.
func TestI2A_LegacyBootstrapIsDurableIdempotentAndHonest(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	conn := newAcceptanceDB(t)

	// Pre-I2A state: accepted inventory exists, no envelope has ever been established.
	// SaveFolders alone would leave a seed_only (never accepted) seed, which is a bootstrap
	// — see TestI2A_FreshSeedOnlyInventoryBootstraps — so record the provenance an
	// inventory accepted before I2A carries.
	writeRuleFolder(t, root, "legacy", pairFiles("L")...)
	if err := SaveFolders(ctx, conn, root, nil, acceptanceExclusions); err != nil {
		t.Fatalf("SaveFolders: %v", err)
	}
	if err := setProvenanceTx(ctx, conn, provenanceAccepted); err != nil {
		t.Fatalf("record accepted provenance: %v", err)
	}
	if _, ok, err := GetSourceEnvelope(ctx, conn); err != nil || ok {
		t.Fatalf("precondition: expected no envelope yet, got ok=%v err=%v", ok, err)
	}

	adopted := ensureEnvelope(t, conn, root, "", nil, acceptanceExclusions)

	if adopted.AdoptionOrigin != originLegacyAdopted {
		t.Errorf("adoption origin = %q, want %q (inventory existed before the identity)",
			adopted.AdoptionOrigin, originLegacyAdopted)
	}
	if !adopted.InventoryPredatesID {
		t.Error("InventoryPredatesID = false; the envelope is claiming this SourceID is as old as the inventory it names")
	}

	// Idempotent: re-running establishes nothing new.
	again := ensureEnvelope(t, conn, root, "", nil, acceptanceExclusions)
	if again != adopted {
		t.Errorf("re-establish was not idempotent:\n first: %+v\n again: %+v", adopted, again)
	}
	revisions, err := CountSourceRevisions(ctx, conn, adopted.SourceID)
	if err != nil {
		t.Fatalf("CountSourceRevisions: %v", err)
	}
	if revisions != 1 {
		t.Errorf("revision count after re-establish = %d, want 1", revisions)
	}

	// Durable: a reopened store sees the same identity, revision and endpoint.
	reloaded, ok, err := GetSourceEnvelope(ctx, conn)
	if err != nil || !ok {
		t.Fatalf("GetSourceEnvelope after re-establish: ok=%v err=%v", ok, err)
	}
	if reloaded != adopted {
		t.Errorf("envelope did not survive a reopen:\n stored: %+v\n read:   %+v", adopted, reloaded)
	}

	// Contrast: a genuinely fresh DB bootstraps honestly as `bootstrap`, so the two
	// cases stay distinguishable forever.
	freshRoot := t.TempDir()
	freshConn := newAcceptanceDB(t)
	fresh := ensureEnvelope(t, freshConn, freshRoot, "", nil, acceptanceExclusions)
	if fresh.AdoptionOrigin != originBootstrap {
		t.Errorf("fresh adoption origin = %q, want %q", fresh.AdoptionOrigin, originBootstrap)
	}
	if fresh.InventoryPredatesID {
		t.Error("fresh bootstrap reported InventoryPredatesID = true")
	}
	if fresh.SourceID == adopted.SourceID {
		t.Error("two independent sources were minted the same SourceID")
	}
}

// I2A-T04c: the normal CLI flow seeds with SaveFolders before its first SyncFolders. That
// fresh seed has folder rows but was never accepted (provenance seed_only), so the
// envelope minted on the first sync is an honest bootstrap. Classifying by row count alone
// labeled it legacy-adopted with InventoryPredatesID=true, permanently. A DB with rows and
// no provenance at all (pre-v0.3) must still adopt as legacy: fail-closed is unchanged.
func TestI2A_FreshSeedOnlyInventoryBootstraps(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	conn := newAcceptanceDB(t)

	writeRuleFolder(t, root, "runA", pairFiles("A")...)
	if err := SaveFolders(ctx, conn, root, nil, acceptanceExclusions); err != nil {
		t.Fatalf("SaveFolders: %v", err)
	}
	if p, ok, err := getProvenance(ctx, conn); err != nil || !ok || p != provenanceSeedOnly {
		t.Fatalf("precondition: provenance = %q ok=%v err=%v, want %q", p, ok, err, provenanceSeedOnly)
	}
	if empty, err := acceptedInventoryEmpty(conn); err != nil || empty {
		t.Fatalf("precondition: seed must hold folder rows (empty=%v err=%v)", empty, err)
	}

	// The production order: first SyncFolders after the seed establishes the envelope.
	res, err := SyncFolders(ctx, conn, root, nil, acceptanceExclusions)
	if err != nil {
		t.Fatalf("SyncFolders: %v", err)
	}
	if res.Scope != ScopeConfirmed {
		t.Fatalf("scope = %s, want CONFIRMED: %s", res.Scope, res.Reason)
	}
	env, ok, err := GetSourceEnvelope(ctx, conn)
	if err != nil || !ok {
		t.Fatalf("GetSourceEnvelope: ok=%v err=%v", ok, err)
	}
	if env.AdoptionOrigin != originBootstrap {
		t.Errorf("adoption origin = %q, want %q (a never-accepted seed is not legacy inventory)",
			env.AdoptionOrigin, originBootstrap)
	}
	if env.InventoryPredatesID {
		t.Error("InventoryPredatesID = true for a fresh seed_only inventory")
	}

	// Contrast: rows with no recorded provenance (pre-v0.3) are never inferred to be a seed.
	legacyRoot := t.TempDir()
	legacyConn := newAcceptanceDB(t)
	writeRuleFolder(t, legacyRoot, "legacy", pairFiles("L")...)
	if err := SaveFolders(ctx, legacyConn, legacyRoot, nil, acceptanceExclusions); err != nil {
		t.Fatalf("SaveFolders (legacy): %v", err)
	}
	if _, err := legacyConn.ExecContext(ctx,
		"DELETE FROM snapshot_meta WHERE key = ?", metaKeyAcceptanceProvenance); err != nil {
		t.Fatalf("drop provenance to model a pre-v0.3 DB: %v", err)
	}
	legacy := ensureEnvelope(t, legacyConn, legacyRoot, "", nil, acceptanceExclusions)
	if legacy.AdoptionOrigin != originLegacyAdopted || !legacy.InventoryPredatesID {
		t.Errorf("rows without provenance adopted as origin=%q predates=%v, want %q/true",
			legacy.AdoptionOrigin, legacy.InventoryPredatesID, originLegacyAdopted)
	}
}

// I2A-T04b: the envelope survives a real store reopen with its identity, revision and
// endpoint intact — the packet's "durable" claim, proven against a closed/reopened DB
// file rather than a live handle.
func TestI2A_EnvelopeSurvivesStoreReopen(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "file_monitor.db")

	first, err := ConnectDB("sqlite3", dbPath, true)
	if err != nil {
		t.Fatalf("ConnectDB: %v", err)
	}
	if err := InitializeDatabase(first); err != nil {
		t.Fatalf("InitializeDatabase: %v", err)
	}
	established := ensureEnvelope(t, first, root, "cred-1", []string{"tmp"}, acceptanceExclusions)
	if err := first.Close(); err != nil {
		t.Fatalf("close first handle: %v", err)
	}

	second, err := ConnectDB("sqlite3", dbPath, true)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() {
		if cErr := second.Close(); cErr != nil {
			t.Logf("close second handle: %v", cErr)
		}
	})

	reopened, ok, err := GetSourceEnvelope(ctx, second)
	if err != nil || !ok {
		t.Fatalf("GetSourceEnvelope after reopen: ok=%v err=%v", ok, err)
	}
	if reopened != established {
		t.Errorf("envelope changed across reopen:\n before: %+v\n after:  %+v", established, reopened)
	}

	// And the frozen meaning is still readable from the persisted revision.
	canonical, ok, err := SourceRevisionCanonical(ctx, second, reopened.SourceID, reopened.CurrentRevisionID)
	if err != nil || !ok {
		t.Fatalf("SourceRevisionCanonical after reopen: ok=%v err=%v", ok, err)
	}
	if !strings.Contains(canonical, "foldersExclusions") {
		t.Errorf("persisted canonical semantics look wrong: %s", canonical)
	}
}

// I2A-T05: the continuity witness token cannot serve as the SourceID. The witness proves
// only that this endpoint is still the same physical source; it is rotatable and lives in
// the source filesystem, so promoting it to identity would make the logical source die
// and be reborn on every re-bootstrap.
func TestI2A_WitnessTokenCannotBeSourceID(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	conn := newAcceptanceDB(t)

	writeRuleFolder(t, root, "runA", pairFiles("A")...)
	acceptBaseline(t, conn, root)

	witness, ok, err := metaGet(ctx, conn, metaKeySourceWitness)
	if err != nil || !ok || witness == "" {
		t.Fatalf("precondition: expected a recorded witness, got %q ok=%v err=%v", witness, ok, err)
	}

	env, ok, err := GetSourceEnvelope(ctx, conn)
	if err != nil || !ok {
		t.Fatalf("GetSourceEnvelope: ok=%v err=%v", ok, err)
	}

	// The minted identity is namespaced and independent of the witness.
	if env.SourceID == witness {
		t.Fatalf("SourceID was minted as the witness token %s", witness)
	}
	if !strings.HasPrefix(env.SourceID, sourceIDPrefix) {
		t.Errorf("SourceID %q lacks the %q namespace that keeps it disjoint from a bare witness token",
			env.SourceID, sourceIDPrefix)
	}

	// Corrupt the stored identity to the witness token and prove the guard fails closed
	// rather than continuing to write state under it.
	if _, err := conn.ExecContext(ctx,
		"UPDATE source_envelope SET source_id = ? WHERE singleton = 1", witness); err != nil {
		t.Fatalf("corrupt source_id: %v", err)
	}
	_, err = EnsureSourceEnvelope(ctx, conn, SourceEnvelopeInput{
		RootDir:           root,
		FoldersExclusions: nil,
		FilesExclusions:   acceptanceExclusions,
	})
	if !errors.Is(err, errWitnessAsSourceID) {
		t.Errorf("EnsureSourceEnvelope error = %v, want %v", err, errWitnessAsSourceID)
	}

	// The prefixed form is rejected too, so "namespace the witness" is not a loophole.
	if gErr := validateSourceIDNotWitness(sourceIDPrefix+witness, witness); !errors.Is(gErr, errWitnessAsSourceID) {
		t.Errorf("validateSourceIDNotWitness(prefixed witness) = %v, want %v", gErr, errWitnessAsSourceID)
	}
	// A normal minted id against the same witness is fine.
	if gErr := validateSourceIDNotWitness(env.SourceID, witness); gErr != nil {
		t.Errorf("validateSourceIDNotWitness(minted id) = %v, want nil", gErr)
	}
}

// I2A-T06: establishing the envelope creates no Generation / Auto-Run / publication
// identity. I2A is identity plumbing only; publication is a later packet.
func TestI2A_EstablishCreatesNoPublicationIdentity(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	conn := newAcceptanceDB(t)

	writeRuleFolder(t, root, "runA", pairFiles("A")...)

	env := ensureEnvelope(t, conn, root, "", nil, acceptanceExclusions)
	if env.SourceID == "" {
		t.Fatal("expected an established envelope")
	}

	// No projection was generated.
	if _, err := os.Stat(filepath.Join(root, "datablock.pb")); !os.IsNotExist(err) {
		t.Errorf("establishing the envelope produced a projection at datablock.pb (stat err = %v)", err)
	}

	// No acceptance/publication state was advanced.
	for _, key := range []string{metaKeyAcceptanceState, metaKeyAcceptedVersion, metaKeyTargetVersion} {
		if v, ok, err := metaGet(ctx, conn, key); err != nil {
			t.Fatalf("metaGet(%s): %v", key, err)
		} else if ok {
			t.Errorf("establishing the envelope set acceptance meta %s = %q", key, v)
		}
	}

	// No classification/publication basis was pinned.
	if n, err := countSemanticsAtVersion(ctx, conn, 0); err != nil {
		t.Fatalf("countSemanticsAtVersion: %v", err)
	} else if n != 0 {
		t.Errorf("establishing the envelope pinned %d classification basis row(s)", n)
	}

	// And it did not touch the accepted inventory.
	folders, err := GetFoldersFromDB(conn)
	if err != nil {
		t.Fatalf("GetFoldersFromDB: %v", err)
	}
	if len(folders) != 0 {
		t.Errorf("establishing the envelope wrote %d inventory row(s)", len(folders))
	}
}
