package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	u "github.com/seoyhaein/utils"
)

// SaveFolders rootPath 하위의 모든 Folder 에 대해 파일 정보를 DB에 삽입함.
func SaveFolders(ctx context.Context, db *sql.DB, rootPath string, foldersExclusions, filesExclusions []string) error {
	// rootPath 하위의 Folder 목록 조회
	folders, err := GetSubFolders(rootPath, foldersExclusions)
	if err != nil {
		return fmt.Errorf("failed to get subfolders from %s: %w", rootPath, err)
	}

	// 각 서브 Folder 에 대해 파일 정보를 DB에 삽입
	for _, folder := range folders {
		err = StoreFilesFolderInfo(ctx, db, folder.Path, filesExclusions)
		if err != nil {
			return fmt.Errorf("failed to load files info for folder %s: %w", folder.Path, err)
		}
	}

	return nil
}

// UpdateDB 폴더 변경 내역과 파일 변경 내역을 DB에 반영
func UpdateDB(ctx context.Context, db *sql.DB, diffs []FolderDiff, changes []FileChange) error {
	// 폴더 변경 업데이트
	if err := UpsertFolders(ctx, db, diffs); err != nil {
		return err
	}
	// UpsertFolders 해줘야지만, db 에 folderId 가 생겨서 검색할 수 가 있음.
	for i := range changes {
		folderId, err := getFolderID(db, changes[i].Path)
		if err != nil {
			return fmt.Errorf("failed to get folder ID for path %q: %w", changes[i].Path, err)
		}
		changes[i].FolderID = folderId
	}
	// 파일 변경 업데이트,
	if err := UpsertDelFiles(ctx, db, changes); err != nil {
		return err
	}
	return nil
}

// DiffFolders 폴더 파일 비교
func DiffFolders(db *sql.DB, rootPath string, foldersExclusions, filesExclusions []string) ([][]string, []FolderDiff, []FileChange, error) {
	// 1. 폴더 비교: 디스크 폴더들과 db의 폴더 목록을 비교
	_, folders, folderDiffs, err := CompareFolders(db, rootPath, foldersExclusions, filesExclusions)
	if err != nil {
		return nil, nil, nil, err
	}

	var (
		folderFiles    [][]string
		allFileChanges []FileChange
	)

	// 2. 각 폴더에 대해 파일 비교
	for _, folder := range folders {
		filesMatch, files, fileChanges, err := CompareFiles(db, folder.Path, filesExclusions)
		if err != nil {
			return nil, nil, nil, err
		}
		if !filesMatch {
			allFileChanges = append(allFileChanges, fileChanges...)
		}

		fileNames := ExtractFileNames(files)
		folderFiles = append(folderFiles, append([]string{folder.Path}, fileNames...))
	}

	// 전체 동일 여부 판단: folderDiffs 와 allFileChanges 가 모두 비어 있으면 동일
	if len(folderDiffs) == 0 && len(allFileChanges) == 0 {
		return folderFiles, nil, nil, nil
	}

	return folderFiles, folderDiffs, allFileChanges, nil
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

	// 파일 정보 삽입 (insert_file.sql)
	for _, file := range fileDetails {
		err = execSQLTx(ctx, tx, "insert_file.sql",
			folderID,
			file.Name,
			file.Size,
			file.CreatedTime)
		if err != nil {
			if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
				logger.Infof("rollback failed: %v", rbErr)
			}
			return fmt.Errorf("failed to insert file: %w", err)
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

func getFolderID(db *sql.DB, path string) (int64, error) {
	rows, err := querySQLNoCtx(db, "get_folder_id.sql", path)
	if err != nil {
		return 0, fmt.Errorf("querySQLNoCtx failed (get_folder_id.sql): %w", err)
	}
	defer func() {
		if cErr := rows.Close(); cErr != nil {
			logger.Warnf("failed to close rows: %v", cErr)
		}
	}()

	if !rows.Next() {
		return 0, sql.ErrNoRows
	}
	var id int64
	if err := rows.Scan(&id); err != nil {
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
	//defer rows.Close()
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
