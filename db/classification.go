package db

// TDI-I4F: frozen classification-semantics basis for legacy acceptance/reconcile.
//
// The I1/I10 acceptance boundary (acceptance.go) rebuilds the compatibility projection
// from the accepted DB rows, but block.GenerateFileBlock re-reads each folder's mutable
// on-disk rule.json at projection time. A rule-only edit therefore produces no
// file/folder diff (rule.json is excluded from the inventory) yet a later crash/reconcile
// re-projects the SAME accepted DB under the NEW rule semantics — an unpinned, silent
// reinterpretation.
//
// I4F pins, per participating folder, a deterministic frozen classification-semantics
// basis (rules.ClassificationSemantics) at acceptance time and rebuilds/reconciles ONLY
// against that pinned basis. The basis rows are version-keyed in classification_semantics:
//   - the TARGET basis is written under target_version, atomically with the pending
//     transition, BEFORE the accepted DB rows mutate;
//   - the ACCEPTED basis is simply the rows under accepted_version; promotion of
//     target -> accepted is the accepted_version pointer flip in commitClean.
// Reconcile consumes the target basis while pending and the accepted basis while clean,
// with a target->accepted fallback so every crash window resolves to a frozen basis and
// never to the mutable on-disk rule.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"google.golang.org/protobuf/proto"

	"github.com/HeaInSeo/tori/block"
	"github.com/HeaInSeo/tori/protoio"
	pb "github.com/HeaInSeo/tori/protos/ichthys/v1"
	"github.com/HeaInSeo/tori/rules"
)

const (
	// originNative marks a basis frozen from the source rule.json at acceptance time.
	originNative = "native"
	// originMigrationAdopted marks a basis adopted at first-I4F upgrade for a legacy
	// clean snapshot, only after it was proven to reproduce the accepted projection.
	originMigrationAdopted = "migration-adopted"
)

// ErrFrozenBasisUnavailable indicates the frozen classification basis required to
// rebuild an accepted/target folder projection could not be resolved. It is
// fail-closed: callers must HOLD rather than fall back to the mutable on-disk rule.
var ErrFrozenBasisUnavailable = errors.New("frozen classification basis unavailable")

// folderBasis is one folder's frozen classification-semantics basis.
type folderBasis struct {
	Path       string
	RevisionID string
	Canonical  string
	Origin     string
}

// ensureSemanticsTable creates classification_semantics if absent. Idempotent and safe
// on every access, matching the snapshot_meta pattern, so pre-existing DBs work without
// a migration step.
func ensureSemanticsTable(ctx context.Context, e sqlDBTX) error {
	const create = `CREATE TABLE IF NOT EXISTS classification_semantics (
		version INTEGER NOT NULL,
		folder_path TEXT NOT NULL,
		revision_id TEXT NOT NULL,
		canonical TEXT NOT NULL,
		origin TEXT NOT NULL,
		PRIMARY KEY (version, folder_path)
	);`
	if _, err := e.ExecContext(ctx, create); err != nil {
		return fmt.Errorf("failed to ensure classification_semantics table: %w", err)
	}
	return nil
}

// pinSemanticsTx upserts one folder's frozen basis under version. It takes a sqlDBTX so
// it can (and must, for crash-atomicity) run inside the same transaction that records
// the pending transition.
func pinSemanticsTx(ctx context.Context, e sqlDBTX, version int64, b folderBasis) error {
	if err := ensureSemanticsTable(ctx, e); err != nil {
		return err
	}
	const upsert = `INSERT INTO classification_semantics (version, folder_path, revision_id, canonical, origin)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(version, folder_path) DO UPDATE SET
			revision_id = excluded.revision_id,
			canonical   = excluded.canonical,
			origin      = excluded.origin;`
	if _, err := e.ExecContext(ctx, upsert, version, b.Path, b.RevisionID, b.Canonical, b.Origin); err != nil {
		return fmt.Errorf("failed to pin classification semantics (v%d, %s): %w", version, b.Path, err)
	}
	return nil
}

