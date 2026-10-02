package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	u "github.com/HeaInSeo/tori/internal/utils"
)

// SaveFolders rootPath 하위의 모든 Folder 에 대해 파일 정보를 DB에 삽입함.
func SaveFolders(ctx context.Context, db *sql.DB, rootPath string, foldersExclusions, filesExclusions []string) error {
	// rootPath 하위의 Folder 목록 조회
	folders, err := GetSubFolders(rootPath, foldersExclusions)
	if err != nil {
		return fmt.Errorf("failed to get subfolders from %s: %w", rootPath, err)
	}

	// TDI-I4F v0.3: a genuine fresh seed is one applied to an EMPTY DB. Capture that BEFORE
	// seeding so a re-seed over an existing (possibly pre-v0.3 legacy) inventory is never
	// mislabeled SEED_ONLY — that would reopen the F1 silent-reinterpretation hole.
	existing, err := GetFoldersFromDB(db)
	if err != nil {
		return fmt.Errorf("failed to read existing inventory: %w", err)
	}
	wasEmpty := len(existing) == 0

	// TDI-I4F v0.3 (F1): durably record a fresh seed's acceptance provenance BEFORE the
	// first seed row is written, not after. Ordering the SEED_ONLY marker strictly ahead of
	// any StoreFilesFolderInfo commit guarantees the invariant "seed rows exist ⇒ provenance
	// is set": an interrupted seed can then only leave (provenance=SEED_ONLY, 0 or partial
	// rows), which resolveProvenanceForSync classifies SEED_ONLY and the accept path
	// bootstraps — never (rows, no provenance), which would wedge into UNKNOWN_LEGACY/HOLD
	// with no way to repair via a re-seed (wasEmpty would then be false). Only a true fresh
	// seed (empty DB) records it, and only when not already set (never clobbers
	// ACCEPTED/UNKNOWN_LEGACY).
	if wasEmpty {
		if err := recordSeedProvenanceIfUnset(ctx, db); err != nil {
			return fmt.Errorf("failed to record seed provenance: %w", err)
		}
	}

	// 각 서브 Folder 에 대해 파일 정보를 DB에 삽입
	for _, folder := range folders {
		err = StoreFilesFolderInfo(ctx, db, folder.Path, filesExclusions)
		if err != nil {
			return fmt.Errorf("failed to load files info for folder %s: %w", folder.Path, err)
		}
	}

	// 초기 스냅샷을 즉시 source-continuity witness 로 보호한다 (부트스트랩 시에만 생성).
	if err := establishWitnessIfBootstrap(ctx, db, rootPath); err != nil {
		return fmt.Errorf("failed to establish source witness: %w", err)
	}

	return nil
}

