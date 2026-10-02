package db

// TDI-I3M: Atomic Publication Minimum.
//
// One durable publication authority that accepts an exact frozen semantic manifest and
// returns (or reconciles to) one immutable Generation. Three identities are kept apart:
//
//	PublicationOperationIdentity  the caller's explicit request ID. It exists so a retry
//	                              after ack loss finds the SAME logical operation. Reusing
//	                              it with different request semantics is a conflict.
//	PublicationSemanticManifest   the frozen meaning being published: exact source
//	                              identity + revision, per-folder classification revision,
//	                              stable subject coordinates and normalized member/role
//	                              facts with their integrity identity. ManifestID is the
//	                              sha256 of its canonical JSON.
//	Generation                    the immutable published result. Its ID derives from the
//	                              ManifestID only, so two different operations (or two
//	                              observation cycles) with an equal manifest converge to the
//	                              same Generation, and one Generation ID can never name
//	                              different content.
//
// Crash safety is two ordered transactions:
//
//  1. accept: record the manifest and the operation (state=accepted) together.
//  2. mint:   reconcile-before-mint — reuse the Generation already recorded for the
//     ManifestID, otherwise insert it with its subject rows — and complete the
//     operation, in one transaction.
//
// A crash after (1) leaves an accepted operation with no result. The retry of that same
// operation resumes at (2), which reuses any existing Generation, so an ambiguous prior
// acceptance never mints a second Generation.
//
// Out of scope here (TDI-I3M Forbidden): supersession/reclassification (I3C), Auto-Run
// admission or any Run creation, Sori promotion, HA/fencing, UI. datablock.pb / FileBlock
// stay a compatibility projection and are neither read nor written here.

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/HeaInSeo/tori/rules"
)

const (
	generationIDPrefix = "gen-"

	publicationOpAccepted  = "accepted"
	publicationOpCompleted = "completed"
)

// ErrPublicationConflict is returned when an operation ID is reused with different
// immutable request semantics. Nothing is written.
var ErrPublicationConflict = errors.New("publication operation ID reused with different semantics")

// ErrPublicationManifestInvalid is returned for a manifest that cannot be frozen.
var ErrPublicationManifestInvalid = errors.New("invalid publication semantic manifest")

// ErrPublicationNotAccepted is returned when a manifest is requested from a snapshot
// that is not a clean accepted snapshot with a pinned source and classification basis.
var ErrPublicationNotAccepted = errors.New("no clean accepted snapshot to publish")

// ManifestMember is one subject member fact. Integrity is the authoritative integrity
// identity of the member for the supported profile.
type ManifestMember struct {
	ObservedKey    string `json:"observedKey"`
	NormalizedRole string `json:"normalizedRole"`
	FileName       string `json:"fileName"`
	Integrity      string `json:"integrity"`
}

// ManifestSubject is one subject under its I11A stable coordinate.
type ManifestSubject struct {
	SubjectKey string           `json:"subjectKey"`
	Components []string         `json:"components"`
	Members    []ManifestMember `json:"members"`
}

// ManifestFolder groups the subjects of one source folder. Path is relative to the
// source root, so an endpoint relocation cannot change the manifest.
type ManifestFolder struct {
	Path                     string            `json:"path"`
	ClassificationRevisionID string            `json:"classificationRevisionId"`
	Subjects                 []ManifestSubject `json:"subjects"`
}

