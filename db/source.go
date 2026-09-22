package db

// TDI-I2A: durable Source envelope — logical identity, immutable semantic revision,
// replaceable physical access endpoint.
//
// Before I2A, `RootDir` was everything at once: it named the source, it carried the
// observation scope, and it was the access route. That conflation means a perfectly
// ordinary operational event (a mount moves, a credential rotates, a failover promotes
// a replica) is indistinguishable from "this is a different source", while a genuine
// semantic change (the include/exclude scope is edited) is indistinguishable from "same
// source, same meaning". I2A separates the three concepts and persists them:
//
//	SourceID              stable logical observation-domain identity. Minted once and
//	                      durable. NEVER a path, mount, host, PVC or credential, and
//	                      never the continuity witness token (see errWitnessAsSourceID).
//	SourceRevision        freezes observation MEANING: the logical scope / include-
//	                      exclude semantics. Append-only: a semantic change mints a NEW
//	                      revision, it never edits an existing one in place.
//	SourceAccessEndpoint  the physical access route/capability actually used to reach
//	                      the source right now — the mounted RootDir plus an opaque
//	                      credential reference. Append-only history; relocation and
//	                      credential rotation move the CURRENT pointer and nothing else.
//
// Relationship to the I1/I10 continuity witness (acceptance.go): the `.tori_source_<token>`
// marker proves that the endpoint in front of us is still the same physical source we
// accepted from. That is continuity EVIDENCE for an endpoint; it is deliberately not an
// identity. A witness token is rotated/re-bootstrapped per source root, it lives in the
// source filesystem where anything may overwrite it, and it says nothing about observation
// meaning. So the witness gates whether an endpoint change may be adopted under the
// existing SourceID; it is never stored as the SourceID itself.
//
// Scope note: I2A only ESTABLISHES and maintains the envelope. Nothing here changes an
// acceptance decision — SyncFolders records the envelope and then proceeds exactly as
// before. Wiring acceptance to CONSUME the envelope is I2B and is deliberately not done
// here.

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
)

const (
	// sourceIDPrefix namespaces a minted SourceID. The continuity witness token is bare
	// hex (see newWitnessToken), so the prefix makes the two syntactically disjoint: a
	// witness token can never be mistaken for — or silently become — a SourceID.
	sourceIDPrefix = "src-"

	metaKeySourceCurrentRevision = "source_current_revision"
	metaKeySourceCurrentEndpoint = "source_current_endpoint"

	// originBootstrap marks an envelope minted on a DB with no accepted inventory: the
	// identity is at least as old as the data, so it describes the data honestly.
	originBootstrap = "bootstrap"
	// originLegacyAdopted marks an envelope minted for a pre-I2A DB that ALREADY held
	// accepted inventory. The identity is strictly younger than the data it names, which
	// source_envelope.inventory_predates_id records explicitly so no later reader can
	// mistake adoption for history.
	originLegacyAdopted = "legacy-adopted"
	// originScopeChange marks a revision minted because the semantic scope was edited on
	// an already-established source. It is distinct from originBootstrap so the first
	// revision of a source is never confused with a later reinterpretation of it.
	originScopeChange = "scope-change"
)

// errWitnessAsSourceID is returned when a source-continuity witness token is found
// standing in as the SourceID. The witness proves endpoint continuity only; promoting it
// to identity would make the logical source die and be reborn every time the marker is
// re-bootstrapped. Fail closed rather than carry a corrupted identity forward.
var errWitnessAsSourceID = errors.New("source witness token cannot be used as SourceID")

// SourceSemantics is the deterministic, canonical projection of the source-level
// configuration that defines observation MEANING. It is exactly the logical scope: which
// folders and files are in the observation domain.
//
// It deliberately excludes everything physical — RootDir, host, mount, credential — so an
// endpoint move can never look like a meaning change. Both exclusion lists are matched
// as "any pattern matches" (see rules.ListFilesExclude), so they are unordered SETS: the
// canonical form is sorted and de-duplicated, and merely reordering or repeating a
// pattern in config is therefore NOT a new revision.
type SourceSemantics struct {
	FoldersExclusions []string `json:"foldersExclusions"`
	FilesExclusions   []string `json:"filesExclusions"`
}

