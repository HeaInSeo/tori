package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
)

// This file implements TDI-I5P-1 (I5F0 profile: local POSIX only): a per-file stat-evidence
// tuple and the SUSPECT/HOLD comparator that closes the same-size / same-second rewrite
// defect of the legacy size + 1-second-mtime comparator.
//
// What the tuple is — and is not:
//
//   - (size, mtime_ns, ctime_ns, inode) is a METADATA OBSERVATION (a change hint with finer
//     resolution). Tuple equality is never a content-identity or write-completion proof, so
//     every stored row carries content_proof = NONE explicitly. Producer manifests, .done
//     markers, atomic-rename recognition, content hashing and quiet-period stability are out
//     of scope (I5F0 OPEN), as are NFS/Lustre/object profiles.
//   - The raw tuple is ephemeral observation evidence; it is NOT a Generation/publication
//     identity and is never folded into a SourceRevision or snapshot version (TDI-I2B).
//
// Comparator (disk tuple vs the tuple recorded when the accepted row was last written):
//
//   - size, mtime_ns or inode differ               → modified (inode change = rename-into-place)
//   - size, mtime_ns and inode equal, ctime differs → SUSPECT: HOLD, never "unchanged"
//   - disk tuple UNKNOWN (non-POSIX / no Sys())     → UNKNOWN: HOLD (fail closed)
//   - accepted row with no recorded tuple (legacy)  → HINT_ONLY: the first scan after upgrade
//     records the observed tuple only and HOLDs accepted-projection advance for that scan.
//     Existing accepted rows are not retroactively invalidated.

// EvidenceClass classifies what a file observation can prove. Like Scope/Coverage, the zero
// value is UNKNOWN so an unset class fails closed.
type EvidenceClass int

const (
	// EvidenceUnknown means no POSIX stat tuple could be observed for the file.
	EvidenceUnknown EvidenceClass = iota
	// EvidenceHintOnly means only the legacy 1-second hint exists for the accepted row; the
	// observed tuple is recorded as a baseline without advancing acceptance.
	EvidenceHintOnly
	// EvidenceStatTuple means a full tuple was observed and compared (still metadata only).
	EvidenceStatTuple
	// EvidenceSuspect means size, mtime_ns and inode match the recorded tuple but ctime_ns
	// does not: the file may have been rewritten with its mtime preserved.
	EvidenceSuspect
)

func (c EvidenceClass) String() string {
	switch c {
	case EvidenceHintOnly:
		return "HINT_ONLY"
	case EvidenceStatTuple:
		return "STAT_TUPLE"
	case EvidenceSuspect:
		return "SUSPECT"
	default:
		return "UNKNOWN"
	}
}

// contentProofNone is stored explicitly on every evidence row: no path in this package
// derives content identity from stat metadata.
const contentProofNone = "NONE"

// StatTuple is a local POSIX stat observation. Known=false means the platform did not expose
// the tuple and it must be treated as UNKNOWN.
type StatTuple struct {
	Known   bool
	Size    int64
	MtimeNs int64
	CtimeNs int64
	Inode   int64
}

// statTupleOf extracts the tuple from a FileInfo. It is a variable only so tests can simulate
// a platform without Sys() support; production uses the build-specific statTupleFromSys.
var statTupleOf = statTupleFromSys

// statObservation is the per-file evidence classification produced while diffing a folder.
type statObservation struct {
	FolderID int64
	Path     string // folder path
	Name     string
	Class    EvidenceClass
	Disk     StatTuple
}

func (o statObservation) String() string {
	return o.Path + string(os.PathSeparator) + o.Name
}

// storedEvidence is the tuple recorded for an accepted file row.
type storedEvidence struct {
	Tuple StatTuple
	Class string
}

// ensureStatEvidenceTable creates file_stat_evidence if absent. Idempotent and safe on every
// access, like the other side tables, so an existing DB needs no migration step and the files
// table schema is unchanged.
func ensureStatEvidenceTable(ctx context.Context, e sqlDBTX) error {
	const create = `CREATE TABLE IF NOT EXISTS file_stat_evidence (
		folder_id INTEGER NOT NULL,
		name TEXT NOT NULL,
		size INTEGER NOT NULL,
		mtime_ns INTEGER NOT NULL,
		ctime_ns INTEGER NOT NULL,
		inode INTEGER NOT NULL,
		evidence_class TEXT NOT NULL,
		content_proof TEXT NOT NULL DEFAULT 'NONE',
		PRIMARY KEY (folder_id, name)
	);`
	if _, err := e.ExecContext(ctx, create); err != nil {
		return fmt.Errorf("failed to ensure file_stat_evidence table: %w", err)
	}
	return nil
}

