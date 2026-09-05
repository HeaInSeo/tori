package db

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/HeaInSeo/tori/block"
	"github.com/HeaInSeo/tori/rules"
)

// This file implements the TDI-I1+I10 "Legacy Observation Acceptance Boundary".
//
// The legacy SyncFolders path mutated the accepted inventory (DB rows) and the
// generated projection (FileBlock/DataBlock) directly from whatever a single
// os.ReadDir happened to return. That has two failure modes this boundary closes:
//
//   I1  — acceptance boundary: a readable-but-wrong or empty source scope (e.g.
//         a detached shared/remote mount) could be accepted as the original
//         source and silently wipe the accepted inventory + projection.
//   I10 — crash-recoverable acceptance: the DB could advance while the projection
//         stayed stale, and a partial multi-row DB mutation could survive; on the
//         next run an empty diff + an existing datablock.pb looked like a complete
//         accepted snapshot.
//
// The mechanism is deliberately the smallest that distinguishes the audited
// failure cases; it is NOT a platform-level source-identity authority:
//
//   * Source-continuity witness (local/shared POSIX profile): a zero-length marker
//     file named "<witnessPrefix><token>" placed at the source root. The token is
//     also recorded in the DB (snapshot_meta). Continuity holds only when the
//     recorded token is non-empty AND exactly one witness file with that token is
//     present at the root. A merely readable path (empty detached mount, wrong
//     replacement mount) does not carry our token and is therefore UNKNOWN.
//     Detection uses os.ReadDir only (no file-content read), so the marker cannot
//     pollute the tracked inventory (top-level files are never enumerated).
//
//   * Coverage encoding: {Complete, Partial}. Every disk subfolder in scope must be
//     fully readable before any mutation; an unreadable subfolder yields Partial.
//
//   * Acceptance/reconcile: a durable dirty marker + accepted-version record in
//     snapshot_meta, combined with an atomic (single-transaction) DB mutation and
//     a rebuild-from-DB projection. The ordered phases guarantee that a crash can
//     never leave a stale projection silently accepted:
//        1. mark pending + bump target version   (durable "about to mutate")
//        2. apply all DB row mutations atomically (single tx, all-or-nothing)
//        3. rebuild projection (FileBlock/DataBlock) from the accepted DB
//        4. mark clean + set accepted = target   (acceptance is now durable)
//     On restart, a pending marker forces a rebuild-from-DB BEFORE any "unchanged"
//     can be returned; the rebuild is idempotent, so re-entry converges.

const (
	// witnessPrefix names the source-continuity marker file. The full name is
	// witnessPrefix followed by the recorded token. It lives at the source root
	// and is never part of the tracked inventory (top-level files are not
	// enumerated by the folder/file diff).
	witnessPrefix = ".tori_source_"

	metaKeySourceWitness   = "source_witness"
	metaKeyAcceptanceState = "acceptance_state"
	metaKeyAcceptedVersion = "accepted_version"
	metaKeyTargetVersion   = "target_version"

	acceptanceClean   = "clean"
	acceptancePending = "pending"
)

// Scope is the source-continuity classification of an observation.
type Scope int

const (
	// ScopeUnknown means source continuity could not be proven: no witness, a
	// wrong/replaced witness token, or the root could not be observed at all.
	ScopeUnknown Scope = iota
	// ScopeConfirmed means the observed root carries our recorded witness token
	// (or this is a legitimate bootstrap with no prior accepted snapshot).
	ScopeConfirmed
)

func (s Scope) String() string {
	switch s {
	case ScopeConfirmed:
		return "CONFIRMED"
	default:
		return "UNKNOWN"
	}
}

// Coverage is the enumeration-completeness classification of an observation.
type Coverage int

const (
	// CoverageUnknown means completeness could not be determined (root not observed).
	CoverageUnknown Coverage = iota
	// CoveragePartial means at least one in-scope subfolder could not be fully read.
	CoveragePartial
	// CoverageComplete means every in-scope subfolder was fully enumerated.
	CoverageComplete
)