// CanonicalizeSourceSemantics normalizes the authored exclusion lists into the canonical
// set form. Nil and empty normalize identically so a nil-vs-empty difference cannot mint
// a spurious revision.
func CanonicalizeSourceSemantics(foldersExclusions, filesExclusions []string) SourceSemantics {
	return SourceSemantics{
		FoldersExclusions: canonPatternSet(foldersExclusions),
		FilesExclusions:   canonPatternSet(filesExclusions),
	}
}

// RevisionID is the semantic revision identity: sha256 (hex) over the canonical JSON.
// Content-addressed, so equal meaning is always the same revision and different meaning
// is always a different one. Internal identity only — not a public wire commitment.
func (s SourceSemantics) RevisionID() (canonicalJSON, revisionID string, err error) {
	b, err := json.Marshal(s)
	if err != nil {
		return "", "", fmt.Errorf("failed to canonicalize source semantics: %w", err)
	}
	sum := sha256.Sum256(b)
	return string(b), hex.EncodeToString(sum[:]), nil
}

// SourceAccessEndpoint is the physical access route/capability. CredentialRef is an
// opaque HANDLE (a key name, a secret reference), never the secret material itself;
// nothing here is ever hashed into a revision.
type SourceAccessEndpoint struct {
	RootDir       string `json:"rootDir"`
	CredentialRef string `json:"credentialRef"`
}

// EndpointID identifies one physical access route. Both the route and the capability
// used to traverse it participate, so a credential rotation is a new ENDPOINT — which is
// the point: it is recorded as an endpoint event and provably not a semantic one.
func (e SourceAccessEndpoint) EndpointID() (canonicalJSON, endpointID string, err error) {
	b, err := json.Marshal(e)
	if err != nil {
		return "", "", fmt.Errorf("failed to canonicalize source endpoint: %w", err)
	}
	sum := sha256.Sum256(b)
	return string(b), hex.EncodeToString(sum[:]), nil
}

// SourceEnvelope is the durable envelope as stored.
type SourceEnvelope struct {
	SourceID string
	// AdoptionOrigin is originBootstrap or originLegacyAdopted.
	AdoptionOrigin string
	// InventoryPredatesID is true when accepted inventory already existed at the moment
	// the SourceID was minted. It is the explicit record that this identity does NOT
	// reach back over that data: callers must not claim the SourceID existed historically.
	InventoryPredatesID bool
	// CurrentRevisionID is the semantic revision in force.
	CurrentRevisionID string
	// CurrentEndpointID is the physical access endpoint in force.
	CurrentEndpointID string
}

// SourceEnvelopeInput is the caller-supplied view of the source for one establish call.
type SourceEnvelopeInput struct {
	// RootDir is the currently mounted access route.
	RootDir string
	// CredentialRef is an opaque reference to the access credential (never the secret).
	CredentialRef string
	// FoldersExclusions / FilesExclusions are the source-level semantic scope.
	FoldersExclusions []string
	FilesExclusions   []string
}

// syncOptions holds the optional physical-access attributes a caller may supply to
// SyncFolders. Only access-endpoint attributes belong here; semantic scope stays in the
// explicit exclusion parameters so that a scope change is never expressible as an option.
type syncOptions struct {
	accessCredentialRef string
}

// SyncOption configures one optional access attribute for SyncFolders.
type SyncOption func(*syncOptions)

// WithAccessCredentialRef supplies the opaque reference to the credential used to reach
// the root (never the secret material). Rotating it moves the recorded access endpoint and
// leaves both SourceID and the semantic revision untouched.
func WithAccessCredentialRef(ref string) SyncOption {
	return func(o *syncOptions) { o.accessCredentialRef = ref }
}