// getSemantics returns the pinned basis for (version, folderPath), if present.
func getSemantics(ctx context.Context, e sqlDBTX, version int64, folderPath string) (folderBasis, bool, error) {
	if err := ensureSemanticsTable(ctx, e); err != nil {
		return folderBasis{}, false, err
	}
	var b folderBasis
	b.Path = folderPath
	err := e.QueryRowContext(ctx,
		"SELECT revision_id, canonical, origin FROM classification_semantics WHERE version = ? AND folder_path = ?",
		version, folderPath).Scan(&b.RevisionID, &b.Canonical, &b.Origin)
	if errors.Is(err, sql.ErrNoRows) {
		return folderBasis{}, false, nil
	}
	if err != nil {
		return folderBasis{}, false, fmt.Errorf("failed to read classification semantics (v%d, %s): %w", version, folderPath, err)
	}
	return b, true, nil
}

// countSemanticsAtVersion reports how many folder bases are pinned under version. Zero
// at accepted_version for a non-empty accepted DB signals a legacy pre-I4F snapshot.
func countSemanticsAtVersion(ctx context.Context, e sqlDBTX, version int64) (int, error) {
	if err := ensureSemanticsTable(ctx, e); err != nil {
		return 0, err
	}
	var n int
	if err := e.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM classification_semantics WHERE version = ?", version).Scan(&n); err != nil {
		return 0, fmt.Errorf("failed to count classification semantics at v%d: %w", version, err)
	}
	return n, nil
}

// resolveFrozenRuleSet returns the frozen RuleSet for folderPath, consuming the target
// basis while pending (falling back to the accepted basis for folders untouched by the
// in-flight target) and the accepted basis while clean. It NEVER reads on-disk
// rule.json. A folder with no pinned basis under either version fails closed.
func resolveFrozenRuleSet(ctx context.Context, e sqlDBTX, folderPath string, pending bool, acceptedVer, targetVer int64) (rules.RuleSet, error) {
	if pending {
		if b, ok, err := getSemantics(ctx, e, targetVer, folderPath); err != nil {
			return rules.RuleSet{}, err
		} else if ok {
			return rules.RuleSetFromCanonical(b.Canonical)
		}
	}
	if b, ok, err := getSemantics(ctx, e, acceptedVer, folderPath); err != nil {
		return rules.RuleSet{}, err
	} else if ok {
		return rules.RuleSetFromCanonical(b.Canonical)
	}
	return rules.RuleSet{}, fmt.Errorf("%w: folder %s (pending=%v accepted=v%d target=v%d)",
		ErrFrozenBasisUnavailable, folderPath, pending, acceptedVer, targetVer)
}

// freezeDiskBases loads, validates, canonicalizes and freezes each folder's on-disk
// rule.json exactly ONCE, returning a path->basis map, whether ALL froze successfully,
// and the first error. The SAME frozen values are used for both drift comparison and
// acceptance, so a rule cannot change between a "check" read and a separate "pin" read
// (TOCTOU): what is compared for drift is exactly what would be pinned. allOK==false
// means at least one folder's rule is missing/unreadable/invalid — the accept path
// treats that as a preflight HOLD before the accepted DB advances, while drift/unchanged
// runs simply skip the folders absent from the map (their frozen accepted basis stays
// authoritative).
func freezeDiskBases(folderPaths []string, origin string) (map[string]folderBasis, bool, error) {
	bases := make(map[string]folderBasis, len(folderPaths))
	allOK := true
	var firstErr error
	for _, p := range folderPaths {
		rs, err := rules.LoadRuleSetFromFile(p)
		if err != nil {
			allOK = false
			if firstErr == nil {
				firstErr = fmt.Errorf("rule preflight failed for %s: %w", p, err)
			}
			continue
		}
		canonical, revID, err := rules.FreezeRuleSet(rs)
		if err != nil {
			allOK = false
			if firstErr == nil {
				firstErr = fmt.Errorf("rule preflight failed for %s: %w", p, err)
			}
			continue
		}
		bases[p] = folderBasis{Path: p, RevisionID: revID, Canonical: canonical, Origin: origin}
	}
	return bases, allOK, firstErr
}