func (c Coverage) String() string {
	switch c {
	case CoverageComplete:
		return "COMPLETE"
	case CoveragePartial:
		return "PARTIAL"
	default:
		return "UNKNOWN"
	}
}

// SyncOutcome is the authoritative result classification returned by SyncFolders.
type SyncOutcome int

const (
	// OutcomeUnchanged means a fully accepted snapshot already matches the source;
	// nothing was mutated or rewritten.
	OutcomeUnchanged SyncOutcome = iota
	// OutcomeAcceptedUpdate means a confirmed+complete observation was durably
	// accepted this run (a legacy add/modify/remove, a first generation, or a
	// crash-recovery reconcile that rebuilt the projection from the accepted DB).
	OutcomeAcceptedUpdate
	// OutcomeDegradedHold means the observation was not accepted: the source scope
	// was UNKNOWN or coverage was not COMPLETE. The previous accepted DB and
	// projection are retained together, untouched.
	OutcomeDegradedHold
	// OutcomeReclassifyHold means the observation was not accepted because the on-disk
	// classification semantics (rule.json) drifted from the accepted frozen basis, or a
	// legacy/pending snapshot's frozen basis could not be proven (TDI-I4F). This is
	// deliberately DISTINCT from OutcomeUnchanged (a rule-only change is not "no change")
	// and from OutcomeDegradedHold (scope/coverage): the accepted R1 DB + projection are
	// retained, R2 is not adopted, and no new/changed data is accepted under stale R1.
	// Resolving it requires the later reclassification/publication path, which I4F does
	// not implement.
	OutcomeReclassifyHold
)

func (o SyncOutcome) String() string {
	switch o {
	case OutcomeAcceptedUpdate:
		return "accepted-update"
	case OutcomeDegradedHold:
		return "degraded-hold"
	case OutcomeReclassifyHold:
		return "reclassify-hold"
	default:
		return "unchanged"
	}
}

// SyncResult is the structured status of a SyncFolders call. It lets callers
// distinguish unchanged / accepted-complete-update / degraded-HOLD without
// conflating a controlled HOLD with a hard error or with "no change".
type SyncResult struct {
	Outcome   SyncOutcome
	Scope     Scope
	Coverage  Coverage
	Reconcile bool   // an incomplete prior acceptance was reconciled this run
	Reason    string // human-readable explanation, primarily for HOLD
}

// sqlDBTX is satisfied by both *sql.DB and *sql.Tx, so the small metadata helpers
// can run either standalone or inside a transaction.
type sqlDBTX interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// ensureMetaTable creates the snapshot_meta table if it does not yet exist. It is
// idempotent and safe to call on every access, which keeps pre-existing DBs (and
// test DBs built from inline schema) working without a migration step.
func ensureMetaTable(ctx context.Context, e sqlDBTX) error {
	const create = `CREATE TABLE IF NOT EXISTS snapshot_meta (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);`
	if _, err := e.ExecContext(ctx, create); err != nil {
		return fmt.Errorf("failed to ensure snapshot_meta table: %w", err)
	}
	return nil
}

// metaGet returns the value for key and whether it was present.
func metaGet(ctx context.Context, e sqlDBTX, key string) (string, bool, error) {
	if err := ensureMetaTable(ctx, e); err != nil {
		return "", false, err
	}
	var value string
	err := e.QueryRowContext(ctx, "SELECT value FROM snapshot_meta WHERE key = ?", key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("failed to read snapshot_meta key %q: %w", key, err)
	}
	return value, true, nil
}

// metaGetInt returns the integer value for key, or 0 when absent.
func metaGetInt(ctx context.Context, e sqlDBTX, key string) (int64, error) {
	v, ok, err := metaGet(ctx, e, key)
	if err != nil {
		return 0, err
	}
	if !ok || v == "" {
		return 0, nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse snapshot_meta int key %q value %q: %w", key, v, err)
	}
	return n, nil
}

