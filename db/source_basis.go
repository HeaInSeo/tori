package db

// TDI-I2B: acceptance consumes the exact source envelope.
//
// I2A made the source envelope durable (SourceID / SourceRevision / access endpoint) but
// nothing read it back: an accepted compatibility snapshot did not say which source
// revision it represented, so a later scope edit (R1 → R2) was free to reinterpret the
// already accepted snapshot under R2 — during a crash/reconcile rebuild or a projection
// restore — as if it had always been R2.
//
// I2B pins, per snapshot version, the exact source basis the snapshot was accepted under:
//
//	snapshot_source_basis(version) = SourceID + SourceRevision + access endpoint
//
// next to the I4F classification basis of the same version. It is written under the
// target version in the SAME transaction that records the pending transition, so a crash
// can never leave a pending target without its source basis, and it is promoted to the
// accepted basis by the same accepted_version pointer flip as the classification basis.
//
// Consumption rules:
//   - A pinned basis is never rewritten. Adopting a newer SourceRevision is a NEW accepted
//     version with its own pin; the old version keeps R1.
//   - Recovery (pending reconcile) and projection restore rebuild under the observation
//     scope of the PINNED revision (read from source_revisions, never from current config),
//     together with the I4F frozen classification basis.
//   - A pin that does not resolve inside the current envelope (different SourceID, or a
//     revision/endpoint row that is not recorded for it) has unclear authority and HOLDs.
//   - An endpoint change alone (relocation, credential rotation, failover) proven by the
//     continuity witness continues the same SourceRevision; it does not need a new version.
//
// datablock.pb stays a compatibility projection. Nothing here mints a Generation,
// publication, or Auto-Run identity.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

const (
	// sourceBasisNative marks a basis pinned by the acceptance that produced the version.
	sourceBasisNative = "native"
	// sourceBasisCarried marks a basis copied forward from the accepted version by a
	// recovery that rebuilt the target under that accepted basis.
	sourceBasisCarried = "carried-forward"
	// sourceBasisLegacyAdopted marks a basis adopted for a pre-I2B accepted snapshot, only
	// after the current source revision was shown to reproduce its accepted inventory.
	sourceBasisLegacyAdopted = "legacy-adopted"
)

// errSourceBasisUnresolved reports a pinned source basis whose authority cannot be
// established against the current envelope. Callers HOLD rather than guess.
var errSourceBasisUnresolved = errors.New("pinned source basis does not resolve in the current source envelope")

// SnapshotSourceBasis is the exact source basis one snapshot version was accepted under.
type SnapshotSourceBasis struct {
	Version    int64
	SourceID   string
	RevisionID string
	EndpointID string
	// Origin is sourceBasisNative, sourceBasisCarried or sourceBasisLegacyAdopted.
	Origin string
}

// sameSource reports whether two bases name the same source and the same meaning. The
// endpoint is deliberately not compared: a proven endpoint change continues the revision.
func (b SnapshotSourceBasis) sameSource(o SnapshotSourceBasis) bool {
	return b.SourceID == o.SourceID && b.RevisionID == o.RevisionID
}

// currentSourceBasis is the basis the current observation runs under.
func currentSourceBasis(env SourceEnvelope) SnapshotSourceBasis {
	return SnapshotSourceBasis{
		SourceID:   env.SourceID,
		RevisionID: env.CurrentRevisionID,
		EndpointID: env.CurrentEndpointID,
		Origin:     sourceBasisNative,
	}
}

func ensureSourceBasisTable(ctx context.Context, e sqlDBTX) error {
	const create = `CREATE TABLE IF NOT EXISTS snapshot_source_basis (
		version INTEGER PRIMARY KEY,
		source_id TEXT NOT NULL,
		revision_id TEXT NOT NULL,
		endpoint_id TEXT NOT NULL,
		origin TEXT NOT NULL
	);`
	if _, err := e.ExecContext(ctx, create); err != nil {
		return fmt.Errorf("failed to ensure snapshot_source_basis table: %w", err)
	}
	return nil
}

// pinSourceBasisTx records the source basis for version. An existing pin for the same
// version is kept as it is (DO NOTHING): once a version names its source basis, nothing
// may rewrite it. A retried pending transition re-pins the same target version with the
// same basis, so this is idempotent for the legitimate caller.
func pinSourceBasisTx(ctx context.Context, e sqlDBTX, version int64, b SnapshotSourceBasis) error {
	if err := ensureSourceBasisTable(ctx, e); err != nil {
		return err
	}
	if b.SourceID == "" || b.RevisionID == "" || b.EndpointID == "" {
		return fmt.Errorf("refusing to pin an incomplete source basis at v%d: %+v", version, b)
	}
	if _, err := e.ExecContext(ctx,
		`INSERT INTO snapshot_source_basis (version, source_id, revision_id, endpoint_id, origin)
		 VALUES (?, ?, ?, ?, ?) ON CONFLICT(version) DO NOTHING;`,
		version, b.SourceID, b.RevisionID, b.EndpointID, b.Origin); err != nil {
		return fmt.Errorf("failed to pin source basis (v%d): %w", version, err)
	}
	return nil
}

