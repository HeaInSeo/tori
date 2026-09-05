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

// freezeDiskBasis loads, validates, canonicalizes and freezes the on-disk rule.json for
// each folder, returning the pinned-ready target basis set. Any folder whose rule is
// missing/unreadable/invalid yields an error so the caller HOLDs BEFORE the accepted DB
// advances (rule preflight must precede DB mutation).
func freezeDiskBasis(folderPaths []string, origin string) ([]folderBasis, error) {
	bases := make([]folderBasis, 0, len(folderPaths))
	for _, p := range folderPaths {
		rs, err := rules.LoadRuleSetFromFile(p)
		if err != nil {
			return nil, fmt.Errorf("rule preflight failed for %s: %w", p, err)
		}
		canonical, revID, err := rules.FreezeRuleSet(rs)
		if err != nil {
			return nil, fmt.Errorf("rule preflight failed for %s: %w", p, err)
		}
		bases = append(bases, folderBasis{Path: p, RevisionID: revID, Canonical: canonical, Origin: origin})
	}
	return bases, nil
}

// diskRevisionID freezes the current on-disk rule.json of folderPath and returns its
// revision id. Used only to COMPARE against a pinned basis (drift detection); it is
// never used as projection authority.
func diskRevisionID(folderPath string) (string, error) {
	rs, err := rules.LoadRuleSetFromFile(folderPath)
	if err != nil {
		return "", err
	}
	_, revID, err := rules.FreezeRuleSet(rs)
	if err != nil {
		return "", err
	}
	return revID, nil
}

// detectSemanticsDrift compares each accepted folder's on-disk rule.json against its
// pinned accepted basis. It returns the first folder whose on-disk semantics differ
// (R1 -> R2 drift), so the caller can surface a classification-semantics/reclassification
// HOLD instead of ordinary "unchanged". A folder whose on-disk rule is missing/invalid
// is NOT treated as drift here (the frozen accepted basis remains authority and the
// accepted projection is intact); only a readable, semantically-different rule triggers
// drift. Folders without a pinned accepted basis are skipped (migration handles those).
func detectSemanticsDrift(ctx context.Context, db *sql.DB, acceptedVer int64) (driftFolder, from, to string, err error) {
	folders, err := GetFoldersFromDB(db)
	if err != nil {
		return "", "", "", err
	}
	for _, f := range folders {
		pinned, ok, gErr := getSemantics(ctx, db, acceptedVer, f.Path)
		if gErr != nil {
			return "", "", "", gErr
		}
		if !ok {
			continue
		}
		diskRev, dErr := diskRevisionID(f.Path)
		if dErr != nil {
			// On-disk rule unreadable/invalid: not a semantic-difference signal here.
			// The pinned accepted basis stays authoritative for projection.
			logger.Warnf("drift check: on-disk rule for %s unreadable/invalid; keeping accepted basis: %v", f.Path, dErr)
			continue
		}
		if diskRev != pinned.RevisionID {
			return f.Path, pinned.RevisionID, diskRev, nil
		}
	}
	return "", "", "", nil
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
func migrateLegacyBasisIfNeeded(ctx context.Context, db *sql.DB, rootPath string, acceptedVer int64) (held bool, reason string, err error) {
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

	// A legacy clean snapshot has an EXISTING accepted projection to reproduce. If there
	// is no datablock.pb, nothing has actually been accepted yet — this is a bootstrap
	// (possibly with SaveFolders-seeded DB rows), not a migration. The normal accept path
	// will freeze and pin the basis; do not HOLD here.
	if _, statErr := os.Stat(filepath.Join(rootPath, "datablock.pb")); os.IsNotExist(statErr) {
		return false, "", nil
	}

	// Legacy: derive a candidate basis from the current on-disk rules.
	folderFiles, complete, err := buildFolderFilesFromDB(db)
	if err != nil {
		return false, "", err
	}
	if !complete {
		return true, "legacy migration HOLD: an accepted folder is no longer present on disk; cannot verify historical semantics", nil
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
		rs, lErr := rules.LoadRuleSetFromFile(folderPath)
		if lErr != nil {
			return true, fmt.Sprintf("legacy migration HOLD: rule basis for %s unverifiable: %v", folderPath, lErr), nil
		}
		canonical, revID, fErr := rules.FreezeRuleSet(rs)
		if fErr != nil {
			return true, fmt.Sprintf("legacy migration HOLD: rule basis for %s unverifiable: %v", folderPath, fErr), nil
		}
		fb, pErr := block.ProjectFileBlock(folderPath, names, rs)
		if pErr != nil {
			return true, fmt.Sprintf("legacy migration HOLD: cannot project %s with current rules: %v", folderPath, pErr), nil
		}
		expectedBlocks[folderPath] = fb
		candidates = append(candidates, folderBasis{Path: folderPath, RevisionID: revID, Canonical: canonical, Origin: originMigrationAdopted})
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
	if len(storedBlocks) != len(expected) {
		return false, fmt.Sprintf("block count differs (accepted=%d, regenerated=%d)", len(storedBlocks), len(expected))
	}
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
