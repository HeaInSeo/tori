package db

import (
	"context"
	"database/sql"
)

// Acceptance provenance (TDI-I4F v0.3, F1). A durable classification of THIS DB's
// acceptance history, stored in snapshot_meta. It is the ONLY authority that distinguishes
// a fresh seed (never accepted) from a legacy inventory that WAS accepted before v0.3 —
// two states that are otherwise indistinguishable from accepted_version, datablock.pb
// presence, row count, or pinned-basis count. Central v0.3 F1 forbids inferring SEED_ONLY
// from any of those ambiguous negatives, so a pre-v0.3 DB with inventory defaults, fail
// closed, to UNKNOWN_LEGACY.
const metaKeyAcceptanceProvenance = "acceptance_provenance"

const (
	// provenanceUnknownLegacy: a pre-v0.3 DB (or any DB whose acceptance history is not
	// trustworthy). Fail-closed: it may become authoritative only if its current rules
	// reproduce an existing accepted projection exactly (migration-adopt); otherwise HOLD.
	// It must NEVER be treated as a fresh seed.
	provenanceUnknownLegacy = "unknown_legacy"
	// provenanceSeedOnly: a genuinely fresh post-v0.3 seed (SaveFolders on an empty DB)
	// that has never been accepted. A first clean acceptance may legitimately bootstrap it.
	provenanceSeedOnly = "seed_only"
	// provenanceAccepted: at least one clean acceptance has been committed (v0.3+, or a
	// legacy snapshot whose migration basis was proven); the accepted snapshot + pinned
	// basis are authoritative.
	provenanceAccepted = "accepted"
)

// getProvenance returns the recorded provenance and whether it was present.
func getProvenance(ctx context.Context, e sqlDBTX) (string, bool, error) {
	return metaGet(ctx, e, metaKeyAcceptanceProvenance)
}

// setProvenanceTx records provenance within the given executor/transaction. Callers that
// need crash-atomicity with the accepted/clean transition MUST pass the same *sql.Tx.
func setProvenanceTx(ctx context.Context, e sqlDBTX, p string) error {
	return metaSet(ctx, e, metaKeyAcceptanceProvenance, p)
}

// recordSeedProvenanceIfUnset marks a genuinely fresh seed as SEED_ONLY, but ONLY when no
// provenance has been recorded yet. It never clobbers ACCEPTED or UNKNOWN_LEGACY: a
// re-seed of an already-classified DB must not downgrade its history. The caller
// (SaveFolders) additionally gates this on the DB having been empty before seeding, so a
// legacy pre-v0.3 inventory is never mislabeled as a fresh seed.
func recordSeedProvenanceIfUnset(ctx context.Context, db *sql.DB) error {
	if _, ok, err := getProvenance(ctx, db); err != nil || ok {
		return err
	}
	return setProvenanceTx(ctx, db, provenanceSeedOnly)
}

// resolveProvenanceForSync returns the effective acceptance provenance for a sync,
// initializing a pre-v0.3 DB fail-closed. Rules (central v0.3 F1):
//   - provenance already recorded → use it verbatim;
//   - no provenance + empty inventory → a true unseeded bootstrap (nothing accepted to
//     protect); reported as SEED_ONLY-equivalent so the first accept can proceed, but NOT
//     persisted (SaveFolders owns the durable seed record);
//   - no provenance + non-empty inventory → a pre-v0.3 legacy DB; durably initialized to
//     UNKNOWN_LEGACY. SEED_ONLY is never inferred for a non-empty DB.
func resolveProvenanceForSync(ctx context.Context, db *sql.DB) (string, error) {
	p, ok, err := getProvenance(ctx, db)
	if err != nil {
		return "", err
	}
	if ok {
		return p, nil
	}
	folders, err := GetFoldersFromDB(db)
	if err != nil {
		return "", err
	}
	if len(folders) == 0 {
		return provenanceSeedOnly, nil
	}
	if err := setProvenanceTx(ctx, db, provenanceUnknownLegacy); err != nil {
		return "", err
	}
	logger.Warn("TDI-I4F v0.3: pre-v0.3 DB with inventory and no acceptance provenance → initialized UNKNOWN_LEGACY (fail-closed)")
	return provenanceUnknownLegacy, nil
}
