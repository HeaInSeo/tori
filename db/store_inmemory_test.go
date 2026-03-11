package db

import (
	"context"
	"database/sql"
	"fmt"
	u "github.com/seoyhaein/utils"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

var (
	TestFolderPath = "/test"
	Exclusions     = []string{"*.json", "invalid_files", "*.csv", "*.pb"}
)

// createLargeTestStructure 는 rootDir 아래에 numDirs 개의 하위 디렉토리를 생성하고,
// 각 디렉토리마다 numFilesPerDir 개의 파일을 생성한다.
func createLargeTestStructure(rootDir string, numDirs, numFilesPerDir int, createRule bool) error {
	// rootDir 이 빈 디렉토리인지 확인
	entries, err := os.ReadDir(rootDir)
	if err != nil {
		return err
	}
	if len(entries) > 0 {
		return fmt.Errorf("rootDir %s is not empty", rootDir)
	}

	// 하위 디렉토리 및 파일 생성
	for i := 0; i < numDirs; i++ {
		subDir := filepath.Join(rootDir, fmt.Sprintf("dir_%d", i))
		if err := os.Mkdir(subDir, 0755); err != nil {
			return err
		}
		// 옵션에 따라 rule.json 파일 생성
		if createRule {
			rulePath := filepath.Join(subDir, "rule.json")
			// rule.json 내용은 빈 객체("{}")로 설정 (필요에 따라 수정 가능)
			if err := os.WriteFile(rulePath, []byte("{}"), 0644); err != nil {
				return err
			}
		}
		// 각 하위 디렉토리에 파일 생성
		for j := 0; j < numFilesPerDir; j++ {
			filePath := filepath.Join(subDir, fmt.Sprintf("file_%d.txt", j))
			// 파일마다 내용 길이를 다르게 설정
			content := []byte(fmt.Sprintf("Content of file %d in directory %d", j, i))
			if err := os.WriteFile(filePath, content, 0644); err != nil {
				return err
			}
		}
	}
	return nil
}

// TODO 비정상 디렉토리 구조를 만들어서 제대로 에러를 리턴하는지 테스트 해야함.

// SetupInMemoryDB in‑memory SQLite DB를 생성하고, 필요한 테이블 스키마를 설정한다.
// 실패 시 t.Fatalf 를 호출하여 테스트를 중단한다.
func SetupInMemoryDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory db: %v", err)
	}

	schemas := []string{
		`CREATE TABLE IF NOT EXISTS folders (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			path TEXT NOT NULL UNIQUE,
			total_size INTEGER DEFAULT 0,
			file_count INTEGER DEFAULT 0,
			created_time DATETIME DEFAULT CURRENT_TIMESTAMP NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS files (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			folder_id INTEGER NOT NULL,
			name TEXT NOT NULL,
			size INTEGER NOT NULL,
			created_time DATETIME DEFAULT CURRENT_TIMESTAMP NOT NULL,
			FOREIGN KEY (folder_id) REFERENCES folders(id) ON DELETE CASCADE,
			UNIQUE(folder_id, name)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_files_folder_id ON files(folder_id);`,
	}

	for _, schema := range schemas {
		if _, err := db.Exec(schema); err != nil {
			t.Fatalf("failed to execute schema: %v", err)
		}
	}

	return db
}