// uncoveredDiffFolder returns the path of the first added/modified folder in the diff
// whose rule basis was not frozen — i.e. it appeared on disk AFTER the basis was frozen,
// so the diff (a later disk enumeration) would add it to the accepted DB without a
// recoverable rule basis. Returns "" when every mutated (non-removal) folder is covered.
// Removals need no basis and are exempt. This guards the freeze→diff TOCTOU: the caller
// HOLDs and retries next sync (which re-freezes the current folder set).
func uncoveredDiffFolder(fDiff []FolderDiff, fChange []FileChange, frozen map[string]folderBasis) string {
	for i := range fDiff {
		if fDiff[i].ChangeType == "removed" {
			continue
		}
		if _, ok := frozen[fDiff[i].Path]; !ok {
			return fDiff[i].Path
		}
	}
	for i := range fChange {
		if fChange[i].ChangeType == "removed" {
			continue
		}
		if _, ok := frozen[fChange[i].Path]; !ok {
			return fChange[i].Path
		}
	}
	return ""
}

// basesSlice returns the frozen bases for the given paths, in the paths' order, skipping
// any path that failed to freeze (absent from the map).
func basesSlice(frozen map[string]folderBasis, paths []string) []folderBasis {
	out := make([]folderBasis, 0, len(paths))
	for _, p := range paths {
		if b, ok := frozen[p]; ok {
			out = append(out, b)
		}
	}
	return out
}