// metaSet upserts key=value.
func metaSet(ctx context.Context, e sqlDBTX, key, value string) error {
	if err := ensureMetaTable(ctx, e); err != nil {
		return err
	}
	const upsert = `INSERT INTO snapshot_meta (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value;`
	if _, err := e.ExecContext(ctx, upsert, key, value); err != nil {
		return fmt.Errorf("failed to set snapshot_meta key %q: %w", key, err)
	}
	return nil
}

// readWitnessToken scans the source root for the continuity marker and returns the
// token encoded in its filename. It uses os.ReadDir only, so it never reads file
// contents and never enumerates the marker as tracked inventory.
//
// Returns:
//   - token=="" , ambiguous==false : no marker present
//   - token==t  , ambiguous==false : exactly one marker carrying token t
//   - token=="" , ambiguous==true  : more than one marker (source is ambiguous)
func readWitnessToken(rootPath string) (token string, ambiguous bool, err error) {
	entries, err := os.ReadDir(rootPath)
	if err != nil {
		return "", false, fmt.Errorf("failed to read source root %s: %w", rootPath, err)
	}
	found := ""
	count := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, witnessPrefix) {
			suffix := strings.TrimPrefix(name, witnessPrefix)
			if suffix == "" {
				continue
			}
			found = suffix
			count++
		}
	}
	switch {
	case count == 0:
		return "", false, nil
	case count > 1:
		return "", true, nil
	default:
		return found, false, nil
	}
}

