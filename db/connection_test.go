package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func TestIsDBInitialized(t *testing.T) {
	// in-memory SQLite 데이터베이스 생성
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to create in-memory DB: %v", err)
	}
	defer func() {
		cErr := db.Close()
		if cErr != nil {
			logger.Warnf("failed to db close: %v", cErr)
		}
	}()

	// 초기 상태: 아무 테이블도 없으므로 false 여야 함.
	if isDBInitialized(db) {
		t.Error("isDBInitialized returned true on a DB with no tables")
	}

	// folders 테이블만 생성
	_, err = db.Exec("CREATE TABLE folders (id INTEGER PRIMARY KEY)")
	if err != nil {
		t.Fatalf("failed to create folders table: %v", err)
	}

	// files 테이블이 없으므로 여전히 false 여야 함.
	if isDBInitialized(db) {
		t.Error("isDBInitialized returned true when only folders table exists")
	}

	// files 테이블 생성
	_, err = db.Exec("CREATE TABLE files (id INTEGER PRIMARY KEY)")
	if err != nil {
		t.Fatalf("failed to create files table: %v", err)
	}

	// 이제 folders, files 두 테이블이 모두 존재하므로 true 여야 함.
	if !isDBInitialized(db) {
		t.Error("isDBInitialized returned false even though both folders and files tables exist")
	}
}

// TestConnectDB_EnableForeignKeys_Success 는 SQLite in‑memory DB 에서 외래 키 활성화가 정상 동작하는지 검증
func TestConnectDB_EnableForeignKeys_Success(t *testing.T) {
	// SQLite in‑memory DB 사용, enableForeignKeys=true
	db, err := ConnectDB("sqlite3", ":memory:", true)
	if err != nil {
		t.Fatalf("failed to connect DB: %v", err)
	}
	defer func() {
		cErr := db.Close()
		if cErr != nil {
			logger.Warnf("failed to db close: %v", cErr)
		}
	}()

	// 외래 키 활성화 상태 확인
	row := db.QueryRow("PRAGMA foreign_keys;")
	var fk int
	if err := row.Scan(&fk); err != nil {
		t.Fatalf("failed to query foreign_keys pragma: %v", err)
	}
	if fk != 1 {
		t.Errorf("expected foreign_keys to be enabled (1), got %d", fk)
	}
}

// TestConnectDB_DisableForeignKeys 는 SQLite in‑memory DB 에서 외래 키 활성화 옵션을 false 로 설정했을 때 검증
func TestConnectDB_DisableForeignKeys(t *testing.T) {
	// SQLite in‑memory DB 사용, enableForeignKeys=false
	db, err := ConnectDB("sqlite3", ":memory:", false)
	if err != nil {
		t.Fatalf("failed to connect DB: %v", err)
	}
	defer func() {
		cErr := db.Close()
		if cErr != nil {
			logger.Warnf("failed to db close: %v", cErr)
		}
	}()

	// 외래 키 활성화 상태 확인
	row := db.QueryRow("PRAGMA foreign_keys;")
	var fk int
	if err := row.Scan(&fk); err != nil {
		t.Fatalf("failed to query foreign_keys pragma: %v", err)
	}
	if fk != 0 {
		t.Errorf("expected foreign_keys to be disabled (0), got %d", fk)
	}
}

func createTestFolder(root, folderName string) (string, error) {
	folderPath := filepath.Join(root, folderName)
	if err := os.MkdirAll(folderPath, 0755); err != nil {
		return "", fmt.Errorf("failed to create folder %q: %w", folderPath, err)
	}
	filePath := filepath.Join(folderPath, "file.txt")
	content := []byte("test content")
	if err := os.WriteFile(filePath, content, 0644); err != nil {
		return "", fmt.Errorf("failed to create file %q: %w", filePath, err)
	}
	return folderPath, nil
}

func insertFolderInfo(db *sql.DB, folder Folder) error {
	query := "INSERT INTO folders (path, total_size, file_count, created_time) VALUES (?, ?, ?, ?)"
	_, err := db.Exec(query, folder.Path, folder.TotalSize, folder.FileCount, folder.CreatedTime)
	return err
}

func TestCompareFolders(t *testing.T) {
	tempRoot, err := os.MkdirTemp("", "test_compare_folders")
	if err != nil {
		t.Fatalf("failed to create temp root: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(tempRoot); err != nil {
			logger.Warnf("failed to RemoveAll: %v", err)
		}
	}()

	_, err = createTestFolder(tempRoot, "folder1")
	if err != nil {
		t.Fatalf("failed to create test folder: %v", err)
	}

	dbConn, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory DB: %v", err)
	}
	defer func() {
		if err := dbConn.Close(); err != nil {
			logger.Warnf("failed to close DB: %v", err)
		}
	}()

	// 종속성인 InitializeDatabase 실패 시 테스트를 건너뜀.
	if err := InitializeDatabase(dbConn); err != nil {
		t.Skipf("Skipping tests because InitializeDatabase failed: %v", err)
	}

	var foldersExclusions, filesExclusions []string

	t.Run("DB empty - differences exist", func(t *testing.T) {
		unchanged, diskFolders, diffs, err := CompareFolders(dbConn, tempRoot, foldersExclusions, filesExclusions)
		if err != nil {
			t.Fatalf("CompareFolders returned error: %v", err)
		}

		if len(diskFolders) == 0 {
			t.Errorf("expected at least 1 disk folder, got 0")
		}

		if unchanged {
			t.Errorf("expected overallSame to be false, got true")
		}
		if len(diffs) == 0 {
			t.Errorf("expected folder differences, got none")
		}
	})

	t.Run("DB matches disk - no differences", func(t *testing.T) {
		diskFolders, err := GetFoldersInfo(tempRoot, foldersExclusions)
		if err != nil {
			t.Fatalf("GetFoldersInfo failed: %v", err)
		}
		if len(diskFolders) == 0 {
			t.Fatalf("no disk folders found")
		}

		err = insertFolderInfo(dbConn, diskFolders[0])
		if err != nil {
			t.Fatalf("failed to insert folder info into DB: %v", err)
		}

		unchanged, _, diffs, err := CompareFolders(dbConn, tempRoot, foldersExclusions, filesExclusions)
		if err != nil {
			t.Fatalf("CompareFolders returned error: %v", err)
		}

		if !unchanged {
			t.Errorf("expected overallSame to be true, got false")
		}
		if len(diffs) != 0 {
			t.Errorf("expected no folder differences, got: %v", diffs)
		}
	})
}