// detectSemanticsDrift compares each accepted folder's on-disk rule.json against its
// pinned accepted basis. It returns the first folder whose on-disk semantics differ
// (R1 -> R2 drift), so the caller can surface a classification-semantics/reclassification
// HOLD instead of ordinary "unchanged". A folder whose on-disk rule is missing/invalid
// is NOT treated as drift here (the frozen accepted basis remains authority and the
// accepted projection is intact); only a readable, semantically-different rule triggers
// drift. Folders without a pinned accepted basis are skipped (migration handles those).
//
// frozen is the once-frozen in-scope disk basis (freezeDiskBases). Drift compares the
// SAME frozen value that acceptance would pin, so no rule.json is re-read here (TOCTOU-
// free). inScope is the full set of current target folders. Handling by case for each
// accepted DB folder:
//   - out of the current scope (not in inScope): pruned by the diff path → skip;
//   - in scope, disk rule frozen (in frozen), pinned accepted basis present: compare
//     revisions → drift if different;
//   - in scope, disk rule frozen, NO pinned accepted basis: blind spot → HOLD;
//   - in scope, disk rule NOT frozen (unreadable/invalid), pinned accepted basis present:
//     the frozen accepted basis remains authoritative for projection → not drift, skip;
//   - in scope, disk rule NOT frozen, NO pinned accepted basis (TDI-I4F v0.3 F3): the
//     folder can be neither reproduced from an accepted basis nor re-frozen from disk →
//     fail-closed unverifiable-basis HOLD (must not be skipped as if out-of-scope, and
//     must not fall through to ordinary "unchanged" or a raw projection hard-error).
func detectSemanticsDrift(ctx context.Context, db *sql.DB, acceptedVer int64, inScope map[string]struct{}, frozen map[string]folderBasis) (driftFolder, from, to string, err error) {
	// Drift is defined relative to a pinned accepted basis. Skip only when NO basis is
	// pinned at acceptedVer — a true bootstrap or SaveFolders-seeded rows awaiting their
	// first accept, where the normal accept path will freeze and pin the basis. Do NOT key
	// this on acceptedVer==0: accepted_version is advanced only by commitClean, so a legacy
	// pre-I4F snapshot that migrateLegacyBasisIfNeeded adopts sits at version 0 yet HAS a
	// pinned basis at v0 that must still be checked for drift.
	//
	// ORDERING DEPENDENCY: SyncFolders MUST run migrateLegacyBasisIfNeeded before this. When
	// pinnedAtVer==0 with accepted inventory (not a SEED_ONLY bootstrap), migration has
	// already HELD fail-closed (datablock missing → HOLD; present → reproduce-or-HOLD), so
	// this early return is only reached for a genuine seed/bootstrap. If that ordering were
	// ever inverted, an all-basis-lost accepted snapshot could slip past drift detection.
	pinnedAtVer, err := countSemanticsAtVersion(ctx, db, acceptedVer)
	if err != nil {
		return "", "", "", err
	}
	if pinnedAtVer == 0 {
		return "", "", "", nil
	}
	folders, err := GetFoldersFromDB(db)
	if err != nil {
		return "", "", "", err
	}
	for _, f := range folders {
		if _, ok := inScope[f.Path]; !ok {
			continue // out-of-scope: pruned by the diff path, not a drift signal
		}
		pinned, pinnedOK, gErr := getSemantics(ctx, db, acceptedVer, f.Path)
		if gErr != nil {
			return "", "", "", gErr
		}
		cand, frozenOK := frozen[f.Path]
		if !frozenOK {
			// In scope but its on-disk rule could not be frozen/read/validated.
			if !pinnedOK {
				// F3: no accepted basis to fall back on and no usable disk rule → the folder
				// cannot be reproduced or re-frozen. Fail closed (from == "" marks "no
				// recoverable basis") rather than skipping it as out-of-scope.
				return f.Path, "", "", nil
			}
			// Has an accepted basis: projection rebuilds from it regardless of the
			// unreadable disk rule, so this is not drift.
			continue
		}
		if !pinnedOK {
			// An IN-SCOPE accepted folder with no pinned accepted basis is a blind spot:
			// migration has already run (pinnedCount>0) so it will not adopt it, and a
			// later data update would otherwise pin whatever rule is on disk, silently
			// reinterpreting the legacy inventory. This can arise if a prior scoped
			// migration/accept committed only a subset and the folder was excluded then
			// re-included before pruning. Fail closed: surface it as an unverifiable-basis
			// reclassification HOLD (from == "" marks "no recoverable basis").
			return f.Path, "", cand.RevisionID, nil
		}
		if cand.RevisionID != pinned.RevisionID {
			return f.Path, pinned.RevisionID, cand.RevisionID, nil
		}
	}
	return "", "", "", nil
}

// pendingBasisHold implements TDI-I4F v0.3 (F2): per-in-scope-folder validation that a
// pending recovery has a resolvable frozen basis for every accepted folder it must rebuild.
// It replaces the previous aggregate basis-count guard, which a crash mid-scoped-migration
// (target pins only a subset) could bypass, letting reconcile hit ErrFrozenBasisUnavailable
// and wedge on every retry. Here a missing per-folder basis is surfaced as a recoverable
// reclassification HOLD BEFORE reconcile runs, so no permanent hard-error wedge occurs.
//
// A folder is considered resolvable if a basis is pinned at either the target version
// (consumed while pending) or the accepted version (fallback). Out-of-scope folders are
// skipped: they are pruned by the normal diff/accept path (matching reconcile's
// buildFolderFilesFromDB(inScope) skip), so an excluded/vanished folder never wedges
// recovery. An empty in-scope inventory needs no basis and does not HOLD (an empty root's
// pending initial acceptance reconciles to an empty projection).
func pendingBasisHold(ctx context.Context, db *sql.DB, inScope map[string]struct{}, targetVer, acceptedVer int64) (hold bool, missing string, err error) {
	folders, err := GetFoldersFromDB(db)
	if err != nil {
		return false, "", err
	}
	for _, f := range folders {
		if _, ok := inScope[f.Path]; !ok {
			continue
		}
		if _, tOK, gErr := getSemantics(ctx, db, targetVer, f.Path); gErr != nil {
			return false, "", gErr
		} else if tOK {
			continue
		}
		if _, aOK, gErr := getSemantics(ctx, db, acceptedVer, f.Path); gErr != nil {
			return false, "", gErr
		} else if aOK {
			continue
		}
		return true, f.Path, nil
	}
	return false, "", nil
}