// getSourceBasis returns the pinned source basis for version, if any.
func getSourceBasis(ctx context.Context, e sqlDBTX, version int64) (SnapshotSourceBasis, bool, error) {
	if err := ensureSourceBasisTable(ctx, e); err != nil {
		return SnapshotSourceBasis{}, false, err
	}
	b := SnapshotSourceBasis{Version: version}
	err := e.QueryRowContext(ctx,
		"SELECT source_id, revision_id, endpoint_id, origin FROM snapshot_source_basis WHERE version = ?",
		version).Scan(&b.SourceID, &b.RevisionID, &b.EndpointID, &b.Origin)
	if errors.Is(err, sql.ErrNoRows) {
		return SnapshotSourceBasis{}, false, nil
	}
	if err != nil {
		return SnapshotSourceBasis{}, false, fmt.Errorf("failed to read source basis (v%d): %w", version, err)
	}
	return b, true, nil
}

// GetAcceptedSourceBasis returns the source basis pinned for the currently accepted
// snapshot version, and whether one is pinned.
func GetAcceptedSourceBasis(ctx context.Context, db *sql.DB) (SnapshotSourceBasis, bool, error) {
	acceptedVer, err := metaGetInt(ctx, db, metaKeyAcceptedVersion)
	if err != nil {
		return SnapshotSourceBasis{}, false, err
	}
	return getSourceBasis(ctx, db, acceptedVer)
}

// GetSourceBasisAt returns the source basis pinned for one snapshot version.
func GetSourceBasisAt(ctx context.Context, db *sql.DB, version int64) (SnapshotSourceBasis, bool, error) {
	return getSourceBasis(ctx, db, version)
}

// validateSourceBasis checks that a pinned basis resolves inside the current envelope:
// same SourceID, and both its revision and its endpoint are recorded rows of that source.
// Anything else is an alias/union or a pin of unclear authority → errSourceBasisUnresolved.
func validateSourceBasis(ctx context.Context, db *sql.DB, env SourceEnvelope, b SnapshotSourceBasis) error {
	if b.SourceID != env.SourceID {
		return fmt.Errorf("%w: v%d pins SourceID %s, current source is %s",
			errSourceBasisUnresolved, b.Version, b.SourceID, env.SourceID)
	}
	if _, ok, err := SourceRevisionCanonical(ctx, db, b.SourceID, b.RevisionID); err != nil {
		return err
	} else if !ok {
		return fmt.Errorf("%w: v%d pins revision %s, which is not recorded for source %s",
			errSourceBasisUnresolved, b.Version, shortRev(b.RevisionID), b.SourceID)
	}
	var n int
	if err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM source_endpoints WHERE source_id = ? AND endpoint_id = ?",
		b.SourceID, b.EndpointID).Scan(&n); err != nil {
		return fmt.Errorf("failed to read source endpoint %s: %w", shortRev(b.EndpointID), err)
	}
	if n == 0 {
		return fmt.Errorf("%w: v%d pins endpoint %s, which is not recorded for source %s",
			errSourceBasisUnresolved, b.Version, shortRev(b.EndpointID), b.SourceID)
	}
	return nil
}

// revisionSemantics reads the frozen observation scope of one recorded revision. This is
// how recovery gets the scope a snapshot was accepted under without re-reading config.
func revisionSemantics(ctx context.Context, db *sql.DB, sourceID, revisionID string) (SourceSemantics, error) {
	canonical, ok, err := SourceRevisionCanonical(ctx, db, sourceID, revisionID)
	if err != nil {
		return SourceSemantics{}, err
	}
	if !ok {
		return SourceSemantics{}, fmt.Errorf("%w: revision %s is not recorded for source %s",
			errSourceBasisUnresolved, shortRev(revisionID), sourceID)
	}
	var s SourceSemantics
	if err := json.Unmarshal([]byte(canonical), &s); err != nil {
		return SourceSemantics{}, fmt.Errorf("%w: revision %s canonical form unreadable: %v",
			errSourceBasisUnresolved, shortRev(revisionID), err)
	}
	return s, nil
}

// scopeForRevision enumerates the in-scope folder set of rootPath under the frozen scope
// of one pinned revision.
func scopeForRevision(ctx context.Context, db *sql.DB, rootPath string, b SnapshotSourceBasis) (map[string]struct{}, error) {
	sem, err := revisionSemantics(ctx, db, b.SourceID, b.RevisionID)
	if err != nil {
		return nil, err
	}
	folders, err := GetSubFolders(rootPath, sem.FoldersExclusions)
	if err != nil {
		return nil, err
	}
	scope := make(map[string]struct{}, len(folders))
	for _, f := range folders {
		scope[f.Path] = struct{}{}
	}
	return scope, nil
}

// carrySourceBasisForward pins the accepted version's source basis at targetVer when a
// recovery rebuilt an unpinned target under it. The rebuild really used that basis, so
// recording it is a fact, not an inference. No-op when target == accepted, when the target
// is already pinned, or when the accepted version has no pin either (legacy: stays unpinned).
func carrySourceBasisForward(ctx context.Context, e sqlDBTX, targetVer, acceptedVer int64) error {
	if targetVer == acceptedVer {
		return nil
	}
	if _, ok, err := getSourceBasis(ctx, e, targetVer); err != nil || ok {
		return err
	}
	b, ok, err := getSourceBasis(ctx, e, acceptedVer)
	if err != nil || !ok {
		return err
	}
	b.Origin = sourceBasisCarried
	return pinSourceBasisTx(ctx, e, targetVer, b)
}