// ensureSourceTables creates the envelope tables if absent. Idempotent and safe on every
// access, matching the snapshot_meta / classification_semantics pattern, so pre-existing
// DBs work without a migration step.
//
// source_revisions and source_endpoints are append-only by construction: their primary
// key IS the content hash of the row's canonical payload, so an "update" is impossible —
// the same content is the same row, and different content is a different row.
func ensureSourceTables(ctx context.Context, e sqlDBTX) error {
	const createEnvelope = `CREATE TABLE IF NOT EXISTS source_envelope (
		singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
		source_id TEXT NOT NULL,
		adoption_origin TEXT NOT NULL,
		inventory_predates_id INTEGER NOT NULL,
		adopted_at DATETIME DEFAULT CURRENT_TIMESTAMP NOT NULL
	);`
	if _, err := e.ExecContext(ctx, createEnvelope); err != nil {
		return fmt.Errorf("failed to ensure source_envelope table: %w", err)
	}
	const createRevisions = `CREATE TABLE IF NOT EXISTS source_revisions (
		source_id TEXT NOT NULL,
		revision_id TEXT NOT NULL,
		canonical TEXT NOT NULL,
		origin TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP NOT NULL,
		PRIMARY KEY (source_id, revision_id)
	);`
	if _, err := e.ExecContext(ctx, createRevisions); err != nil {
		return fmt.Errorf("failed to ensure source_revisions table: %w", err)
	}
	const createEndpoints = `CREATE TABLE IF NOT EXISTS source_endpoints (
		source_id TEXT NOT NULL,
		endpoint_id TEXT NOT NULL,
		canonical TEXT NOT NULL,
		first_seen_at DATETIME DEFAULT CURRENT_TIMESTAMP NOT NULL,
		PRIMARY KEY (source_id, endpoint_id)
	);`
	if _, err := e.ExecContext(ctx, createEndpoints); err != nil {
		return fmt.Errorf("failed to ensure source_endpoints table: %w", err)
	}
	return nil
}