// upsertStatEvidence records the tuple observed for an accepted file row.
func upsertStatEvidence(ctx context.Context, e sqlDBTX, folderID int64, name string, t StatTuple, class EvidenceClass) error {
	if !t.Known {
		return fmt.Errorf("refusing to record an UNKNOWN stat tuple for %s", name)
	}
	const upsert = `INSERT INTO file_stat_evidence
		(folder_id, name, size, mtime_ns, ctime_ns, inode, evidence_class, content_proof)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(folder_id, name) DO UPDATE SET
			size = excluded.size, mtime_ns = excluded.mtime_ns, ctime_ns = excluded.ctime_ns,
			inode = excluded.inode, evidence_class = excluded.evidence_class,
			content_proof = excluded.content_proof;`
	if _, err := e.ExecContext(ctx, upsert, folderID, name, t.Size, t.MtimeNs, t.CtimeNs, t.Inode, class.String(), contentProofNone); err != nil {
		return fmt.Errorf("failed to record stat evidence for %s: %w", name, err)
	}
	return nil
}

// deleteStatEvidence drops the evidence for one file row.
func deleteStatEvidence(ctx context.Context, e sqlDBTX, folderID int64, name string) error {
	if _, err := e.ExecContext(ctx, "DELETE FROM file_stat_evidence WHERE folder_id = ? AND name = ?", folderID, name); err != nil {
		return fmt.Errorf("failed to delete stat evidence for %s: %w", name, err)
	}
	return nil
}

// deleteFolderStatEvidence drops the evidence for every file of a removed folder. It does not
// rely on foreign-key cascades, which are only active when the connection enables them.
func deleteFolderStatEvidence(ctx context.Context, e sqlDBTX, folderID int64) error {
	if _, err := e.ExecContext(ctx, "DELETE FROM file_stat_evidence WHERE folder_id = ?", folderID); err != nil {
		return fmt.Errorf("failed to delete stat evidence for folder id %d: %w", folderID, err)
	}
	return nil
}

// getStatEvidenceByPath returns the recorded tuples for the accepted files of folderPath.
func getStatEvidenceByPath(ctx context.Context, db *sql.DB, folderPath string) (map[string]storedEvidence, error) {
	if err := ensureStatEvidenceTable(ctx, db); err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `SELECT e.name, e.size, e.mtime_ns, e.ctime_ns, e.inode, e.evidence_class
		FROM file_stat_evidence e JOIN folders f ON f.id = e.folder_id WHERE f.path = ?`, folderPath)
	if err != nil {
		return nil, fmt.Errorf("failed to query stat evidence for %s: %w", folderPath, err)
	}
	defer func() {
		if cErr := rows.Close(); cErr != nil {
			logger.Warnf("failed to close stat evidence rows: %v", cErr)
		}
	}()
	out := make(map[string]storedEvidence)
	for rows.Next() {
		var (
			name string
			ev   storedEvidence
		)
		if err := rows.Scan(&name, &ev.Tuple.Size, &ev.Tuple.MtimeNs, &ev.Tuple.CtimeNs, &ev.Tuple.Inode, &ev.Class); err != nil {
			return nil, fmt.Errorf("failed to scan stat evidence for %s: %w", folderPath, err)
		}
		ev.Tuple.Known = true
		out[name] = ev
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate stat evidence for %s: %w", folderPath, err)
	}
	return out, nil
}

// classifyExisting compares an accepted row's recorded tuple with the disk tuple. modified
// reports a tuple-level change (size, mtime_ns or inode); class is the evidence class of the
// observation. The caller combines modified with the legacy size/1s-mtime comparison.
func classifyExisting(disk StatTuple, stored storedEvidence, hasStored bool) (modified bool, class EvidenceClass) {
	switch {
	case !disk.Known:
		return false, EvidenceUnknown
	case !hasStored:
		return false, EvidenceHintOnly
	case disk.Size != stored.Tuple.Size || disk.MtimeNs != stored.Tuple.MtimeNs || disk.Inode != stored.Tuple.Inode:
		return true, EvidenceStatTuple
	case disk.CtimeNs != stored.Tuple.CtimeNs:
		return false, EvidenceSuspect
	default:
		return false, EvidenceStatTuple
	}
}