// TestGetCurrentFolderFileInfo tests the GetCurrentFolderFileInfo function.
func TestGetCurrentFolderFileInfo(t *testing.T) {
	t.Run("NonExistentDirectory", func(t *testing.T) {
		_, _, err := GetCurrentFolderFileInfo("/nonexistent_directory_12345", nil)
		if err == nil {
			t.Error("expected error for non-existent directory, got nil")
		}
	})

	t.Run("DirectoryWithFilesAndExclusions", func(t *testing.T) {
		// 임시 디렉토리 생성
		tmpDir, err := os.MkdirTemp("", "getfolderinfo_test")
		if err != nil {
			t.Fatalf("failed to create temporary directory: %v", err)
		}

		// 테스트 종료 후 임시 디렉토리 삭제
		defer func() {
			if err := os.RemoveAll(tmpDir); err != nil {
				t.Logf("failed to remove temporary directory: %v", err)
			}
		}()

		// 테스트용 파일 생성
		// 포함되어야 할 파일: file1.txt, file2.log
		// 제외되어야 할 파일: file3.json, invalid_files, file4.csv
		fileData := map[string]string{
			"file1.txt":     "Content of file1",   // 포함
			"file2.log":     "Content of file2",   // 포함
			"file3.json":    "Content of file3",   // 제외: *.json
			"invalid_files": "Should be excluded", // 제외: 정확히 "invalid_files"
			"file4.csv":     "Content of file4",   // 제외: *.csv
		}
		var expectedTotalSize int64
		expectedFiles := []string{}
		for name, content := range fileData {
			filePath := filepath.Join(tmpDir, name)
			if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
				t.Fatalf("failed to write file %s: %v", name, err)
			}
			// determine if file should be included according to exclusions below
			// exclusions: {"*.json", "invalid_files", "*.csv"}
			exclude := false
			// check patterns: if file name ends with ".json" or ".csv" or equals "invalid_files"
			if strings.HasSuffix(name, ".json") || strings.HasSuffix(name, ".csv") || name == "invalid_files" {
				exclude = true
			}
			if !exclude {
				expectedTotalSize += int64(len(content))
				expectedFiles = append(expectedFiles, name)
			}
		}

		// 제외 패턴
		exclusions := []string{"*.json", "invalid_files", "*.csv"}

		// GetCurrentFolderFileInfo 함수 호출
		folder, files, err := GetCurrentFolderFileInfo(tmpDir, exclusions)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// 폴더 경로 비교
		if folder.Path != tmpDir {
			t.Errorf("expected folder path %q, got %q", tmpDir, folder.Path)
		}

		// 파일 개수 및 총 크기 검증
		if folder.FileCount != int64(len(expectedFiles)) {
			t.Errorf("expected file count %d, got %d", len(expectedFiles), folder.FileCount)
		}
		if folder.TotalSize != expectedTotalSize {
			t.Errorf("expected total size %d, got %d", expectedTotalSize, folder.TotalSize)
		}

		// 반환된 files slice에서 포함된 파일 이름 추출
		var returnedNames []string
		for _, f := range files {
			returnedNames = append(returnedNames, f.Name)
		}

		// 정렬 후 비교 (순서는 상관없으므로)
		sort.Strings(expectedFiles)
		sort.Strings(returnedNames)

		if len(returnedNames) != len(expectedFiles) {
			t.Errorf("expected %d files, got %d", len(expectedFiles), len(returnedNames))
		}
		for i, name := range expectedFiles {
			if returnedNames[i] != name {
				t.Errorf("expected file %q at index %d, got %q", name, i, returnedNames[i])
			}
		}

		// 파일의 CreatedTime 형식 검증 (간단하게 길이로 검사)
		for _, f := range files {
			if len(f.CreatedTime) != len("2006-01-02 15:04:05") {
				t.Errorf("unexpected CreatedTime format for file %q: got %q", f.Name, f.CreatedTime)
			}
		}
	})
}