// migrateLegacyBasisIfNeeded bootstraps a frozen accepted basis for a legacy pre-I4F
// clean snapshot that has accepted DB rows but no pinned classification semantics.
//
// It is fail-safe: the current on-disk rule set is adopted as `migration-adopted`
// semantics ONLY when it reproduces the existing accepted compatibility projection
// (datablock.pb) exactly. If any accepted folder's rule is missing/invalid, or the
// regenerated projection does not match the stored one, or the stored projection cannot
// be read, it does NOT adopt and reports held=true so the caller HOLDs rather than
// guessing historical semantics.
func migrateLegacyBasisIfNeeded(ctx context.Context, db *sql.DB, rootPath string, acceptedVer int64, provenance string, inScope map[string]struct{}, frozen map[string]folderBasis) (held bool, reason string, err error) {
	folders, err := GetFoldersFromDB(db)
	if err != nil {
		return false, "", err
	}
	if len(folders) == 0 {
		return false, "", nil // nothing accepted → true bootstrap, no migration needed
	}
	pinnedCount, err := countSemanticsAtVersion(ctx, db, acceptedVer)
	if err != nil {
		return false, "", err
	}
	if pinnedCount > 0 {
		return false, "", nil // already has a pinned accepted basis (post-I4F)
	}

	// Non-empty inventory with no pinned basis. TDI-I4F v0.3 (F1): the durable acceptance
	// provenance — NOT accepted_version / datablock presence / row count — decides whether
	// this inventory was ever accepted:
	//   - SEED_ONLY: a genuinely fresh seed that has never been accepted → bootstrap; the
	//     normal accept path freezes and pins the basis and marks ACCEPTED.
	//   - ACCEPTED / UNKNOWN_LEGACY: this inventory is (or must be assumed to have been)
	//     accepted, so the current rules may only be adopted if they reproduce the existing
	//     accepted projection exactly. If datablock.pb is absent there is nothing to
	//     reproduce against → HOLD (never re-adopt current on-disk rules blindly, which
	//     would silently reclassify legacy data).
	if provenance == provenanceSeedOnly {
		return false, "", nil
	}
	_, statErr := os.Stat(filepath.Join(rootPath, "datablock.pb"))
	if os.IsNotExist(statErr) {
		return true, "legacy migration HOLD: inventory has no pinned classification basis and datablock.pb is missing; historical semantics are unverifiable (provenance=" + provenance + ")", nil
	}

	// Legacy: derive a candidate basis from the once-frozen in-scope rules. Only IN-SCOPE
	// present folders are reproduced/adopted; an out-of-scope (excluded) or physically
	// removed accepted folder is skipped by buildFolderFilesFromDB (complete=false) and is
	// pruned by the normal diff path, so it does not block migration of the remaining
	// inventory.
	folderFiles, _, err := buildFolderFilesFromDB(db, inScope)
	if err != nil {
		return false, "", err
	}
	candidates := make([]folderBasis, 0, len(folderFiles))
	expectedBlocks := make(map[string]*pb.FileBlock)
	for _, ff := range folderFiles {
		if len(ff) == 0 {
			continue
		}
		folderPath := ff[0]
		var names []string
		if len(ff) > 1 {
			names = ff[1:]
		}
		basis, ok := frozen[folderPath]
		if !ok {
			return true, fmt.Sprintf("legacy migration HOLD: rule basis for %s is missing/invalid; historical semantics unverifiable", folderPath), nil
		}
		rs, rErr := rules.RuleSetFromCanonical(basis.Canonical)
		if rErr != nil {
			return true, fmt.Sprintf("legacy migration HOLD: frozen basis for %s unusable: %v", folderPath, rErr), nil
		}
		fb, pErr := block.ProjectFileBlock(folderPath, names, rs)
		if pErr != nil {
			return true, fmt.Sprintf("legacy migration HOLD: cannot project %s with current rules: %v", folderPath, pErr), nil
		}
		expectedBlocks[folderPath] = fb
		candidates = append(candidates, folderBasis{
			Path: folderPath, RevisionID: basis.RevisionID, Canonical: basis.Canonical, Origin: originMigrationAdopted,
		})
	}

	// Verify the candidate basis reproduces the existing accepted projection exactly.
	if ok, why := candidateReproducesAcceptedProjection(rootPath, expectedBlocks); !ok {
		return true, "legacy migration HOLD: current rules do not reproduce the accepted projection (" + why + "); refusing to adopt", nil
	}

	// Reproduction proven → pin the adopted basis at the accepted version atomically.
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return false, "", fmt.Errorf("failed to begin migration tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	for _, b := range candidates {
		if pErr := pinSemanticsTx(ctx, tx, acceptedVer, b); pErr != nil {
			return false, "", pErr
		}
	}
	// The migration proved the inventory reproduces its accepted projection: it IS an
	// accepted snapshot with a migration-adopted basis. Record provenance=ACCEPTED in the
	// same transaction so a subsequent sync treats it as accepted (TDI-I4F v0.3 F1).
	if pErr := setProvenanceTx(ctx, tx, provenanceAccepted); pErr != nil {
		return false, "", pErr
	}
	if cErr := tx.Commit(); cErr != nil {
		return false, "", fmt.Errorf("failed to commit migration basis: %w", cErr)
	}
	committed = true
	logger.Warnf("TDI-I4F: adopted migration-time classification semantics for %d legacy folder(s) at v%d (reproduces accepted projection)", len(candidates), acceptedVer)
	return false, "", nil
}