// errStatEvidenceUnknown marks a fail-closed UNKNOWN observation.
var errStatEvidenceUnknown = errors.New("stat evidence UNKNOWN")

// holdOnStatEvidence decides whether the diffed observation may advance acceptance. It
// returns hold=true with a reason when any in-scope file is UNKNOWN or SUSPECT, or when
// legacy accepted rows have no recorded tuple yet. In the legacy case (first scan after the
// upgrade) it records the observed tuples as HINT_ONLY — an evidence write only; no accepted
// inventory row or projection changes — so the next scan compares tuples normally.
func holdOnStatEvidence(ctx context.Context, db *sql.DB, obs []statObservation) (hold bool, reason string, err error) {
	var unknown, suspect, hint []statObservation
	for _, o := range obs {
		switch o.Class {
		case EvidenceUnknown:
			unknown = append(unknown, o)
		case EvidenceSuspect:
			suspect = append(suspect, o)
		case EvidenceHintOnly:
			hint = append(hint, o)
		}
	}
	if len(unknown) > 0 {
		return true, fmt.Sprintf("%v for %s (no local POSIX stat tuple; %d file(s)); fail closed, accepted snapshot retained",
			errStatEvidenceUnknown, unknown[0], len(unknown)), nil
	}
	if len(hint) > 0 {
		if err := recordHintOnly(ctx, db, hint); err != nil {
			return false, "", err
		}
	}
	if len(suspect) > 0 {
		return true, fmt.Sprintf("stat evidence SUSPECT for %s (size/mtime_ns/inode unchanged but ctime_ns changed; %d file(s)); possible same-size rewrite, not accepted as unchanged",
			suspect[0], len(suspect)), nil
	}
	if len(hint) > 0 {
		return true, fmt.Sprintf("first stat-evidence scan: %d legacy accepted file(s) recorded as HINT_ONLY (e.g. %s); accepted projection advance HOLD for this scan",
			len(hint), hint[0]), nil
	}
	return false, "", nil
}

// recordHintOnly stores the observed tuples of legacy rows in one transaction.
func recordHintOnly(ctx context.Context, db *sql.DB, hint []statObservation) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin stat evidence tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	if err := ensureStatEvidenceTable(ctx, tx); err != nil {
		return err
	}
	for _, o := range hint {
		if err := upsertStatEvidence(ctx, tx, o.FolderID, o.Name, o.Disk, EvidenceHintOnly); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit stat evidence: %w", err)
	}
	committed = true
	return nil
}

// recordSeedStatEvidenceTx records the tuples observed while seeding a folder's rows.
func recordSeedStatEvidenceTx(ctx context.Context, tx *sql.Tx, folderID int64, files []File) error {
	if err := ensureStatEvidenceTable(ctx, tx); err != nil {
		return err
	}
	for _, f := range files {
		if !f.Stat.Known {
			continue
		}
		if err := upsertStatEvidence(ctx, tx, folderID, f.Name, f.Stat, EvidenceStatTuple); err != nil {
			return err
		}
	}
	return nil
}

// recordChangeEvidenceTx keeps the evidence side table consistent with an applied file
// change inside the same transaction as the row mutation. A change without a known tuple
// (e.g. constructed by a caller that did not observe one) drops any stale evidence so the
// row is treated as HINT_ONLY on the next scan instead of being compared to an old tuple.
func recordChangeEvidenceTx(ctx context.Context, tx *sql.Tx, fc FileChange) error {
	switch fc.ChangeType {
	case "added", "modified":
		if !fc.Stat.Known {
			return deleteStatEvidence(ctx, tx, fc.FolderID, fc.Name)
		}
		return upsertStatEvidence(ctx, tx, fc.FolderID, fc.Name, fc.Stat, EvidenceStatTuple)
	case "removed":
		return deleteStatEvidence(ctx, tx, fc.FolderID, fc.Name)
	default:
		return fmt.Errorf("unknown change type: %s", fc.ChangeType)
	}
}