func TestGetCurrentFolderFileInfo_LargeStructure_Extended(t *testing.T) {
	// 임시 루트 디렉토리 생성
	tmpDir, err := os.MkdirTemp("", "large_structure_test")
	if err != nil {
		t.Fatalf("failed to create temporary directory: %v", err)
	}
	// 테스트 종료 후 임시 디렉토리 삭제
	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Logf("failed to remove temporary directory: %v", err)
		}
	}()

	// 대규모 구조 생성: 예를 들어, 5개의 하위 디렉토리, 각 디렉토리당 10개의 파일, 그리고 각 디렉토리에 rule.json 생성
	numDirs := 5
	numFilesPerDir := 10
	createRule := true

	// 생성 전, tmpDir이 빈 디렉토리여야 함.
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("failed to read temporary directory: %v", err)
	}
	if len(entries) > 0 {
		t.Fatalf("temporary directory %s is not empty", tmpDir)
	}

	if err := createLargeTestStructure(tmpDir, numDirs, numFilesPerDir, createRule); err != nil {
		t.Fatalf("failed to create large test structure: %v", err)
	}

	// exclusions 설정: rule.json 파일은 제외
	exclusions := []string{"rule.json"}

	// 이제, tmpDir 아래의 각 하위 디렉토리(예: "dir_0", "dir_1", …)에 대해 GetCurrentFolderFileInfo를 호출하고 검증한다.
	subdirs, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("failed to read subdirectories of %s: %v", tmpDir, err)
	}

	for _, entry := range subdirs {
		if !entry.IsDir() {
			continue
		}
		subDirPath := filepath.Join(tmpDir, entry.Name())

		// GetCurrentFolderFileInfo 호출
		folder, files, err := GetCurrentFolderFileInfo(subDirPath, exclusions)
		if err != nil {
			t.Fatalf("GetCurrentFolderFileInfo failed for %s: %v", subDirPath, err)
		}

		// 예상: 각 하위 디렉토리에는 rule.json + numFilesPerDir 파일이 있으나 rule.json은 제외되어야 함.
		expectedFileCount := numFilesPerDir
		// 계산: 예상 총 크기는 각 파일의 내용 길이 합계.
		// createLargeTestStructure에서 파일 내용은: fmt.Sprintf("Content of file %d in directory %d", j, i)
		// 여기서 i는 해당 디렉토리의 인덱스 (예: "dir_0" → 0, "dir_1" → 1 등)
		var dirIndex int
		_, err = fmt.Sscanf(entry.Name(), "dir_%d", &dirIndex)
		if err != nil {
			t.Fatalf("failed to parse directory index from %s: %v", entry.Name(), err)
		}
		var expectedTotalSize int64 = 0
		for j := 0; j < numFilesPerDir; j++ {
			content := fmt.Sprintf("Content of file %d in directory %d", j, dirIndex)
			expectedTotalSize += int64(len(content))
		}

		// 검증: Folder.FileCount와 TotalSize
		if folder.FileCount != int64(expectedFileCount) {
			t.Errorf("for %s: expected file count %d, got %d", subDirPath, expectedFileCount, folder.FileCount)
		}
		if folder.TotalSize != expectedTotalSize {
			t.Errorf("for %s: expected total size %d, got %d", subDirPath, expectedTotalSize, folder.TotalSize)
		}

		// 파일 이름 검증: 반환된 files slice에서 파일 이름 목록 추출
		var returnedNames []string
		for _, f := range files {
			returnedNames = append(returnedNames, f.Name)
		}

		// 예상 파일 이름: "file_0.txt", "file_1.txt", ... "file_{numFilesPerDir-1}.txt"
		expectedNames := make([]string, 0, numFilesPerDir)
		for j := 0; j < numFilesPerDir; j++ {
			expectedNames = append(expectedNames, fmt.Sprintf("file_%d.txt", j))
		}

		sort.Strings(expectedNames)
		sort.Strings(returnedNames)
		if len(returnedNames) != len(expectedNames) {
			t.Errorf("for %s: expected %d files, got %d", subDirPath, len(expectedNames), len(returnedNames))
		} else {
			for i, name := range expectedNames {
				if returnedNames[i] != name {
					t.Errorf("for %s: expected file %q at index %d, got %q", subDirPath, name, i, returnedNames[i])
				}
			}
		}
		// 파일의 CreatedTime 형식 (길이) 검증
		for _, f := range files {
			if len(f.CreatedTime) != len("2006-01-02 15:04:05") {
				t.Errorf("for %s: unexpected CreatedTime format for file %q: got %q", subDirPath, f.Name, f.CreatedTime)
			}
		}
	}
}