// newWitnessToken returns a fresh random continuity token.
func newWitnessToken() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("failed to generate witness token: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// establishWitnessIfBootstrap records and materializes a source-continuity witness
// only when none has been recorded yet (a legitimate bootstrap). It is a no-op when
// a witness is already recorded, so it can never silently paper over a discontinuity
// detected by observe.
//
// Marker file and DB record cannot be written atomically together, so bootstrap is
// made crash-safe by ADOPTING an already-present single marker rather than always
// generating a new one: if a prior bootstrap crashed after writing the marker but
// before recording the token, restart adopts that marker instead of writing a
// second one (which would later read as ambiguous → a stuck HOLD).
func establishWitnessIfBootstrap(ctx context.Context, db *sql.DB, rootPath string) error {
	recorded, _, err := metaGet(ctx, db, metaKeySourceWitness)
	if err != nil {
		return err
	}
	if recorded != "" {
		return nil
	}
	existing, ambiguous, err := readWitnessToken(rootPath)
	if err != nil {
		return err
	}
	if ambiguous {
		return fmt.Errorf("cannot establish witness: multiple markers already present at %s", rootPath)
	}
	if existing != "" {
		// Adopt the orphan marker left by an interrupted bootstrap.
		return metaSet(ctx, db, metaKeySourceWitness, existing)
	}
	token, err := newWitnessToken()
	if err != nil {
		return err
	}
	markerPath := filepath.Join(rootPath, witnessPrefix+token)
	if err := os.WriteFile(markerPath, []byte{}, 0o600); err != nil {
		return fmt.Errorf("failed to write source witness marker: %w", err)
	}
	return metaSet(ctx, db, metaKeySourceWitness, token)
}

// observe classifies the current source root without mutating anything. It proves
// (a) source-scope continuity and (b) enumeration coverage, in that order.
func observe(ctx context.Context, db *sql.DB, rootPath string, foldersExclusions, filesExclusions []string) (SyncResult, error) {
	// (0) Root observability. If we cannot even read the root, the observation is
	// unavailable: HOLD and retain the previous accepted state.
	fileToken, ambiguous, err := readWitnessToken(rootPath)
	if err != nil {
		return SyncResult{
			Outcome:  OutcomeDegradedHold,
			Scope:    ScopeUnknown,
			Coverage: CoverageUnknown,
			Reason:   fmt.Sprintf("observation unavailable: %v", err),
		}, nil
	}

	// (1) Source-continuity witness.
	recorded, _, err := metaGet(ctx, db, metaKeySourceWitness)
	if err != nil {
		return SyncResult{}, err
	}
	if recorded == "" {
		// No continuity witness recorded. This is a safe bootstrap ONLY when there is
		// genuinely nothing accepted to protect. A legacy/pre-feature DB (or a witness
		// write interrupted after the inventory was committed) can hold accepted
		// inventory with recorded=="": treating that as a bootstrap would let a
		// readable-but-wrong or empty mount be adopted and wipe the accepted inventory
		// (the exact I1 failure this boundary exists to prevent). So the bootstrap
		// branch is gated on an EMPTY accepted inventory, not on the absent witness.
		empty, eErr := acceptedInventoryEmpty(db)
		if eErr != nil {
			return SyncResult{}, eErr
		}
		if !empty {
			// Legacy adoption is only safe when the current source still carries every
			// accepted folder (plausibly the same source, not a wrong/empty mount). If
			// any accepted folder is absent we cannot prove continuity → HOLD; the
			// witness is NOT backfilled here, so a wrong mount is never adopted.
			missing, mErr := firstMissingAcceptedFolder(db, rootPath)
			if mErr != nil {
				return SyncResult{}, mErr
			}
			if missing != "" {
				return SyncResult{
					Outcome: OutcomeDegradedHold, Scope: ScopeUnknown, Coverage: CoverageComplete,
					Reason: fmt.Sprintf("source scope UNKNOWN: accepted inventory exists without a continuity witness and the source diverges (%s missing); refusing to accept — re-bootstrap required", missing),
				}, nil
			}
			// Every accepted folder is present → validated legacy adoption; SyncFolders
			// backfills the witness before any accept/unchanged.
		}
		// Empty inventory → true bootstrap; witness established at acceptance.
	} else {
		switch {
		case ambiguous:
			return SyncResult{
				Outcome: OutcomeDegradedHold, Scope: ScopeUnknown, Coverage: CoverageComplete,
				Reason: "source scope UNKNOWN: multiple/ambiguous continuity markers at root",
			}, nil
		case fileToken == "":
			return SyncResult{
				Outcome: OutcomeDegradedHold, Scope: ScopeUnknown, Coverage: CoverageComplete,
				Reason: "source scope UNKNOWN: continuity witness absent (a readable path is not source-continuity proof)",
			}, nil
		case fileToken != recorded:
			return SyncResult{
				Outcome: OutcomeDegradedHold, Scope: ScopeUnknown, Coverage: CoverageComplete,
				Reason: "source scope UNKNOWN: continuity witness does not match the accepted source",
			}, nil
		}
	}

	// (2) Coverage completeness: every in-scope subfolder must be fully readable
	// before any mutation. A partial enumeration must not be accepted.
	folders, err := GetSubFolders(rootPath, foldersExclusions)
	if err != nil {
		return SyncResult{
			Outcome: OutcomeDegradedHold, Scope: ScopeConfirmed, Coverage: CoverageUnknown,
			Reason: fmt.Sprintf("coverage UNKNOWN: %v", err),
		}, nil
	}
	for _, folder := range folders {
		if _, _, ferr := GetCurrentFolderFileInfo(folder.Path, filesExclusions); ferr != nil {
			return SyncResult{
				Outcome: OutcomeDegradedHold, Scope: ScopeConfirmed, Coverage: CoveragePartial,
				Reason: fmt.Sprintf("coverage PARTIAL: %s not fully readable: %v", folder.Path, ferr),
			}, nil
		}
	}

	return SyncResult{Outcome: OutcomeUnchanged, Scope: ScopeConfirmed, Coverage: CoverageComplete}, nil
}

// acceptedInventoryEmpty reports whether the accepted DB holds no folders. It
// distinguishes a true bootstrap (nothing to protect) from a legacy DB that holds
// accepted inventory but has not yet recorded a continuity witness.
func acceptedInventoryEmpty(db *sql.DB) (bool, error) {
	folders, err := GetFoldersFromDB(db)
	if err != nil {
		return false, err
	}
	return len(folders) == 0, nil
}

// firstMissingAcceptedFolder returns the path of the first accepted folder that is
// not present as a directory UNDER rootPath, or "" when every accepted folder is
// present under it. It is the source-continuity check used for legacy adoption when
// no witness has been recorded yet: an empty/wrong mount (or a different root that
// merely happens to leave the old absolute paths readable elsewhere) will be missing
// the accepted folders and must therefore be refused rather than adopted.
func firstMissingAcceptedFolder(db *sql.DB, rootPath string) (string, error) {
	folders, err := GetFoldersFromDB(db)
	if err != nil {
		return "", err
	}
	for _, f := range folders {
		if !pathWithinRoot(f.Path, rootPath) {
			return f.Path, nil
		}
		info, statErr := os.Stat(f.Path)
		if statErr != nil || !info.IsDir() {
			return f.Path, nil
		}
	}
	return "", nil
}

// pathWithinRoot reports whether p is root itself or lies inside root, guarding the
// legacy-adoption continuity check against accepted paths that resolve outside the
// currently observed source root.
func pathWithinRoot(p, root string) bool {
	rel, err := filepath.Rel(root, p)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)))
}