// candidateReproducesAcceptedProjection reports whether regenerating each folder's
// FileBlock from candidate rules matches the FileBlocks stored in the accepted
// datablock.pb. DataBlock.UpdatedAt is ignored (it changes every generation); only the
// per-folder FileBlock content (block id, headers, rows) must match.
func candidateReproducesAcceptedProjection(rootPath string, expected map[string]*pb.FileBlock) (bool, string) {
	datablockPath := filepath.Join(rootPath, "datablock.pb")
	if _, statErr := os.Stat(datablockPath); statErr != nil {
		return false, "accepted datablock.pb not present"
	}
	stored, err := protoio.LoadDataBlock(datablockPath)
	if err != nil {
		return false, fmt.Sprintf("accepted datablock.pb unreadable: %v", err)
	}
	storedBlocks := make(map[string]*pb.FileBlock, len(stored.GetBlocks()))
	for _, b := range stored.GetBlocks() {
		storedBlocks[b.GetBlockId()] = b
	}
	// Subset match: every in-scope (expected) folder's regenerated block must match the
	// stored accepted block. Stored blocks for folders no longer in scope (excluded or
	// removed) are intentionally not required to match — they are being pruned by the diff
	// path and their basis is not adopted.
	for id, want := range expected {
		got, ok := storedBlocks[id]
		if !ok {
			return false, fmt.Sprintf("accepted projection missing block %s", id)
		}
		if !proto.Equal(want, got) {
			return false, fmt.Sprintf("block %s content differs", id)
		}
	}
	return true, ""
}
