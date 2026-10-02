package db

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
)

// Accepted change epoch (TDI-I3M publication integrity).
//
// The local POSIX profile records no content digest, and the stat tuple is observation
// evidence that must never become a publication identity (TDI-I5P-1 / TDI-I2B). Size alone
// cannot tell two accepted contents of the same length apart, so every time an accepted
// inventory row is inserted or modified it is stamped with a fresh epoch from a monotonic,
// never-reused counter, in the same transaction as the row change. A publication member's
// integrity identity carries that epoch, so a same-length content change that was accepted as
// "modified" always yields a distinct manifest, while unchanged accepted rows keep their epoch
// and still converge across observation cycles.
//
// The epoch is NOT a content proof: an accepted metadata-only modification also gets a new
// epoch (a conservative split, never a false merge). Rows accepted before this table existed
// have no epoch; any later accepted change gives them one, so they can never collide with a
// changed revision of themselves.

const metaKeyFileEpochSeq = "file_accepted_epoch_seq"

// Acceptance boundary for seeded rows. A seed (StoreFilesFolderInfo) writes inventory rows
// without an acceptance transition; on an already-accepted DB a re-seed can add a new file to
// an established folder while the snapshot stays clean. Such a row has not crossed the
// acceptance boundary, so every row a seed inserts is marked in file_unaccepted_seed in the
// same transaction. The marker is cleared only by a canonical clean transition (commitClean)
// that advances accepted_version, which promotes exactly the DB rows the accepted projection
// was rebuilt from; a no-version-bump restore keeps it. A publication manifest never includes
// a marked row. A folder row a seed inserts is marked the same way in folder_unaccepted_seed:
// it has no classification basis at the current accepted_version, so the manifest must skip
// it rather than refuse to build.

// ensureAcceptedEpochTable creates file_accepted_epoch, file_unaccepted_seed and
// folder_unaccepted_seed if absent.
// Idempotent, like the other side tables, so an existing DB needs no migration step and the
// files table is unchanged.
func ensureAcceptedEpochTable(ctx context.Context, e sqlDBTX) error {
	const create = `CREATE TABLE IF NOT EXISTS file_accepted_epoch (
		folder_id INTEGER NOT NULL,
		name TEXT NOT NULL,
		epoch INTEGER NOT NULL,
		PRIMARY KEY (folder_id, name)
	);`
	if _, err := e.ExecContext(ctx, create); err != nil {
		return fmt.Errorf("failed to ensure file_accepted_epoch table: %w", err)
	}
	const createUnaccepted = `CREATE TABLE IF NOT EXISTS file_unaccepted_seed (
		folder_id INTEGER NOT NULL,
		name TEXT NOT NULL,
		PRIMARY KEY (folder_id, name)
	);`
	if _, err := e.ExecContext(ctx, createUnaccepted); err != nil {
		return fmt.Errorf("failed to ensure file_unaccepted_seed table: %w", err)
	}
	const createUnacceptedFolder = `CREATE TABLE IF NOT EXISTS folder_unaccepted_seed (
		folder_id INTEGER PRIMARY KEY
	);`
	if _, err := e.ExecContext(ctx, createUnacceptedFolder); err != nil {
		return fmt.Errorf("failed to ensure folder_unaccepted_seed table: %w", err)
	}
	return nil
}

// seedFileKey identifies one file_unaccepted_seed marker.
type seedFileKey struct {
	folderID int64
	name     string
}

// seedFrontier is the set of unaccepted-seed markers that existed when a projection rebuild
// started. Every marked row and folder in it was in the DB before the rebuild read the
// accepted rows, so a complete rebuild projected it. A seed that commits after the capture may
// or may not be in that projection, so its marker is not in the frontier and stays.
type seedFrontier struct {
	files   []seedFileKey
	folders []int64
}

// captureSeedFrontier reads the current unaccepted-seed markers. It must run before the
// projection rebuild whose clean transition will promote them.
func captureSeedFrontier(ctx context.Context, db *sql.DB) (seedFrontier, error) {
	if err := ensureAcceptedEpochTable(ctx, db); err != nil {
		return seedFrontier{}, err
	}
	files, err := seededFileMarkers(ctx, db)
	if err != nil {
		return seedFrontier{}, err
	}
	folders, err := seededFolderMarkers(ctx, db)
	if err != nil {
		return seedFrontier{}, err
	}
	return seedFrontier{files: files, folders: folders}, nil
}