// buildFolderFilesFromDB reconstructs the [folderPath, name...] projection input
// from the accepted DB rows, so the generated projection always reflects the
// accepted inventory rather than a possibly-drifted disk enumeration.
//
// A folder that vanished from disk during a crash window is skipped rather than
// failing the whole rebuild: projection generation still needs the folder directory
// present to write its compatibility artifacts (csv/invalid/*.pb), so a since-removed
// folder would otherwise hard-error and wedge reconcile into a permanent pending state.
// (The classification rule itself comes from the frozen pinned basis, never disk
// rule.json, per TDI-I4F; only the folder's on-disk presence is required here.)
// Skipping keeps reconcile convergent;
// the removal is then applied authoritatively by the normal diff path in the same
// SyncFolders call (which prunes the DB row and rebuilds a consistent projection).
// It reports complete=false when any folder had to be skipped, so callers do not
// declare a snapshot clean while the projection omits an accepted DB row.
//
// Folders and their file names are emitted in a deterministic sorted order so the
// same accepted inventory always projects to the same FileBlock/row ordering,
// regardless of DB row order or the history by which the snapshot was reached
// (reproducibility: same data + same method = same result).
func buildFolderFilesFromDB(db *sql.DB) (folderFiles [][]string, complete bool, err error) {
	folders, err := GetFoldersFromDB(db)
	if err != nil {
		return nil, false, err
	}
	sort.Slice(folders, func(i, j int) bool { return folders[i].Path < folders[j].Path })
	folderFiles = make([][]string, 0, len(folders))
	complete = true
	for _, folder := range folders {
		if info, statErr := os.Stat(folder.Path); statErr != nil || !info.IsDir() {
			logger.Warnf("reconcile: skipping folder no longer present on disk: %s", folder.Path)
			complete = false
			continue
		}
		files, fErr := GetFilesByPathFromDB(db, folder.Path)
		if fErr != nil {
			return nil, false, fErr
		}
		names := ExtractFileNames(files)
		sort.Strings(names)
		row := append([]string{folder.Path}, names...)
		folderFiles = append(folderFiles, row)
	}
	return folderFiles, complete, nil
}