// canonicalFolderPath reports whether p is the one spelling of a root-relative folder
// path: slash-separated, not absolute, no backslash, unchanged by path.Clean, and with no
// "." or ".." segment. "." alone is the source root itself. Any other spelling of the
// same folder (for example "./runA" or "runA/") would hash to a different ManifestID.
func canonicalFolderPath(p string) bool {
	if p == "." {
		return true
	}
	if p == "" || strings.HasPrefix(p, "/") || strings.Contains(p, `\`) || path.Clean(p) != p {
		return false
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == "." || seg == ".." {
			return false
		}
	}
	return true
}

// PublicationSemanticManifest is the exact frozen meaning of one publication.
type PublicationSemanticManifest struct {
	SourceID         string           `json:"sourceId"`
	SourceRevisionID string           `json:"sourceRevisionId"`
	Folders          []ManifestFolder `json:"folders"`
}

// canonical returns a sorted deep copy and validates it. Folders sort by Path, subjects
// by SubjectKey, members by ObservedKey, so list order in the input never matters.
func (m PublicationSemanticManifest) canonical() (PublicationSemanticManifest, error) {
	if m.SourceID == "" || m.SourceRevisionID == "" {
		return PublicationSemanticManifest{}, fmt.Errorf("%w: source identity/revision required", ErrPublicationManifestInvalid)
	}
	out := PublicationSemanticManifest{SourceID: m.SourceID, SourceRevisionID: m.SourceRevisionID}
	subjects := 0
	seenFolder := make(map[string]struct{}, len(m.Folders))
	for _, f := range m.Folders {
		if f.Path == "" || f.ClassificationRevisionID == "" {
			return PublicationSemanticManifest{}, fmt.Errorf("%w: folder path/classification revision required", ErrPublicationManifestInvalid)
		}
		if !canonicalFolderPath(f.Path) {
			return PublicationSemanticManifest{}, fmt.Errorf("%w: folder path %q is not a clean root-relative slash path", ErrPublicationManifestInvalid, f.Path)
		}
		if _, dup := seenFolder[f.Path]; dup {
			return PublicationSemanticManifest{}, fmt.Errorf("%w: duplicate folder %s", ErrPublicationManifestInvalid, f.Path)
		}
		seenFolder[f.Path] = struct{}{}
		cf := ManifestFolder{Path: f.Path, ClassificationRevisionID: f.ClassificationRevisionID, Subjects: []ManifestSubject{}}
		seenSubject := make(map[string]struct{}, len(f.Subjects))
		for _, s := range f.Subjects {
			if s.SubjectKey == "" || len(s.Members) == 0 {
				return PublicationSemanticManifest{}, fmt.Errorf("%w: subject key and members required (%s)", ErrPublicationManifestInvalid, f.Path)
			}
			if _, dup := seenSubject[s.SubjectKey]; dup {
				return PublicationSemanticManifest{}, fmt.Errorf("%w: duplicate subject %s in %s", ErrPublicationManifestInvalid, s.SubjectKey, f.Path)
			}
			seenSubject[s.SubjectKey] = struct{}{}
			cs := ManifestSubject{
				SubjectKey: s.SubjectKey,
				Components: append([]string{}, s.Components...),
				Members:    append([]ManifestMember{}, s.Members...),
			}
			for _, mem := range cs.Members {
				if mem.ObservedKey == "" || mem.FileName == "" || mem.Integrity == "" {
					return PublicationSemanticManifest{}, fmt.Errorf("%w: member observed key, file name and integrity required (subject %s in %s)",
						ErrPublicationManifestInvalid, s.SubjectKey, f.Path)
				}
			}
			sort.Slice(cs.Members, func(i, j int) bool { return cs.Members[i].ObservedKey < cs.Members[j].ObservedKey })
			for i := 1; i < len(cs.Members); i++ {
				if cs.Members[i].ObservedKey == cs.Members[i-1].ObservedKey {
					return PublicationSemanticManifest{}, fmt.Errorf("%w: duplicate member %s in subject %s", ErrPublicationManifestInvalid, cs.Members[i].ObservedKey, s.SubjectKey)
				}
			}
			cf.Subjects = append(cf.Subjects, cs)
			subjects++
		}
		sort.Slice(cf.Subjects, func(i, j int) bool { return cf.Subjects[i].SubjectKey < cf.Subjects[j].SubjectKey })
		out.Folders = append(out.Folders, cf)
	}
	if subjects == 0 {
		return PublicationSemanticManifest{}, fmt.Errorf("%w: a Generation needs at least one subject", ErrPublicationManifestInvalid)
	}
	sort.Slice(out.Folders, func(i, j int) bool { return out.Folders[i].Path < out.Folders[j].Path })
	return out, nil
}

// ManifestID returns the canonical JSON and its sha256 (hex). Equal meaning always gives
// the same ID, whatever the input order.
func (m PublicationSemanticManifest) ManifestID() (canonicalJSON, manifestID string, err error) {
	c, err := m.canonical()
	if err != nil {
		return "", "", err
	}
	b, err := json.Marshal(c)
	if err != nil {
		return "", "", fmt.Errorf("failed to canonicalize publication manifest: %w", err)
	}
	sum := sha256.Sum256(b)
	return string(b), hex.EncodeToString(sum[:]), nil
}

// PublicationRequest is one explicit publication request.
type PublicationRequest struct {
	OperationID string
	Manifest    PublicationSemanticManifest
}

// PublicationResult is the authoritative result of a publication request.
type PublicationResult struct {
	OperationID  string
	ManifestID   string
	GenerationID string
	// Reconciled is true when the request resolved to an operation that was already
	// accepted (a retry), rather than accepting a new one.
	Reconciled bool
}

// publicationCrashHookForTest, when set by a test, is called at each named persistence
// boundary. A non-nil error aborts Publish at that point as a simulated crash; an open
// transaction is rolled back. Always nil outside tests.
var publicationCrashHookForTest func(boundary string) error

const (
	boundaryAfterAccept   = "after-accept-commit"
	boundaryBeforeMintEnd = "before-mint-commit"
	boundaryAfterMint     = "after-mint-commit"
)

func publicationCrash(boundary string) error {
	if publicationCrashHookForTest == nil {
		return nil
	}
	return publicationCrashHookForTest(boundary)
}

// ensurePublicationTables creates the publication tables and their immutability triggers
// if absent. They are created only here, never by SyncFolders, so observation and
// acceptance alone mint no publication identity.
func ensurePublicationTables(ctx context.Context, e sqlDBTX) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS publication_manifests (
			manifest_id TEXT PRIMARY KEY,
			canonical TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS publication_generations (
			generation_id TEXT PRIMARY KEY,
			manifest_id TEXT NOT NULL UNIQUE,
			subject_count INTEGER NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS publication_generation_subjects (
			generation_id TEXT NOT NULL,
			folder_path TEXT NOT NULL,
			subject_key TEXT NOT NULL,
			classification_revision_id TEXT NOT NULL,
			subject_canonical TEXT NOT NULL,
			PRIMARY KEY (generation_id, folder_path, subject_key)
		);`,
		`CREATE TABLE IF NOT EXISTS publication_operations (
			operation_id TEXT PRIMARY KEY,
			manifest_id TEXT NOT NULL,
			state TEXT NOT NULL,
			generation_id TEXT
		);`,
	}
	// Accepted manifests and Generations are immutable: no row may change or disappear.
	for _, table := range []string{"publication_manifests", "publication_generations", "publication_generation_subjects"} {
		stmts = append(stmts,
			fmt.Sprintf(`CREATE TRIGGER IF NOT EXISTS %[1]s_no_update BEFORE UPDATE ON %[1]s
				BEGIN SELECT RAISE(ABORT, '%[1]s is immutable'); END;`, table),
			fmt.Sprintf(`CREATE TRIGGER IF NOT EXISTS %[1]s_no_delete BEFORE DELETE ON %[1]s
				BEGIN SELECT RAISE(ABORT, '%[1]s is immutable'); END;`, table))
	}
	// Membership is fixed once the Generation row exists (subjects are inserted first).
	// An operation's request semantics are fixed; only accepted→completed may happen, once.
	stmts = append(stmts,
		`CREATE TRIGGER IF NOT EXISTS publication_generation_subjects_sealed BEFORE INSERT ON publication_generation_subjects
			WHEN EXISTS (SELECT 1 FROM publication_generations WHERE generation_id = NEW.generation_id)
			BEGIN SELECT RAISE(ABORT, 'generation membership is immutable'); END;`,
		`CREATE TRIGGER IF NOT EXISTS publication_operations_fixed BEFORE UPDATE ON publication_operations
			WHEN NEW.operation_id IS NOT OLD.operation_id
			  OR NEW.manifest_id IS NOT OLD.manifest_id
			  OR OLD.state <> 'accepted'
			  OR NEW.state <> 'completed'
			  OR NEW.generation_id IS NULL
			BEGIN SELECT RAISE(ABORT, 'publication operation is immutable once accepted'); END;`,
		`CREATE TRIGGER IF NOT EXISTS publication_operations_no_delete BEFORE DELETE ON publication_operations
			BEGIN SELECT RAISE(ABORT, 'publication operation is immutable'); END;`)
	for _, s := range stmts {
		if _, err := e.ExecContext(ctx, s); err != nil {
			return fmt.Errorf("failed to ensure publication tables: %w", err)
		}
	}
	return nil
}