// UpdateDB 폴더 변경 내역과 파일 변경 내역을 DB에 반영.
// IMPORTANT: 전체 변경(폴더 upsert + 파일 add/modify/remove)을 단일 트랜잭션으로 적용한다.
// 다중 행 적용 도중 실패하면 전부 롤백되므로, 부분 변형(partial mutation)이 수용된 스냅샷으로
// 남을 수 없다 (TDI-I10). 트랜잭션 커밋 전까지는 어떤 accepted 상태 변형도 발생하지 않는다.
func UpdateDB(ctx context.Context, db *sql.DB, diffs []FolderDiff, changes []FileChange) (err error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			logger.Warnf("rollback failed: %v", rbErr)
		}
	}()

	// 폴더 변경 업데이트
	for i := range diffs {
		if uErr := diffs[i].UpsertFolderTx(ctx, tx); uErr != nil {
			return uErr
		}
	}
	// UpsertFolders 해줘야지만, db 에 folderId 가 생겨서 검색할 수 가 있음.
	// removed 파일은 Path == "" 이고 FolderID 가 이미 DB 에서 채워져 있으므로 건너뜀.
	// 같은 폴더에 속한 파일이 여럿일 때 getFolderIDTx 쿼리가 중복 실행되지 않도록 캐싱.
	folderIDCache := make(map[string]int64)
	for i := range changes {
		if changes[i].ChangeType == "removed" {
			continue
		}
		if id, ok := folderIDCache[changes[i].Path]; ok {
			changes[i].FolderID = id
			continue
		}
		folderId, gErr := getFolderIDTx(ctx, tx, changes[i].Path)
		if gErr != nil {
			return fmt.Errorf("failed to get folder ID for path %q: %w", changes[i].Path, gErr)
		}
		folderIDCache[changes[i].Path] = folderId
		changes[i].FolderID = folderId
	}
	// 파일 변경 업데이트,
	for i := range changes {
		if uErr := changes[i].UpsertDelFileTx(ctx, tx); uErr != nil {
			return uErr
		}
	}

	// TDI-I5P-1: stat evidence side table 을 같은 tx 에서 행 변경과 일치시킨다.
	// TDI-I3M: accepted change epoch 도 같은 tx 에서 행 변경과 일치시킨다.
	if len(diffs) > 0 || len(changes) > 0 {
		if eErr := ensureStatEvidenceTable(ctx, tx); eErr != nil {
			return eErr
		}
		if eErr := ensureAcceptedEpochTable(ctx, tx); eErr != nil {
			return eErr
		}
	}
	for i := range diffs {
		if diffs[i].ChangeType == "removed" {
			if eErr := deleteFolderStatEvidence(ctx, tx, diffs[i].FolderID); eErr != nil {
				return eErr
			}
			if eErr := deleteFolderAcceptedEpochs(ctx, tx, diffs[i].FolderID); eErr != nil {
				return eErr
			}
		}
	}
	for i := range changes {
		if eErr := recordChangeEvidenceTx(ctx, tx, changes[i]); eErr != nil {
			return eErr
		}
		if eErr := recordChangeEpochTx(ctx, tx, changes[i]); eErr != nil {
			return eErr
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	committed = true
	return nil
}

// DiffFolders 폴더 파일 비교
func DiffFolders(db *sql.DB, rootPath string, foldersExclusions, filesExclusions []string) ([][]string, []FolderDiff, []FileChange, error) {
	folderFiles, folderDiffs, fileChanges, _, err := diffFoldersWithEvidence(db, rootPath, foldersExclusions, filesExclusions)
	return folderFiles, folderDiffs, fileChanges, err
}

// diffFoldersWithEvidence is DiffFolders that also returns every in-scope file's stat-evidence
// observation (TDI-I5P-1), so SyncFolders can HOLD on UNKNOWN/SUSPECT/HINT_ONLY before
// accepting the diff.
func diffFoldersWithEvidence(db *sql.DB, rootPath string, foldersExclusions, filesExclusions []string) ([][]string, []FolderDiff, []FileChange, []statObservation, error) {
	// 1. 폴더 비교: 디스크 폴더들과 db의 폴더 목록을 비교
	_, folders, folderDiffs, err := CompareFolders(db, rootPath, foldersExclusions, filesExclusions)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	var (
		folderFiles    [][]string
		allFileChanges []FileChange
		allObs         []statObservation
	)

	// 2. 각 폴더에 대해 파일 비교
	for _, folder := range folders {
		filesMatch, files, fileChanges, obs, err := compareFilesWithEvidence(db, folder.Path, filesExclusions)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		if !filesMatch {
			allFileChanges = append(allFileChanges, fileChanges...)
		}
		allObs = append(allObs, obs...)

		fileNames := ExtractFileNames(files)
		folderFiles = append(folderFiles, append([]string{folder.Path}, fileNames...))
	}

	// 전체 동일 여부 판단: folderDiffs 와 allFileChanges 가 모두 비어 있으면 동일
	if len(folderDiffs) == 0 && len(allFileChanges) == 0 {
		return folderFiles, nil, nil, allObs, nil
	}

	return folderFiles, folderDiffs, allFileChanges, allObs, nil
}

// StoreFilesFolderInfo 폴더 경로를 받아 폴더 내 파일 정보를 DB에 삽입하는 함수, TODO 한번만 실행되고 말아야 함. 이름 수정하자.
func StoreFilesFolderInfo(ctx context.Context, db *sql.DB, folderPath string, exclusions []string) error {
	folderPath, err := u.CheckPath(folderPath)
	if err != nil {
		return err
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}

	folderDetails, fileDetails, err := GetCurrentFolderFileInfo(folderPath, exclusions)

	if err != nil {
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			logger.Infof("rollback failed: %v", rbErr)
		}
		return fmt.Errorf("failed to get folder details: %w", err)
	}

	// TDI-I3M: 이번 seed 가 folder row 를 새로 넣는지 같은 tx 에서 확인한다 (insert_folder.sql 은
	// 기존 row 를 ON CONFLICT DO NOTHING 으로 남긴다).
	var folderExisted bool
	err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM folders WHERE path = ?)", folderDetails.Path).Scan(&folderExisted)
	if err != nil {
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			logger.Infof("rollback failed: %v", rbErr)
		}
		return fmt.Errorf("failed to check existing folder: %w", err)
	}

	// DB에 폴더 정보 삽입 (insert_folder.sql)
	err = execSQLTx(ctx, tx, "insert_folder.sql",
		folderDetails.Path,
		folderDetails.TotalSize,
		folderDetails.FileCount,
		folderDetails.CreatedTime)
	if err != nil {
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			logger.Infof("rollback failed: %v", rbErr)
		}
		return fmt.Errorf("failed to insert folder: %w", err)
	}

	// 삽입된 폴더의 ID를 조회
	var folderID int64
	err = tx.QueryRowContext(ctx, "SELECT id FROM folders WHERE path = ?", folderDetails.Path).Scan(&folderID)
	if err != nil {
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			logger.Infof("rollback failed: %v", rbErr)
		}
		return fmt.Errorf("failed to query folder ID: %w", err)
	}

	// 파일 정보 삽입 (insert_file.sql) — SQL을 한 번 준비하고 루프에서 재사용
	insertFileSQL, err := loadSQL("insert_file.sql")
	if err != nil {
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			logger.Infof("rollback failed: %v", rbErr)
		}
		return fmt.Errorf("failed to load insert_file.sql: %w", err)
	}
	stmt, err := tx.PrepareContext(ctx, insertFileSQL)
	if err != nil {
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			logger.Infof("rollback failed: %v", rbErr)
		}
		return fmt.Errorf("failed to prepare insert_file statement: %w", err)
	}
	defer func() {
		if closeErr := stmt.Close(); closeErr != nil {
			logger.Warnf("failed to close insert_file statement: %v", closeErr)
		}
	}()

	// insert_file.sql 은 같은 이름의 기존 row 를 ON CONFLICT DO NOTHING 으로 남겨 두므로,
	// 이번 호출이 실제로 새로 넣은 row 만 모은다.
	var inserted []File
	for _, file := range fileDetails {
		res, execErr := stmt.ExecContext(ctx, folderID, file.Name, file.Size, file.CreatedTime)
		if execErr != nil {
			if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
				logger.Infof("rollback failed: %v", rbErr)
			}
			return fmt.Errorf("failed to insert file: %w", execErr)
		}
		n, raErr := res.RowsAffected()
		if raErr != nil {
			if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
				logger.Infof("rollback failed: %v", rbErr)
			}
			return fmt.Errorf("failed to read inserted row count for %s: %w", file.Name, raErr)
		}
		if n > 0 {
			inserted = append(inserted, file)
		}
	}

	// TDI-I5P-1: seed 시 관측한 stat tuple 을 같은 tx 에서 evidence 로 기록한다. 새로 넣은
	// row 에 대해서만 기록한다. 이미 있던 (accepted 일 수 있는) row 의 evidence 는 re-seed 가
	// 덮어쓰지 않는다 — 덮어쓰면 acceptance 없이 baseline 이 바뀌어 same-size rewrite 가 다음
	// sync 에서 unchanged 로 숨는다. evidence 가 없던 기존 row 는 다음 scan 에서 HINT_ONLY 로
	// HOLD 된다. tuple 이 UNKNOWN 인 파일은 기록하지 않으며, 다음 SyncFolders 가 fail closed 로
	// HOLD 한다.
	if err = recordSeedStatEvidenceTx(ctx, tx, folderID, inserted); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			logger.Infof("rollback failed: %v", rbErr)
		}
		return err
	}
	// TDI-I3M: 새로 넣은 row 에만 accepted change epoch 를 부여한다 (기존 row 의 epoch 는 유지).
	if err = recordSeedEpochsTx(ctx, tx, folderID, inserted); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			logger.Infof("rollback failed: %v", rbErr)
		}
		return err
	}
	// 새로 넣은 folder row 는 accepted_version 의 classification basis 가 없으므로, 다음 version
	// 을 올리는 acceptance 전까지 publication manifest 에서 제외되도록 표시한다.
	if !folderExisted {
		if err = markSeededFolderTx(ctx, tx, folderID); err != nil {
			if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
				logger.Infof("rollback failed: %v", rbErr)
			}
			return err
		}
	}

	err = execSQLTx(ctx, tx, "update_folders_fromDB.sql", folderID)
	if err != nil {
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			logger.Infof("rollback failed: %v", rbErr)
		}
		return fmt.Errorf("failed to update folder statistics: %w", err)
	}

	// 트랜잭션 커밋
	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// getFolderIDTx 는 진행 중인 tx 내에서 folder id 를 조회한다.