func TestStoreFilesFolderInfo_RealDirectory(t *testing.T) {
	// in-memory DB 설정 (헬퍼 함수 사용)
	db := SetupInMemoryDB(t)
	defer func() {
		if err := db.Close(); err != nil {
			t.Fatalf("failed to close db: %v", err)
		}
	}()

	// 실제 테스트에 사용할 폴더 경로와 exclusion 패턴
	testFolderPath := TestFolderPath
	exclusions := Exclusions

	// u.CheckPath 의존성: 실패하면 테스트 건너뛰기
	checkedPath, err := u.CheckPath(testFolderPath)
	if err != nil {
		t.Skipf("skipping test because u.CheckPath failed: %v", err)
	}

	// GetCurrentFolderFileInfo 실제 함수 호출, 실패 시 테스트 건너뛰기
	fd, files, err := GetCurrentFolderFileInfo(checkedPath, exclusions)
	if err != nil {
		t.Skipf("skipping test because GetCurrentFolderFileInfo failed: %v", err)
	}
	// 간단한 결과 검증 (필요한 경우)
	if fd.Path == "" || len(files) == 0 {
		t.Skipf("skipping test because GetCurrentFolderFileInfo returned insufficient data")
	}

	// StoreFilesFolderInfo 함수 호출
	err = StoreFilesFolderInfo(context.Background(), db, testFolderPath, exclusions)
	if err != nil {
		t.Fatalf("StoreFilesFolderInfo failed: %v", err)
	}

	// 결과 검증: 폴더 정보가 DB에 저장되었는지 확인
	var folderID int64
	err = db.QueryRow("SELECT id FROM folders WHERE path = ?", testFolderPath).Scan(&folderID)
	if err != nil {
		t.Fatalf("failed to retrieve folder ID: %v", err)
	}

	// 결과 검증: 해당 폴더에 연결된 파일 레코드 수가 0보다 큰지 확인
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM files WHERE folder_id = ?", folderID).Scan(&count)
	if err != nil {
		t.Fatalf("failed to count files: %v", err)
	}
	if count == 0 {
		t.Errorf("expected files to be inserted, got %d", count)
	}
}