// Publish accepts req and returns its immutable Generation. It is idempotent per
// operation ID: a retry with the same semantics returns the same Generation, a reuse with
// different semantics fails with ErrPublicationConflict. Publish creates no Run and
// touches no projection file.
func Publish(ctx context.Context, db *sql.DB, req PublicationRequest) (PublicationResult, error) {
	if req.OperationID == "" {
		return PublicationResult{}, fmt.Errorf("%w: operation ID required", ErrPublicationManifestInvalid)
	}
	canonicalJSON, manifestID, err := req.Manifest.ManifestID()
	if err != nil {
		return PublicationResult{}, err
	}
	res := PublicationResult{OperationID: req.OperationID, ManifestID: manifestID}

	// (1) accept: manifest + operation, one transaction.
	state, genID, existed, err := acceptPublicationOperation(ctx, db, req.OperationID, manifestID, canonicalJSON)
	if err != nil {
		return PublicationResult{}, err
	}
	res.Reconciled = existed
	if state == publicationOpCompleted {
		res.GenerationID = genID
		return res, nil
	}
	if err := publicationCrash(boundaryAfterAccept); err != nil {
		return PublicationResult{}, err
	}

	// (2) reconcile-before-mint + complete, one transaction.
	c, err := req.Manifest.canonical()
	if err != nil {
		return PublicationResult{}, err
	}
	genID, err = mintOrReconcileGeneration(ctx, db, req.OperationID, manifestID, c)
	if err != nil {
		return PublicationResult{}, err
	}
	if err := publicationCrash(boundaryAfterMint); err != nil {
		return PublicationResult{}, err
	}
	res.GenerationID = genID
	return res, nil
}