// EnsureSourceEnvelope establishes or updates the durable source envelope and returns it.
//
// It is idempotent and crash-safe: the whole establish runs in ONE transaction, so a
// crash can only leave the previous complete envelope or the new complete envelope,
// never a SourceID without its revision/endpoint. Re-running with unchanged input is a
// no-op that returns the same envelope.
//
// Behavior:
//   - no envelope yet → mint one SourceID. inventoryPredatesID decides the adoption
//     origin: an empty accepted inventory is an honest bootstrap; a non-empty one is a
//     legacy adoption and is recorded as such.
//   - semantics changed → append a NEW revision and move the current-revision pointer.
//     The prior revision row is left exactly as it was.
//   - endpoint changed (relocation and/or credential rotation) → append a new endpoint
//     and move the current-endpoint pointer. The revision is untouched.
//
// Establishing the envelope never reads or writes the projection and never mints any
// Generation/Auto-Run/publication identity.
func EnsureSourceEnvelope(ctx context.Context, db *sql.DB, in SourceEnvelopeInput) (SourceEnvelope, error) {
	inventoryEmpty, err := acceptedInventoryEmpty(db)
	if err != nil {
		return SourceEnvelope{}, err
	}
	witness, _, err := metaGet(ctx, db, metaKeySourceWitness)
	if err != nil {
		return SourceEnvelope{}, err
	}

	revCanonical, revisionID, err := CanonicalizeSourceSemantics(in.FoldersExclusions, in.FilesExclusions).RevisionID()
	if err != nil {
		return SourceEnvelope{}, err
	}
	epCanonical, endpointID, err := SourceAccessEndpoint{RootDir: in.RootDir, CredentialRef: in.CredentialRef}.EndpointID()
	if err != nil {
		return SourceEnvelope{}, err
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return SourceEnvelope{}, fmt.Errorf("failed to begin source envelope tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	if err := ensureSourceTables(ctx, tx); err != nil {
		return SourceEnvelope{}, err
	}

	env, existed, err := getSourceEnvelopeTx(ctx, tx)
	if err != nil {
		return SourceEnvelope{}, err
	}
	if !existed {
		sourceID, mErr := mintSourceID()
		if mErr != nil {
			return SourceEnvelope{}, mErr
		}
		env = SourceEnvelope{
			SourceID:            sourceID,
			AdoptionOrigin:      originBootstrap,
			InventoryPredatesID: !inventoryEmpty,
		}
		if env.InventoryPredatesID {
			// Accepted inventory already existed when this identity was minted. Record the
			// adoption honestly: the SourceID names this inventory from now on, and makes no
			// claim about the period before now.
			env.AdoptionOrigin = originLegacyAdopted
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO source_envelope (singleton, source_id, adoption_origin, inventory_predates_id)
			 VALUES (1, ?, ?, ?);`,
			env.SourceID, env.AdoptionOrigin, boolToInt(env.InventoryPredatesID)); err != nil {
			return SourceEnvelope{}, fmt.Errorf("failed to record source envelope: %w", err)
		}
		logger.Warnf("TDI-I2A: minted SourceID %s (origin=%s, inventory_predates_id=%v)",
			env.SourceID, env.AdoptionOrigin, env.InventoryPredatesID)
	}

	// Identity guard, re-checked on EVERY call and not just at mint: a witness token
	// standing in as the SourceID means the identity is tied to a rotatable filesystem
	// marker. Fail closed instead of writing more state under a corrupted identity.
	if err := validateSourceIDNotWitness(env.SourceID, witness); err != nil {
		return SourceEnvelope{}, err
	}

	// Semantic revision. Content-addressed insert: an identical revision is a no-op, a
	// changed one is a new row. DO NOTHING (never DO UPDATE) is what makes an existing
	// revision immutable — its meaning can never be edited underneath a snapshot that
	// was accepted against it.
	revisionOrigin := originScopeChange
	if !existed {
		// First revision of this source: it describes the source as established, not a
		// later reinterpretation of it.
		revisionOrigin = originBootstrap
		if env.InventoryPredatesID {
			revisionOrigin = originLegacyAdopted
		}
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO source_revisions (source_id, revision_id, canonical, origin)
		 VALUES (?, ?, ?, ?) ON CONFLICT(source_id, revision_id) DO NOTHING;`,
		env.SourceID, revisionID, revCanonical, revisionOrigin); err != nil {
		return SourceEnvelope{}, fmt.Errorf("failed to record source revision: %w", err)
	}
	if env.CurrentRevisionID != revisionID {
		if env.CurrentRevisionID != "" {
			logger.Warnf("TDI-I2A: source %s semantic scope changed; revision %s → %s (prior revision retained)",
				env.SourceID, env.CurrentRevisionID, revisionID)
		}
		if err := metaSet(ctx, tx, metaKeySourceCurrentRevision, revisionID); err != nil {
			return SourceEnvelope{}, err
		}
		env.CurrentRevisionID = revisionID
	}

	// Physical endpoint. Relocation or credential rotation lands here and ONLY here, so
	// neither can reach the revision above.
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO source_endpoints (source_id, endpoint_id, canonical)
		 VALUES (?, ?, ?) ON CONFLICT(source_id, endpoint_id) DO NOTHING;`,
		env.SourceID, endpointID, epCanonical); err != nil {
		return SourceEnvelope{}, fmt.Errorf("failed to record source endpoint: %w", err)
	}
	if env.CurrentEndpointID != endpointID {
		if env.CurrentEndpointID != "" {
			logger.Warnf("TDI-I2A: source %s access endpoint changed; endpoint %s → %s (SourceID and revision %s unchanged)",
				env.SourceID, env.CurrentEndpointID, endpointID, env.CurrentRevisionID)
		}
		if err := metaSet(ctx, tx, metaKeySourceCurrentEndpoint, endpointID); err != nil {
			return SourceEnvelope{}, err
		}
		env.CurrentEndpointID = endpointID
	}

	if err := tx.Commit(); err != nil {
		return SourceEnvelope{}, fmt.Errorf("failed to commit source envelope: %w", err)
	}
	committed = true
	return env, nil
}

// GetSourceEnvelope returns the recorded envelope and whether one exists. It is
// read-only: a DB that has never established an envelope stays untouched.
func GetSourceEnvelope(ctx context.Context, db *sql.DB) (SourceEnvelope, bool, error) {
	if err := ensureSourceTables(ctx, db); err != nil {
		return SourceEnvelope{}, false, err
	}
	return getSourceEnvelopeTx(ctx, db)
}

// getSourceEnvelopeTx reads the envelope row plus the two current pointers. The caller
// must already have ensured the tables exist.
func getSourceEnvelopeTx(ctx context.Context, e sqlDBTX) (SourceEnvelope, bool, error) {
	var (
		env      SourceEnvelope
		predates int
	)
	err := e.QueryRowContext(ctx,
		"SELECT source_id, adoption_origin, inventory_predates_id FROM source_envelope WHERE singleton = 1").
		Scan(&env.SourceID, &env.AdoptionOrigin, &predates)
	if errors.Is(err, sql.ErrNoRows) {
		return SourceEnvelope{}, false, nil
	}
	if err != nil {
		return SourceEnvelope{}, false, fmt.Errorf("failed to read source envelope: %w", err)
	}
	env.InventoryPredatesID = predates != 0

	rev, _, err := metaGet(ctx, e, metaKeySourceCurrentRevision)
	if err != nil {
		return SourceEnvelope{}, false, err
	}
	env.CurrentRevisionID = rev

	ep, _, err := metaGet(ctx, e, metaKeySourceCurrentEndpoint)
	if err != nil {
		return SourceEnvelope{}, false, err
	}
	env.CurrentEndpointID = ep

	return env, true, nil
}

// SourceRevisionCanonical returns the frozen canonical semantics for one revision. This
// is how a later consumer reads the meaning a snapshot was accepted under WITHOUT
// re-reading mutable config — the same discipline classification.go applies to rule.json.
func SourceRevisionCanonical(ctx context.Context, db *sql.DB, sourceID, revisionID string) (string, bool, error) {
	if err := ensureSourceTables(ctx, db); err != nil {
		return "", false, err
	}
	var canonical string
	err := db.QueryRowContext(ctx,
		"SELECT canonical FROM source_revisions WHERE source_id = ? AND revision_id = ?",
		sourceID, revisionID).Scan(&canonical)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("failed to read source revision %s: %w", revisionID, err)
	}
	return canonical, true, nil
}

// CountSourceRevisions reports how many revisions exist for a source. Growth here is the
// observable proof that a semantic change appended rather than overwrote.
func CountSourceRevisions(ctx context.Context, db *sql.DB, sourceID string) (int, error) {
	if err := ensureSourceTables(ctx, db); err != nil {
		return 0, err
	}
	var n int
	if err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM source_revisions WHERE source_id = ?", sourceID).Scan(&n); err != nil {
		return 0, fmt.Errorf("failed to count revisions for source %s: %w", sourceID, err)
	}
	return n, nil
}

// CountSourceEndpoints reports how many distinct access endpoints have been recorded.
func CountSourceEndpoints(ctx context.Context, db *sql.DB, sourceID string) (int, error) {
	if err := ensureSourceTables(ctx, db); err != nil {
		return 0, err
	}
	var n int
	if err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM source_endpoints WHERE source_id = ?", sourceID).Scan(&n); err != nil {
		return 0, fmt.Errorf("failed to count endpoints for source %s: %w", sourceID, err)
	}
	return n, nil
}

// validateSourceIDNotWitness fails closed when the SourceID is (or is derived from) the
// source-continuity witness token. Both directions are checked because the failure we are
// preventing is conflation in either direction, and the prefix check keeps a bare witness
// token from ever satisfying the SourceID shape.
func validateSourceIDNotWitness(sourceID, witnessToken string) error {
	if sourceID == "" {
		return fmt.Errorf("source envelope has an empty SourceID")
	}
	if witnessToken != "" {
		if sourceID == witnessToken || sourceID == sourceIDPrefix+witnessToken {
			return fmt.Errorf("%w (token %s)", errWitnessAsSourceID, witnessToken)
		}
	}
	return nil
}

// mintSourceID returns a fresh logical source identity. It is random and namespaced: it
// derives from no path, host, mount, credential or witness, so nothing about where the
// source currently lives can leak into what it IS.
func mintSourceID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("failed to generate source id: %w", err)
	}
	return sourceIDPrefix + hex.EncodeToString(buf), nil
}

// canonPatternSet normalizes an exclusion pattern list to a sorted, de-duplicated set.
// Empty patterns are dropped: they match nothing and carry no meaning. A nil input
// returns an empty (non-nil) slice so nil and []string{} canonicalize identically.
func canonPatternSet(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, p := range in {
		if p == "" {
			continue
		}
		if _, dup := seen[p]; dup {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