func getFolderIDTx(ctx context.Context, tx *sql.Tx, path string) (int64, error) {
	query, err := loadSQL("get_folder_id.sql")
	if err != nil {
		return 0, err
	}
	var id int64
	if err := tx.QueryRowContext(ctx, query, path).Scan(&id); err != nil {
		return 0, fmt.Errorf("failed to scan folder id: %w", err)
	}
	return id, nil
}

// UpsertFolders FolderDiff 슬라이스에 대해 DB 업데이트(업서트)를 수행.
func UpsertFolders(ctx context.Context, db *sql.DB, diffs []FolderDiff) error {
	for _, diff := range diffs {
		if err := diff.UpsertFolder(ctx, db); err != nil {
			return err
		}
	}
	return nil
}

// UpsertDelFiles 전에 []FileChange 에 folder_id 와 file id 를 채워 넣는 과정이 필요하다.

// UpsertDelFiles FileChange 슬라이스에 대해 DB 업데이트(업서트)를 수행.
func UpsertDelFiles(ctx context.Context, db *sql.DB, changes []FileChange) error {
	for _, change := range changes {
		if err := change.UpsertDelFile(ctx, db); err != nil {
			return err
		}
	}
	return nil
}

// ClearDatabase for test
func ClearDatabase(db *sql.DB) error {
	// 외래 키 제약 조건이 ON DELETE CASCADE 로 설정되어 있다면, folders 테이블에서 데이터를 삭제하면 files 테이블의 데이터도 자동 삭제.
	_, err := db.Exec("DELETE FROM folders;")
	return err
}