func acceptPublicationOperation(ctx context.Context, db *sql.DB, opID, manifestID, canonicalJSON string) (state, genID string, existed bool, err error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return "", "", false, fmt.Errorf("failed to begin publication accept tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	if err := ensurePublicationTables(ctx, tx); err != nil {
		return "", "", false, err
	}
	var (
		priorManifest string
		priorGen      sql.NullString
	)
	err = tx.QueryRowContext(ctx,
		"SELECT manifest_id, state, generation_id FROM publication_operations WHERE operation_id = ?", opID).
		Scan(&priorManifest, &state, &priorGen)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO publication_manifests (manifest_id, canonical) VALUES (?, ?)
			 ON CONFLICT(manifest_id) DO NOTHING;`, manifestID, canonicalJSON); err != nil {
			return "", "", false, fmt.Errorf("failed to record publication manifest: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO publication_operations (operation_id, manifest_id, state) VALUES (?, ?, ?);",
			opID, manifestID, publicationOpAccepted); err != nil {
			return "", "", false, fmt.Errorf("failed to record publication operation: %w", err)
		}
		state = publicationOpAccepted
	case err != nil:
		return "", "", false, fmt.Errorf("failed to read publication operation %s: %w", opID, err)
	default:
		existed = true
		if priorManifest != manifestID {
			return "", "", false, fmt.Errorf("%w: operation %s was accepted for manifest %s, not %s",
				ErrPublicationConflict, opID, shortRev(priorManifest), shortRev(manifestID))
		}
		genID = priorGen.String
	}
	if err := tx.Commit(); err != nil {
		return "", "", false, fmt.Errorf("failed to commit publication accept: %w", err)
	}
	committed = true
	return state, genID, existed, nil
}

func mintOrReconcileGeneration(ctx context.Context, db *sql.DB, opID, manifestID string, m PublicationSemanticManifest) (string, error) {
	genID := generationIDPrefix + manifestID
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("failed to begin publication mint tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	if err := ensurePublicationTables(ctx, tx); err != nil {
		return "", err
	}
	var existing string
	err = tx.QueryRowContext(ctx,
		"SELECT generation_id FROM publication_generations WHERE manifest_id = ?", manifestID).Scan(&existing)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		subjects := 0
		for _, f := range m.Folders {
			for _, s := range f.Subjects {
				b, mErr := json.Marshal(s)
				if mErr != nil {
					return "", fmt.Errorf("failed to canonicalize subject %s: %w", s.SubjectKey, mErr)
				}
				if _, err := tx.ExecContext(ctx,
					`INSERT INTO publication_generation_subjects
					 (generation_id, folder_path, subject_key, classification_revision_id, subject_canonical)
					 VALUES (?, ?, ?, ?, ?);`,
					genID, f.Path, s.SubjectKey, f.ClassificationRevisionID, string(b)); err != nil {
					return "", fmt.Errorf("failed to record generation subject: %w", err)
				}
				subjects++
			}
		}
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO publication_generations (generation_id, manifest_id, subject_count) VALUES (?, ?, ?);",
			genID, manifestID, subjects); err != nil {
			return "", fmt.Errorf("failed to record generation: %w", err)
		}
	case err != nil:
		return "", fmt.Errorf("failed to read generation for manifest %s: %w", shortRev(manifestID), err)
	default:
		genID = existing
	}
	if _, err := tx.ExecContext(ctx,
		"UPDATE publication_operations SET state = ?, generation_id = ? WHERE operation_id = ? AND state = ?;",
		publicationOpCompleted, genID, opID, publicationOpAccepted); err != nil {
		return "", fmt.Errorf("failed to complete publication operation %s: %w", opID, err)
	}
	if err := publicationCrash(boundaryBeforeMintEnd); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("failed to commit publication mint: %w", err)
	}
	committed = true
	return genID, nil
}

// GenerationSubject is one immutable subject row of a Generation. Later consumers
// address a subject as GenerationID + FolderPath + SubjectKey, never by row number,
// path ordinal or batch position.
type GenerationSubject struct {
	FolderPath               string
	SubjectKey               string
	ClassificationRevisionID string
	SubjectCanonical         string
}

// Generation is the immutable published result.
type Generation struct {
	GenerationID string
	ManifestID   string
	Subjects     []GenerationSubject
}

// GetGeneration reads one Generation and its subjects, sorted by folder and subject key.
func GetGeneration(ctx context.Context, db *sql.DB, generationID string) (Generation, bool, error) {
	if err := ensurePublicationTables(ctx, db); err != nil {
		return Generation{}, false, err
	}
	g := Generation{GenerationID: generationID}
	err := db.QueryRowContext(ctx,
		"SELECT manifest_id FROM publication_generations WHERE generation_id = ?", generationID).Scan(&g.ManifestID)
	if errors.Is(err, sql.ErrNoRows) {
		return Generation{}, false, nil
	}
	if err != nil {
		return Generation{}, false, fmt.Errorf("failed to read generation %s: %w", generationID, err)
	}
	rows, err := db.QueryContext(ctx,
		`SELECT folder_path, subject_key, classification_revision_id, subject_canonical
		 FROM publication_generation_subjects WHERE generation_id = ?
		 ORDER BY folder_path, subject_key`, generationID)
	if err != nil {
		return Generation{}, false, fmt.Errorf("failed to read generation subjects %s: %w", generationID, err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var s GenerationSubject
		if err := rows.Scan(&s.FolderPath, &s.SubjectKey, &s.ClassificationRevisionID, &s.SubjectCanonical); err != nil {
			return Generation{}, false, fmt.Errorf("failed to scan generation subject: %w", err)
		}
		g.Subjects = append(g.Subjects, s)
	}
	if err := rows.Err(); err != nil {
		return Generation{}, false, fmt.Errorf("failed to read generation subjects %s: %w", generationID, err)
	}
	return g, true, nil
}

// BuildAcceptedPublicationManifest freezes the current CLEAN accepted snapshot into a
// semantic manifest. It reads only accepted state: the source basis and classification
// basis pinned at accepted_version and the accepted inventory rows. It never reads the
// on-disk rule.json or datablock.pb. rootPath must be the access root recorded for the
// endpoint the accepted source basis pins (compared after filepath.Clean); any other path,
// including an ancestor of that root, is refused, so one accepted snapshot has exactly one
// set of root-relative folder paths and therefore one ManifestID. Conflicted (I12)
// subjects are not published.
//
// Integrity: the local POSIX profile records no content digest, so a member's integrity
// identity is "size:<bytes>" plus the row's accepted change epoch (accepted_epoch.go). A
// same-length content change accepted as "modified" therefore yields a distinct manifest.
//
// Snapshot: acceptance state, accepted_version, the pinned bases, folders, files and epochs
// are all read inside ONE database transaction, so a SyncFolders commit that lands while the
// manifest is being built can never mix version-N bases with a version-N+1 inventory.
func BuildAcceptedPublicationManifest(ctx context.Context, db *sql.DB, rootPath string) (PublicationSemanticManifest, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return PublicationSemanticManifest{}, fmt.Errorf("failed to begin accepted snapshot read: %w", err)
	}
	defer func() {
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			logger.Warnf("accepted snapshot read rollback failed: %v", rbErr)
		}
	}()
	return buildAcceptedPublicationManifestTx(ctx, tx, rootPath)
}

func buildAcceptedPublicationManifestTx(ctx context.Context, tx *sql.Tx, rootPath string) (PublicationSemanticManifest, error) {
	state, ok, err := metaGet(ctx, tx, metaKeyAcceptanceState)
	if err != nil {
		return PublicationSemanticManifest{}, err
	}
	if !ok {
		state = acceptanceClean
	}
	acceptedVer, err := metaGetInt(ctx, tx, metaKeyAcceptedVersion)
	if err != nil {
		return PublicationSemanticManifest{}, err
	}
	if state != acceptanceClean || acceptedVer == 0 {
		return PublicationSemanticManifest{}, fmt.Errorf("%w (state=%s accepted=v%d)", ErrPublicationNotAccepted, state, acceptedVer)
	}
	if publicationSnapshotHookForTest != nil {
		publicationSnapshotHookForTest()
	}
	src, ok, err := getSourceBasis(ctx, tx, acceptedVer)
	if err != nil {
		return PublicationSemanticManifest{}, err
	}
	if !ok {
		return PublicationSemanticManifest{}, fmt.Errorf("%w: no source basis pinned at v%d", ErrPublicationNotAccepted, acceptedVer)
	}
	acceptedRoot, err := recordedEndpointRoot(ctx, tx, src)
	if err != nil {
		return PublicationSemanticManifest{}, err
	}
	if filepath.Clean(rootPath) != acceptedRoot {
		return PublicationSemanticManifest{}, fmt.Errorf("%w: root %s is not the access root %s recorded for the snapshot accepted at v%d",
			ErrPublicationNotAccepted, rootPath, acceptedRoot, acceptedVer)
	}
	if err := ensureAcceptedEpochTable(ctx, tx); err != nil {
		return PublicationSemanticManifest{}, err
	}
	folders, err := acceptedFolderPathsTx(ctx, tx)
	if err != nil {
		return PublicationSemanticManifest{}, err
	}
	m := PublicationSemanticManifest{SourceID: src.SourceID, SourceRevisionID: src.RevisionID}
	for _, folderPath := range folders {
		basis, ok, err := getSemantics(ctx, tx, acceptedVer, folderPath)
		if err != nil {
			return PublicationSemanticManifest{}, err
		}
		if !ok {
			return PublicationSemanticManifest{}, fmt.Errorf("%w: %v (folder %s at v%d)",
				ErrPublicationNotAccepted, ErrFrozenBasisUnavailable, folderPath, acceptedVer)
		}
		ruleSet, err := rules.RuleSetFromCanonical(basis.Canonical)
		if err != nil {
			return PublicationSemanticManifest{}, err
		}
		files, err := acceptedFileIntegrityTx(ctx, tx, folderPath)
		if err != nil {
			return PublicationSemanticManifest{}, err
		}
		names := make([]string, 0, len(files))
		for name := range files {
			names = append(names, name)
		}
		sort.Strings(names)
		rel, err := filepath.Rel(acceptedRoot, folderPath)
		if err != nil || !pathWithinRoot(folderPath, acceptedRoot) {
			return PublicationSemanticManifest{}, fmt.Errorf("%w: accepted folder %s is outside root %s",
				ErrPublicationNotAccepted, folderPath, acceptedRoot)
		}
		subjects, _ := rules.PublicationSubjects(names, ruleSet)
		mf := ManifestFolder{Path: filepath.ToSlash(rel), ClassificationRevisionID: basis.RevisionID}
		for _, s := range subjects {
			ms := ManifestSubject{SubjectKey: s.SubjectKey, Components: s.Components}
			for _, mem := range s.Members {
				ms.Members = append(ms.Members, ManifestMember{
					ObservedKey:    mem.ObservedKey,
					NormalizedRole: mem.NormalizedRole,
					FileName:       mem.FileName,
					Integrity:      files[mem.FileName],
				})
			}
			mf.Subjects = append(mf.Subjects, ms)
		}
		m.Folders = append(m.Folders, mf)
	}
	return m, nil
}

// recordedEndpointRoot returns the cleaned access root of the endpoint that src pins. A
// basis whose endpoint row is missing or unreadable is refused rather than trusted, and so
// is a row whose content no longer hashes to the pinned EndpointID: the endpoint identity
// is content-addressed, so a different root under the same ID is not the pinned endpoint.
func recordedEndpointRoot(ctx context.Context, e sqlDBTX, src SnapshotSourceBasis) (string, error) {
	var canonical string
	err := e.QueryRowContext(ctx,
		"SELECT canonical FROM source_endpoints WHERE source_id = ? AND endpoint_id = ?",
		src.SourceID, src.EndpointID).Scan(&canonical)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("%w: pinned access endpoint %s is not recorded", ErrPublicationNotAccepted, shortRev(src.EndpointID))
	}
	if err != nil {
		return "", fmt.Errorf("failed to read pinned access endpoint %s: %w", shortRev(src.EndpointID), err)
	}
	var ep SourceAccessEndpoint
	if err := json.Unmarshal([]byte(canonical), &ep); err != nil || ep.RootDir == "" {
		return "", fmt.Errorf("%w: pinned access endpoint %s has no readable root", ErrPublicationNotAccepted, shortRev(src.EndpointID))
	}
	if _, id, err := ep.EndpointID(); err != nil || id != src.EndpointID {
		return "", fmt.Errorf("%w: recorded access endpoint does not match pinned endpoint %s", ErrPublicationNotAccepted, shortRev(src.EndpointID))
	}
	return filepath.Clean(ep.RootDir), nil
}

// publicationSnapshotHookForTest runs inside the snapshot read, after accepted_version is
// read and before the bases and inventory are read. Test-only; nil in production.
var publicationSnapshotHookForTest func()

// acceptedFolderPathsTx lists the accepted folder paths within tx. A folder row a seed
// inserted after the last version-advancing clean transition has not been accepted and is
// excluded.
func acceptedFolderPathsTx(ctx context.Context, tx *sql.Tx) (paths []string, err error) {
	rows, err := tx.QueryContext(ctx, `SELECT fo.path FROM folders fo
		WHERE NOT EXISTS (SELECT 1 FROM folder_unaccepted_seed s WHERE s.folder_id = fo.id)
		ORDER BY fo.path`)
	if err != nil {
		return nil, fmt.Errorf("failed to query accepted folders: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, fmt.Errorf("failed to scan accepted folder: %w", err)
		}
		paths = append(paths, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read accepted folders: %w", err)
	}
	return paths, nil
}

// acceptedFileIntegrityTx maps each accepted file of folderPath to its member integrity
// identity, within tx. A row a seed inserted after the last clean transition has not been
// accepted and is excluded.
func acceptedFileIntegrityTx(ctx context.Context, tx *sql.Tx, folderPath string) (map[string]string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT f.name, f.size, e.epoch
		FROM files f JOIN folders fo ON f.folder_id = fo.id
		LEFT JOIN file_accepted_epoch e ON e.folder_id = f.folder_id AND e.name = f.name
		WHERE fo.path = ?
		AND NOT EXISTS (SELECT 1 FROM file_unaccepted_seed s WHERE s.folder_id = f.folder_id AND s.name = f.name)`, folderPath)
	if err != nil {
		return nil, fmt.Errorf("failed to query accepted files for %s: %w", folderPath, err)
	}
	defer func() { _ = rows.Close() }()
	out := make(map[string]string)
	for rows.Next() {
		var (
			name  string
			size  int64
			epoch sql.NullInt64
		)
		if err := rows.Scan(&name, &size, &epoch); err != nil {
			return nil, fmt.Errorf("failed to scan accepted file for %s: %w", folderPath, err)
		}
		out[name] = memberIntegrity(size, epoch.Int64, epoch.Valid)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read accepted files for %s: %w", folderPath, err)
	}
	return out, nil
}