func seededFileMarkers(ctx context.Context, db *sql.DB) ([]seedFileKey, error) {
	rows, err := db.QueryContext(ctx, "SELECT folder_id, name FROM file_unaccepted_seed")
	if err != nil {
		return nil, fmt.Errorf("failed to read seeded row markers: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []seedFileKey
	for rows.Next() {
		var k seedFileKey
		if err := rows.Scan(&k.folderID, &k.name); err != nil {
			return nil, fmt.Errorf("failed to scan seeded row marker: %w", err)
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

func seededFolderMarkers(ctx context.Context, db *sql.DB) ([]int64, error) {
	rows, err := db.QueryContext(ctx, "SELECT folder_id FROM folder_unaccepted_seed")
	if err != nil {
		return nil, fmt.Errorf("failed to read seeded folder markers: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan seeded folder marker: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// acceptSeededRowsTx clears exactly the unaccepted-seed markers of frontier; markers created
// after the frontier was captured stay. Called only from a version-advancing canonical clean
// transition, inside its transaction.
func acceptSeededRowsTx(ctx context.Context, e sqlDBTX, frontier seedFrontier) error {
	if err := ensureAcceptedEpochTable(ctx, e); err != nil {
		return err
	}
	for _, k := range frontier.files {
		if _, err := e.ExecContext(ctx, "DELETE FROM file_unaccepted_seed WHERE folder_id = ? AND name = ?", k.folderID, k.name); err != nil {
			return fmt.Errorf("failed to accept seeded row %s: %w", k.name, err)
		}
	}
	for _, id := range frontier.folders {
		if _, err := e.ExecContext(ctx, "DELETE FROM folder_unaccepted_seed WHERE folder_id = ?", id); err != nil {
			return fmt.Errorf("failed to accept seeded folder %d: %w", id, err)
		}
	}
	return nil
}

// markSeededFolderTx marks a folder row the seed just inserted as unaccepted until the next
// version-advancing clean transition.
func markSeededFolderTx(ctx context.Context, e sqlDBTX, folderID int64) error {
	if err := ensureAcceptedEpochTable(ctx, e); err != nil {
		return err
	}
	if _, err := e.ExecContext(ctx, "INSERT OR IGNORE INTO folder_unaccepted_seed (folder_id) VALUES (?)", folderID); err != nil {
		return fmt.Errorf("failed to mark seeded folder %d unaccepted: %w", folderID, err)
	}
	return nil
}

// stampAcceptedEpochTx gives one accepted file row a fresh epoch.
func stampAcceptedEpochTx(ctx context.Context, e sqlDBTX, folderID int64, name string) error {
	seq, err := metaGetInt(ctx, e, metaKeyFileEpochSeq)
	if err != nil {
		return err
	}
	seq++
	if err := metaSet(ctx, e, metaKeyFileEpochSeq, strconv.FormatInt(seq, 10)); err != nil {
		return err
	}
	const upsert = `INSERT INTO file_accepted_epoch (folder_id, name, epoch) VALUES (?, ?, ?)
		ON CONFLICT(folder_id, name) DO UPDATE SET epoch = excluded.epoch;`
	if _, err := e.ExecContext(ctx, upsert, folderID, name, seq); err != nil {
		return fmt.Errorf("failed to stamp accepted epoch for %s: %w", name, err)
	}
	return nil
}

// recordChangeEpochTx keeps file_accepted_epoch consistent with an applied file change inside
// the same transaction as the row mutation.
func recordChangeEpochTx(ctx context.Context, e sqlDBTX, fc FileChange) error {
	switch fc.ChangeType {
	case "added", "modified":
		return stampAcceptedEpochTx(ctx, e, fc.FolderID, fc.Name)
	case "removed":
		if _, err := e.ExecContext(ctx, "DELETE FROM file_accepted_epoch WHERE folder_id = ? AND name = ?", fc.FolderID, fc.Name); err != nil {
			return fmt.Errorf("failed to delete accepted epoch for %s: %w", fc.Name, err)
		}
		return nil
	default:
		return fmt.Errorf("unknown change type: %s", fc.ChangeType)
	}
}

// recordSeedEpochsTx stamps the rows a seed actually inserted and marks them unaccepted until
// the next clean transition. Re-seeding over an existing inventory must not re-stamp or
// un-accept rows that were already accepted.
func recordSeedEpochsTx(ctx context.Context, e sqlDBTX, folderID int64, files []File) error {
	if err := ensureAcceptedEpochTable(ctx, e); err != nil {
		return err
	}
	for _, f := range files {
		if err := stampAcceptedEpochTx(ctx, e, folderID, f.Name); err != nil {
			return err
		}
		if _, err := e.ExecContext(ctx, "INSERT OR IGNORE INTO file_unaccepted_seed (folder_id, name) VALUES (?, ?)", folderID, f.Name); err != nil {
			return fmt.Errorf("failed to mark seeded row %s unaccepted: %w", f.Name, err)
		}
	}
	return nil
}

// deleteFolderAcceptedEpochs drops the epochs of every file of a removed folder.
func deleteFolderAcceptedEpochs(ctx context.Context, e sqlDBTX, folderID int64) error {
	if _, err := e.ExecContext(ctx, "DELETE FROM file_accepted_epoch WHERE folder_id = ?", folderID); err != nil {
		return fmt.Errorf("failed to delete accepted epochs for folder id %d: %w", folderID, err)
	}
	return nil
}

// memberIntegrity is a publication member's integrity identity: the accepted size plus, when
// the row has one, its accepted change epoch.
func memberIntegrity(size int64, epoch int64, hasEpoch bool) string {
	s := "size:" + strconv.FormatInt(size, 10)
	if hasEpoch {
		s += ";accepted-change:" + strconv.FormatInt(epoch, 10)
	}
	return s
}