// TestStoreFilesFolderInfo_Integration_RealDirectory 는 실제 디렉토리(임시 디렉토리)를 사용하여,
// GetCurrentFolderFileInfo 함수가 반환한 결과와 DB에 저장된 Folder 및 File 데이터가 일치하는지 검증한다.
func TestStoreFilesFolderInfo_Integration_RealDirectory(t *testing.T) {
	// 임시 디렉토리 생성 (테스트용 실제 디렉토리 역할)
	tmpDir, err := os.MkdirTemp("", "storefiles_test")
	if err != nil {
		t.Fatalf("failed to create temporary directory: %v", err)
	}

	// 테스트 종료 후 임시 디렉토리 삭제
	defer func() {
		rErr := os.RemoveAll(tmpDir)
		if rErr != nil {
			logger.Warnf("failed to remove tmp directories %v", rErr)
		}
	}()

	// 테스트용 파일 생성
	file1Path := filepath.Join(tmpDir, "file1.txt")
	file2Path := filepath.Join(tmpDir, "file2.txt")
	content1 := []byte("Hello, World!")                // 약 13 bytes
	content2 := []byte("This is a test file content.") // 예시 문자열
	if err := os.WriteFile(file1Path, content1, 0644); err != nil {
		t.Fatalf("failed to write file1: %v", err)
	}
	if err := os.WriteFile(file2Path, content2, 0644); err != nil {
		t.Fatalf("failed to write file2: %v", err)
	}

	// in‑memory DB 설정 (픽스쳐 사용)
	db := SetupInMemoryDB(t)
	defer func() {
		if err := db.Close(); err != nil {
			t.Fatalf("failed to close db: %v", err)
		}
	}()

	// 실제 디렉토리 경로와 Exclusions 사용
	// u.CheckPath: 만약 실패하면 테스트 건너뛰기
	checkedPath, err := u.CheckPath(tmpDir)
	if err != nil {
		t.Skipf("skipping test because u.CheckPath failed: %v", err)
	}

	// 실제 GetCurrentFolderFileInfo 함수 호출하여 예상 Folder 와 File 값을 얻음
	expectedFolder, expectedFiles, err := GetCurrentFolderFileInfo(checkedPath, Exclusions)
	if err != nil {
		t.Skipf("skipping test because GetCurrentFolderFileInfo failed: %v", err)
	}
	if expectedFolder.Path == "" || len(expectedFiles) == 0 {
		t.Skipf("skipping test because GetCurrentFolderFileInfo returned insufficient data")
	}

	// StoreFilesFolderInfo 함수 호출: 실제 디렉토리의 정보를 DB에 저장
	err = StoreFilesFolderInfo(context.Background(), db, tmpDir, Exclusions)
	if err != nil {
		t.Fatalf("StoreFilesFolderInfo failed: %v", err)
	}

	// DB 에서 Folder 정보 조회 및 검증
	var folderID int64
	var dbPath string
	var dbTotalSize, dbFileCount int64
	var dbCreatedTime string
	err = db.QueryRow("SELECT id, path, total_size, file_count, created_time FROM folders WHERE path = ?", tmpDir).
		Scan(&folderID, &dbPath, &dbTotalSize, &dbFileCount, &dbCreatedTime)
	if err != nil {
		t.Fatalf("failed to retrieve folder info: %v", err)
	}

	// 예상 Folder 와 비교
	if dbPath != expectedFolder.Path {
		t.Errorf("folder path mismatch: expected %s, got %s", expectedFolder.Path, dbPath)
	}
	if dbTotalSize != expectedFolder.TotalSize {
		t.Errorf("folder total size mismatch: expected %d, got %d", expectedFolder.TotalSize, dbTotalSize)
	}
	if dbFileCount != expectedFolder.FileCount {
		t.Errorf("folder file count mismatch: expected %d, got %d", expectedFolder.FileCount, dbFileCount)
	}

	// DB 에서 File 정보 조회
	rows, err := db.Query("SELECT name, size FROM files WHERE folder_id = ?", folderID)
	if err != nil {
		t.Fatalf("failed to query files: %v", err)
	}

	// row 변경 가능성 때문에 이렇게 처리함.
	defer func(rows *sql.Rows) {
		cErr := rows.Close()
		if cErr != nil {
			logger.Warnf("failed to rows closed %v", cErr)
		}
	}(rows)

	var dbFiles []File
	for rows.Next() {
		var f File
		if err := rows.Scan(&f.Name, &f.Size); err != nil {
			t.Fatalf("failed to scan file row: %v", err)
		}
		dbFiles = append(dbFiles, f)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("error iterating file rows: %v", err)
	}

	// 정렬 후 예상 결과와 실제 DB 결과 비교 (파일 이름 기준)
	sort.Slice(expectedFiles, func(i, j int) bool {
		return expectedFiles[i].Name < expectedFiles[j].Name
	})
	sort.Slice(dbFiles, func(i, j int) bool {
		return dbFiles[i].Name < dbFiles[j].Name
	})

	if len(dbFiles) != len(expectedFiles) {
		t.Errorf("expected %d files, got %d", len(expectedFiles), len(dbFiles))
	} else {
		for i := range expectedFiles {
			if expectedFiles[i].Name != dbFiles[i].Name {
				t.Errorf("file name mismatch at index %d: expected %s, got %s", i, expectedFiles[i].Name, dbFiles[i].Name)
			}
			if expectedFiles[i].Size != dbFiles[i].Size {
				t.Errorf("file size mismatch for %s: expected %d, got %d", expectedFiles[i].Name, expectedFiles[i].Size, dbFiles[i].Size)
			}
		}
	}
}