// GetFoldersFromDB DB의 폴더 정보를 조회하여 Folder 구조체 슬라이스로 반환함.
// IMPORTANT: 호출자가 반환된 rows 를 직접 Close() 할 필요는 없음. 내부에서 모두 처리됨.
func GetFoldersFromDB(db *sql.DB) (folders []Folder, err error) {
	// "select_folders.sql" 파일에 정의된 SELECT 쿼리를 실행하여 폴더 정보를 조회
	rows, err := querySQLNoCtx(db, "select_folders.sql")
	if err != nil {
		return nil, fmt.Errorf("failed to query folders: %w", err)
	}
	// defer rows.Close()
	defer func() {
		if cErr := rows.Close(); cErr != nil {
			if err == nil {
				err = fmt.Errorf("failed to close rows: %w", cErr)
			} else {
				err = fmt.Errorf("%v; failed to close rows: %w", err, cErr)
			}
		}
	}()

	// 각 행을 순회하면서 Folder 구조체에 스캔
	for rows.Next() {
		var f Folder
		err = rows.Scan(&f.ID, &f.Path, &f.TotalSize, &f.FileCount, &f.CreatedTime)
		if err != nil {
			return nil, fmt.Errorf("failed to scan folder: %w", err)
		}
		folders = append(folders, f)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return folders, nil
}

// GetFilesFromDB DB의 파일 정보를 조회하여 File 구조체 슬라이스로 반환함.
// IMPORTANT: 호출자가 반환된 rows 를 직접 Close() 할 필요는 없음. 내부에서 모두 처리됨.
func GetFilesFromDB(db *sql.DB) (files []File, err error) {
	// "select_files.sql" 파일에 정의된 SELECT 쿼리를 실행하여 파일 정보를 조회
	rows, err := querySQLNoCtx(db, "select_files.sql")
	if err != nil {
		return nil, fmt.Errorf("failed to query files: %w", err)
	}

	defer func() {
		if cErr := rows.Close(); cErr != nil {
			if err == nil {
				err = fmt.Errorf("failed to close rows: %w", cErr)
			} else {
				err = fmt.Errorf("%v; failed to close rows: %w", err, cErr)
			}
		}
	}()

	// 각 행을 순회하면서 File 구조체에 스캔
	for rows.Next() {
		var f File
		err = rows.Scan(&f.ID, &f.FolderID, &f.Name, &f.Size, &f.CreatedTime)
		if err != nil {
			return nil, fmt.Errorf("failed to scan file: %w", err)
		}
		files = append(files, f)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return files, nil
}

// GetFilesByPathFromDB 는 주어진 Folder 경로에 해당하는 파일 정보를 DB 에서 조회함.
// IMPORTANT: SQL 쿼리는 "queries/select_files_for_folder.sql" 파일에 분리되어 있음.
func GetFilesByPathFromDB(db *sql.DB, folderPath string) (files []File, err error) {
	// "select_files_for_folder.sql" 파일에 정의된 쿼리를 실행하여 파일 정보를 조회
	rows, err := querySQLNoCtx(db, "select_files_for_folder.sql", folderPath)
	if err != nil {
		return nil, fmt.Errorf("failed to query files for folder %s: %w", folderPath, err)
	}

	defer func() {
		if cErr := rows.Close(); cErr != nil {
			if err == nil {
				err = fmt.Errorf("failed to close rows: %w", cErr)
			} else {
				err = fmt.Errorf("%v; failed to close rows: %w", err, cErr)
			}
		}
	}()

	for rows.Next() {
		var f File
		if err := rows.Scan(&f.ID, &f.FolderID, &f.Name, &f.Size, &f.CreatedTime); err != nil {
			return nil, fmt.Errorf("failed to scan file for folder %s: %w", folderPath, err)
		}
		files = append(files, f)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error for folder %s: %w", folderPath, err)
	}
	return files, err
}