// regenerateProjectionFromDB rebuilds FileBlocks + datablock.pb from the accepted
// DB rows. It is the single projection-completion path used by both first-time
// acceptance and crash-recovery reconcile, which keeps the two idempotent. It
// returns complete=false when the projection had to omit an accepted folder that
// is no longer present on disk.
func regenerateProjectionFromDB(ctx context.Context, db *sql.DB, rootPath string) (bool, error) {
	folderFiles, complete, err := buildFolderFilesFromDB(db)
	if err != nil {
		return false, err
	}
	// TDI-I4F: project against the FROZEN classification basis (target while pending,
	// accepted while clean), never the mutable on-disk rule.json. A folder with no
	// resolvable frozen basis fails closed (ErrFrozenBasisUnavailable) rather than
	// silently reinterpreting the accepted DB under a later rule.
	state, err := readAcceptanceState(ctx, db)
	if err != nil {
		return false, err
	}
	acceptedVer, err := metaGetInt(ctx, db, metaKeyAcceptedVersion)
	if err != nil {
		return false, err
	}
	targetVer, err := metaGetInt(ctx, db, metaKeyTargetVersion)
	if err != nil {
		return false, err
	}
	pending := state == acceptancePending
	ruleFor := func(folderPath string) (rules.RuleSet, error) {
		return resolveFrozenRuleSet(ctx, db, folderPath, pending, acceptedVer, targetVer)
	}
	fbs, err := block.GenerateFBsWithRules(folderFiles, ruleFor)
	if err != nil {
		return false, fmt.Errorf("failed to generate FileBlocks from accepted DB: %w", err)
	}
	outputDatablock := filepath.Join(rootPath, "datablock.pb")
	if err := block.GenerateDataBlock(fbs, outputDatablock); err != nil {
		return false, fmt.Errorf("failed to generate DataBlock (%s): %w", outputDatablock, err)
	}
	return complete, nil
}

// beginPending durably records that an acceptance is in progress and bumps the
// target version. It MUST commit before any DB mutation so a crash after the
// mutation is detectable on restart.
func beginPending(ctx context.Context, db *sql.DB) (int64, error) {
	accepted, err := metaGetInt(ctx, db, metaKeyAcceptedVersion)
	if err != nil {
		return 0, err
	}
	target := accepted + 1
	if err := metaSet(ctx, db, metaKeyTargetVersion, strconv.FormatInt(target, 10)); err != nil {
		return 0, err
	}
	if err := metaSet(ctx, db, metaKeyAcceptanceState, acceptancePending); err != nil {
		return 0, err
	}
	return target, nil
}

// beginPendingWithBasis is beginPending that ALSO durably pins the complete target
// classification-semantics basis, all in a single transaction (TDI-I4F §6). The exact
// target rule basis for every participating folder is therefore committed atomically
// with the pending transition, BEFORE any accepted DB row mutates: a crash after this
// commit can only leave state=pending with a complete, resolvable target basis under
// target_version, never a pending target with an indeterminate rule basis.
func beginPendingWithBasis(ctx context.Context, db *sql.DB, targetBasis []folderBasis) (int64, error) {
	accepted, err := metaGetInt(ctx, db, metaKeyAcceptedVersion)
	if err != nil {
		return 0, err
	}
	target := accepted + 1
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to begin pending+basis tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	if err := metaSet(ctx, tx, metaKeyTargetVersion, strconv.FormatInt(target, 10)); err != nil {
		return 0, err
	}
	if err := metaSet(ctx, tx, metaKeyAcceptanceState, acceptancePending); err != nil {
		return 0, err
	}
	for _, b := range targetBasis {
		if err := pinSemanticsTx(ctx, tx, target, b); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("failed to commit pending+basis: %w", err)
	}
	committed = true
	return target, nil
}

// commitClean marks the acceptance durable: the projection has been written and the
// accepted version now equals the applied target. The two meta writes run in one
// transaction so promotion of the target basis to the accepted basis (accepted_version
// = target) is atomic with the clean transition (TDI-I4F §6): "accepted basis" is
// exactly the classification_semantics rows under accepted_version, so flipping the
// pointer promotes the whole target basis at once.
func commitClean(ctx context.Context, db *sql.DB, target int64) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin commit-clean tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	if err := metaSet(ctx, tx, metaKeyAcceptedVersion, strconv.FormatInt(target, 10)); err != nil {
		return err
	}
	if err := metaSet(ctx, tx, metaKeyAcceptanceState, acceptanceClean); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit clean transition: %w", err)
	}
	committed = true
	return nil
}