func TestStoreFilesFolderInfo_LargeStructureIntegration(t *testing.T) {
	// 임시 디렉토리 생성 (테스트용 실제 디렉토리 역할)
	tmpDir, err := os.MkdirTemp("", "storefiles_large_test")
	if err != nil {
		t.Fatalf("failed to create temporary directory: %v", err)
	}
	// 테스트 종료 후 임시 디렉토리 삭제
	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Logf("failed to remove temporary directory: %v", err)
		}
	}()

	// 큰 규모의 디렉토리 구조 생성
	// 예: 10개의 하위 디렉토리, 각 디렉토리당 100개의 파일, 그리고 각 디렉토리에 rule.json 생성
	numDirs := 10
	numFilesPerDir := 100
	if err := createLargeTestStructure(tmpDir, numDirs, numFilesPerDir, true); err != nil {
		t.Fatalf("failed to create large test structure: %v", err)
	}

	// in-memory DB 설정 (픽스쳐 사용)
	db := SetupInMemoryDB(t)
	defer func() {
		if err := db.Close(); err != nil {
			t.Fatalf("failed to close db: %v", err)
		}
	}()

	// 전체 tmpDir 가 아닌, 각 하위 디렉토리에 대해 StoreFilesFolderInfo 를 호출하고 결과를 검증
	subdirs, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("failed to read subdirectories of %s: %v", tmpDir, err)
	}

	for _, entry := range subdirs {
		if !entry.IsDir() {
			continue
		}
		subDirPath := filepath.Join(tmpDir, entry.Name())

		// u.CheckPath: 실패 시 해당 디렉토리는 건너뛰기
		checkedPath, err := u.CheckPath(subDirPath)
		if err != nil {
			t.Skipf("skipping %s because u.CheckPath failed: %v", subDirPath, err)
		}

		// GetCurrentFolderFileInfo를 호출하여 예상 Folder와 File 정보를 얻는다.
		expectedFolder, expectedFiles, err := GetCurrentFolderFileInfo(checkedPath, Exclusions)
		if err != nil {
			t.Skipf("skipping %s because GetCurrentFolderFileInfo failed: %v", subDirPath, err)
		}
		if expectedFolder.Path == "" || len(expectedFiles) == 0 {
			t.Skipf("skipping %s because GetCurrentFolderFileInfo returned insufficient data", subDirPath)
		}

		// StoreFilesFolderInfo 함수 호출: 해당 하위 디렉토리 정보를 DB에 저장
		err = StoreFilesFolderInfo(context.Background(), db, subDirPath, Exclusions)
		if err != nil {
			t.Fatalf("StoreFilesFolderInfo failed for %s: %v", subDirPath, err)
		}

		// DB에서 Folder 정보 조회 및 검증
		var folderID int64
		var dbPath string
		var dbTotalSize, dbFileCount int64
		var dbCreatedTime string
		err = db.QueryRow("SELECT id, path, total_size, file_count, created_time FROM folders WHERE path = ?", subDirPath).
			Scan(&folderID, &dbPath, &dbTotalSize, &dbFileCount, &dbCreatedTime)
		if err != nil {
			t.Fatalf("failed to retrieve folder info for %s: %v", subDirPath, err)
		}

		if dbPath != expectedFolder.Path {
			t.Errorf("for %s: folder path mismatch: expected %q, got %q", subDirPath, expectedFolder.Path, dbPath)
		}
		if dbTotalSize != expectedFolder.TotalSize {
			t.Errorf("for %s: folder total size mismatch: expected %d, got %d", subDirPath, expectedFolder.TotalSize, dbTotalSize)
		}
		if dbFileCount != expectedFolder.FileCount {
			t.Errorf("for %s: folder file count mismatch: expected %d, got %d", subDirPath, expectedFolder.FileCount, dbFileCount)
		}

		// DB에서 File 정보 조회
		rows, err := db.Query("SELECT name, size FROM files WHERE folder_id = ?", folderID)
		if err != nil {
			t.Fatalf("failed to query files for %s: %v", subDirPath, err)
		}
		var dbFiles []File
		for rows.Next() {
			var f File
			if err := rows.Scan(&f.Name, &f.Size); err != nil {
				t.Fatalf("failed to scan file row for %s: %v", subDirPath, err)
			}
			dbFiles = append(dbFiles, f)
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("error iterating file rows for %s: %v", subDirPath, err)
		}
		if err := rows.Close(); err != nil {
			t.Fatalf("failed to close rows for %s: %v", subDirPath, err)
		}

		// 정렬 후 예상 결과와 실제 DB 결과 비교 (파일 이름 기준)
		sort.Slice(expectedFiles, func(i, j int) bool {
			return expectedFiles[i].Name < expectedFiles[j].Name
		})
		sort.Slice(dbFiles, func(i, j int) bool {
			return dbFiles[i].Name < dbFiles[j].Name
		})

		if len(dbFiles) != len(expectedFiles) {
			t.Errorf("for %s: expected %d files, got %d", subDirPath, len(expectedFiles), len(dbFiles))
		} else {
			for i := range expectedFiles {
				if dbFiles[i].Name != expectedFiles[i].Name {
					t.Errorf("for %s: expected file %q at index %d, got %q", subDirPath, expectedFiles[i].Name, i, dbFiles[i].Name)
				}
				// 파일 크기 비교
				if dbFiles[i].Size != expectedFiles[i].Size {
					t.Errorf("for %s: file size mismatch for %q: expected %d, got %d", subDirPath, expectedFiles[i].Name, expectedFiles[i].Size, dbFiles[i].Size)
				}
			}
		}
	}
}

// TODO 폴더 구조 및 반드시 들어가야 할 파일등에 대한 명시가 문서화 되어야 한다. 그리고 파일이나 폴더 구조가 아닐때 알 수 있도록 하는 체크 기능이 필요하다.
// TODO 지금은 그냥 에러로 처리해버리는데 이렇게 하지 말고 별도로, 사용자에게 고칠 수 있도록 하는 것으로 변경할 필요가 있다.
// TODO 성능 이슈가 발생할 가능성이 있음. 실제 구현 메서드의 성능 측면을 생각하고 관련 테스트를 더욱 고도화 할 필요가 있을듯. 일단 readme 말고 여기에 남겨 놓음.

func TestStoreFilesFolderInfo_TooLargeStructureIntegration(t *testing.T) {
	// 임시 디렉토리 생성 (테스트용 실제 디렉토리 역할)
	tmpDir, err := os.MkdirTemp("", "storefiles_large_test")
	if err != nil {
		t.Fatalf("failed to create temporary directory: %v", err)
	}
	// 테스트 종료 후 임시 디렉토리 삭제
	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Logf("failed to remove temporary directory: %v", err)
		}
	}()

	// 큰 규모의 디렉토리 구조 생성
	// 예: 10개의 하위 디렉토리, 각 디렉토리당 10000개의 파일, 그리고 각 디렉토리에 rule.json 생성
	numDirs := 10
	numFilesPerDir := 10000
	if err := createLargeTestStructure(tmpDir, numDirs, numFilesPerDir, true); err != nil {
		t.Fatalf("failed to create large test structure: %v", err)
	}

	// in-memory DB 설정 (픽스쳐 사용)
	db := SetupInMemoryDB(t)
	defer func() {
		if err := db.Close(); err != nil {
			t.Fatalf("failed to close db: %v", err)
		}
	}()

	// 전체 tmpDir가 아닌, 각 하위 디렉토리에 대해 StoreFilesFolderInfo를 호출하고 결과를 검증
	subdirs, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("failed to read subdirectories of %s: %v", tmpDir, err)
	}

	for _, entry := range subdirs {
		if !entry.IsDir() {
			continue
		}
		subDirPath := filepath.Join(tmpDir, entry.Name())

		// u.CheckPath: 실패 시 해당 디렉토리는 건너뛰기
		checkedPath, err := u.CheckPath(subDirPath)
		if err != nil {
			t.Skipf("skipping %s because u.CheckPath failed: %v", subDirPath, err)
		}

		// GetCurrentFolderFileInfo를 호출하여 예상 Folder와 File 정보를 얻는다.
		expectedFolder, expectedFiles, err := GetCurrentFolderFileInfo(checkedPath, Exclusions)
		if err != nil {
			t.Skipf("skipping %s because GetCurrentFolderFileInfo failed: %v", subDirPath, err)
		}
		if expectedFolder.Path == "" || len(expectedFiles) == 0 {
			t.Skipf("skipping %s because GetCurrentFolderFileInfo returned insufficient data", subDirPath)
		}

		// StoreFilesFolderInfo 함수 호출: 해당 하위 디렉토리 정보를 DB에 저장
		err = StoreFilesFolderInfo(context.Background(), db, subDirPath, Exclusions)
		if err != nil {
			t.Fatalf("StoreFilesFolderInfo failed for %s: %v", subDirPath, err)
		}

		// DB에서 Folder 정보 조회 및 검증
		var folderID int64
		var dbPath string
		var dbTotalSize, dbFileCount int64
		var dbCreatedTime string
		err = db.QueryRow("SELECT id, path, total_size, file_count, created_time FROM folders WHERE path = ?", subDirPath).
			Scan(&folderID, &dbPath, &dbTotalSize, &dbFileCount, &dbCreatedTime)
		if err != nil {
			t.Fatalf("failed to retrieve folder info for %s: %v", subDirPath, err)
		}

		if dbPath != expectedFolder.Path {
			t.Errorf("for %s: folder path mismatch: expected %q, got %q", subDirPath, expectedFolder.Path, dbPath)
		}
		if dbTotalSize != expectedFolder.TotalSize {
			t.Errorf("for %s: folder total size mismatch: expected %d, got %d", subDirPath, expectedFolder.TotalSize, dbTotalSize)
		}
		if dbFileCount != expectedFolder.FileCount {
			t.Errorf("for %s: folder file count mismatch: expected %d, got %d", subDirPath, expectedFolder.FileCount, dbFileCount)
		}

		// DB에서 File 정보 조회
		rows, err := db.Query("SELECT name, size FROM files WHERE folder_id = ?", folderID)
		if err != nil {
			t.Fatalf("failed to query files for %s: %v", subDirPath, err)
		}
		var dbFiles []File
		for rows.Next() {
			var f File
			if err := rows.Scan(&f.Name, &f.Size); err != nil {
				t.Fatalf("failed to scan file row for %s: %v", subDirPath, err)
			}
			dbFiles = append(dbFiles, f)
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("error iterating file rows for %s: %v", subDirPath, err)
		}
		if err := rows.Close(); err != nil {
			t.Fatalf("failed to close rows for %s: %v", subDirPath, err)
		}

		// 정렬 후 예상 결과와 실제 DB 결과 비교 (파일 이름 기준)
		sort.Slice(expectedFiles, func(i, j int) bool {
			return expectedFiles[i].Name < expectedFiles[j].Name
		})
		sort.Slice(dbFiles, func(i, j int) bool {
			return dbFiles[i].Name < dbFiles[j].Name
		})

		if len(dbFiles) != len(expectedFiles) {
			t.Errorf("for %s: expected %d files, got %d", subDirPath, len(expectedFiles), len(dbFiles))
		} else {
			for i := range expectedFiles {
				if dbFiles[i].Name != expectedFiles[i].Name {
					t.Errorf("for %s: expected file %q at index %d, got %q", subDirPath, expectedFiles[i].Name, i, dbFiles[i].Name)
				}
				// 파일 크기 비교
				if dbFiles[i].Size != expectedFiles[i].Size {
					t.Errorf("for %s: file size mismatch for %q: expected %d, got %d", subDirPath, expectedFiles[i].Name, expectedFiles[i].Size, dbFiles[i].Size)
				}
			}
		}
	}
}

// TODO CompareFoldersFiles 이거 테스트 코드 만들어야 함. DB 가 없을때 어떻게 되는지. 즉 테이블은 있는데 안에 내용이 없을때 어떻게 되는지. 특정 테이블만 있을때 등등.