// readAcceptanceState reports whether an acceptance is currently pending.
func readAcceptanceState(ctx context.Context, db *sql.DB) (state string, err error) {
	v, ok, err := metaGet(ctx, db, metaKeyAcceptanceState)
	if err != nil {
		return "", err
	}
	if !ok {
		return acceptanceClean, nil
	}
	return v, nil
}

// reconcileIfPending detects an incomplete prior acceptance and, if found, rebuilds
// the projection from the accepted DB and marks the snapshot clean. It must run
// before any "unchanged" can be returned. It is idempotent: rebuilding a projection
// that is already correct simply rewrites identical inventory and converges.
func reconcileIfPending(ctx context.Context, db *sql.DB, rootPath string) (bool, error) {
	state, err := readAcceptanceState(ctx, db)
	if err != nil {
		return false, err
	}
	if state != acceptancePending {
		return false, nil
	}
	complete, err := regenerateProjectionFromDB(ctx, db, rootPath)
	if err != nil {
		return false, err
	}
	// A bootstrap crash may have left no recorded witness; establish it now so the
	// recovered snapshot is continuity-provable going forward.
	if err := establishWitnessIfBootstrap(ctx, db, rootPath); err != nil {
		return false, err
	}
	if !complete {
		// The projection could not include every accepted folder (one vanished from
		// disk during the pending window). Do NOT declare the snapshot clean while the
		// projection omits an accepted DB row: leave it pending and let the normal
		// diff/accept path in this same SyncFolders run prune the vanished folder and
		// commit a consistent, clean projection. Returning false stops SyncFolders from
		// short-circuiting to "unchanged".
		logger.Warn("reconcile: projection omitted a vanished accepted folder; deferring clean to the diff/accept path")
		return false, nil
	}
	target, err := metaGetInt(ctx, db, metaKeyTargetVersion)
	if err != nil {
		return false, err
	}
	if err := commitClean(ctx, db, target); err != nil {
		return false, err
	}
	return true, nil
}

// acceptWork applies a confirmed+complete observation as a crash-recoverable unit:
// pending marker + frozen target basis (one tx) → atomic DB mutation → projection
// rebuild from the pinned target basis → clean marker (which atomically promotes the
// target basis to the accepted basis). targetBasis is the complete, already-frozen
// per-folder classification basis for every folder that will participate in the accepted
// projection; it MUST be preflighted (freezeDiskBasis) before this call so a missing or
// invalid rule HOLDs before any DB row advances (TDI-I4F §5).
func acceptWork(ctx context.Context, db *sql.DB, rootPath string, diffs []FolderDiff, changes []FileChange, targetBasis []folderBasis) error {
	target, err := beginPendingWithBasis(ctx, db, targetBasis)
	if err != nil {
		return err
	}
	if len(diffs) > 0 || len(changes) > 0 {
		if err := UpdateDB(ctx, db, diffs, changes); err != nil {
			// The mutation is atomic (single tx); on failure the DB is unchanged.
			// The pending marker + target basis remain so the next run reconciles safely.
			return err
		}
	}
	// acceptWork projects the accepted DB it just wrote to match the confirmed source,
	// so the rebuild is expected to be complete; the completeness flag is consumed by
	// the reconcile path (not here), where a vanished folder must defer the clean mark.
	if _, err := regenerateProjectionFromDB(ctx, db, rootPath); err != nil {
		return err
	}
	if err := establishWitnessIfBootstrap(ctx, db, rootPath); err != nil {
		return err
	}
	return commitClean(ctx, db, target)
}
