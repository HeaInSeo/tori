package db

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// TestGetSubFolders verifies sub directory listing with exclusions.
func TestGetSubFolders(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "a"), 0755); err != nil {
		t.Fatalf("mkdir a: %v", err)
	}
	if err := os.Mkdir(filepath.Join(root, "b"), 0755); err != nil {
		t.Fatalf("mkdir b: %v", err)
	}
	if err := os.Mkdir(filepath.Join(root, "skip"), 0755); err != nil {
		t.Fatalf("mkdir skip: %v", err)
	}
	folders, err := GetSubFolders(root, []string{"skip"})
	if err != nil {
		t.Fatalf("GetSubFolders error: %v", err)
	}
	if len(folders) != 2 {
		t.Fatalf("expected 2 folders, got %d", len(folders))
	}
}

// TestGetFoldersInfo computes size and file count.
func TestGetFoldersInfo(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	if err := os.Mkdir(sub, 0755); err != nil {
		t.Fatalf("mkdir sub: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sub, "f1.txt"), []byte("abc"), 0644); err != nil {
		t.Fatalf("write f1.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sub, "f2.csv"), []byte("d"), 0644); err != nil {
		t.Fatalf("write f2.csv: %v", err)
	}
	folders, err := GetFoldersInfo(root, []string{"*.csv"})
	if err != nil {
		t.Fatalf("GetFoldersInfo error: %v", err)
	}
	if len(folders) != 1 {
		t.Fatalf("expected 1 folder, got %d", len(folders))
	}
	if folders[0].FileCount != 1 || folders[0].TotalSize != 3 {
		t.Errorf("unexpected folder stats: %+v", folders[0])
	}
}

// TestCheckForeignKeysEnabled ensures PRAGMA foreign_keys query works.
func TestCheckForeignKeysEnabled(t *testing.T) {
	db, err := ConnectDB("sqlite3", ":memory:", true)
	if err != nil {
		t.Fatalf("connect db: %v", err)
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Fatalf("close db: %v", closeErr)
		}
	}()
	on, err := CheckForeignKeysEnabled(db)
	if err != nil {
		t.Fatalf("CheckForeignKeysEnabled error: %v", err)
	}
	if !on {
		t.Errorf("expected foreign keys on")
	}
}

// TestClearDatabase deletes all data from folders table.
func TestClearDatabase(t *testing.T) {
	db := SetupInMemoryDB(t)
	// insert simple row
	_, err := db.Exec("INSERT INTO folders(path) VALUES('p')")
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	if err := ClearDatabase(db); err != nil {
		t.Fatalf("ClearDatabase error: %v", err)
	}
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM folders").Scan(&n); err != nil {
		t.Fatalf("scan count: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 rows, got %d", n)
	}
}

func TestCompareFoldersMatch(t *testing.T) {
	db := SetupInMemoryDB(t)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Fatalf("close db: %v", closeErr)
		}
	}()
	root := t.TempDir()
	sub := filepath.Join(root, "dir")
	if err := os.Mkdir(sub, 0755); err != nil {
		t.Fatalf("mkdir dir: %v", err)
	}
	// create file to give size
	if err := os.WriteFile(filepath.Join(sub, "f1.txt"), []byte("hi"), 0644); err != nil {
		t.Fatalf("write f1.txt: %v", err)
	}
	// insert folder info in DB matching disk
	_, err := db.Exec("INSERT INTO folders(path,total_size,file_count) VALUES(?,?,?)", sub, int64(2), int64(1))
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	unchanged, _, diffs, err := CompareFolders(db, root, nil, nil)
	if err != nil {
		t.Fatalf("CompareFolders error: %v", err)
	}
	if !unchanged || len(diffs) != 0 {
		t.Errorf("expected no diffs")
	}
}

func TestCompareFilesMatch(t *testing.T) {
	db := SetupInMemoryDB(t)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Fatalf("close db: %v", closeErr)
		}
	}()
	root := t.TempDir()
	// folder path
	folder := root
	// create file on disk
	if err := os.WriteFile(filepath.Join(folder, "f1.txt"), []byte("abc"), 0644); err != nil {
		t.Fatalf("write f1.txt: %v", err)
	}
	// insert folder and file in DB
	res, err := db.Exec("INSERT INTO folders(path,total_size,file_count) VALUES(?,?,?)", folder, int64(3), int64(1))
	if err != nil {
		t.Fatalf("insert folder: %v", err)
	}
	fid, _ := res.LastInsertId()
	_, err = db.Exec("INSERT INTO files(folder_id,name,size) VALUES(?,?,?)", fid, "f1.txt", int64(3))
	if err != nil {
		t.Fatalf("insert file: %v", err)
	}
	unchanged, _, changes, err := CompareFiles(db, folder, nil)
	if err != nil {
		t.Fatalf("CompareFiles error: %v", err)
	}
	if !unchanged || len(changes) != 0 {
		t.Errorf("expected files to match")
	}
}

func TestExtractFileNames(t *testing.T) {
	files := []File{{Name: "a"}, {Name: "b"}, {Name: "c"}}
	got := ExtractFileNames(files)
	want := []string{"a", "b", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ExtractFileNames mismatch: got %v want %v", got, want)
	}
}

// TODO 별도로 정리해야함.

func MakeTestFiles(path string) {
	// 디렉토리 생성
	err := os.MkdirAll(path, os.ModePerm)
	if err != nil {
		logger.Fatalf("Failed to create directory %s: %v", path, err)
	}

	// 디렉토리 권한을 777로 설정 os.ModePerm 해줌.
	/*err = os.Chmod(path, 0777) //0o777 이 방식보다 0777 방식 사용
	if err != nil {
		log.Fatalf("Failed to set permissions for directory %s: %v", path, err)
	}*/

	// 테스트 파일 이름 목록

	fileNames := []string{
		"sample1_S1_L001_R1_001.fastq.gz",
		"sample1_S1_L001_R2_001.fastq.gz",
		"sample1_S1_L002_R1_001.fastq.gz",
		"sample1_S1_L002_R2_001.fastq.gz",
		"sample2_S2_L001_R1_001.fastq.gz",
		"sample2_S2_L001_R2_001.fastq.gz",
		"sample2_S2_L002_R1_001.fastq.gz",
		"sample2_S2_L002_R2_001.fastq.gz",
		"sample3_S3_L001_R1_001.fastq.gz",
		"sample3_S3_L001_R2_001.fastq.gz",
		"sample3_S3_L002_R1_001.fastq.gz",
		"sample3_S3_L002_R2_001.fastq.gz",
		"sample4_S4_L001_R1_001.fastq.gz",
		"sample4_S4_L001_R2_001.fastq.gz",
		"sample4_S4_L002_R1_001.fastq.gz",
		"sample4_S4_L002_R2_001.fastq.gz",
		"sample5_S5_L001_R1_001.fastq.gz",
		"sample5_S5_L001_R2_001.fastq.gz",
		"sample5_S5_L002_R1_001.fastq.gz",
		"sample5_S5_L002_R2_001.fastq.gz",
		"sample6_S6_L001_R1_001.fastq.gz",
		"sample6_S6_L001_R2_001.fastq.gz",
		"sample6_S6_L002_R1_001.fastq.gz",
		"sample6_S6_L002_R2_001.fastq.gz",
		"sample7_S7_L001_R1_001.fastq.gz",
		"sample7_S7_L001_R2_001.fastq.gz",
		"sample7_S7_L002_R1_001.fastq.gz",
		"sample7_S7_L002_R2_001.fastq.gz",
		"sample8_S8_L001_R1_001.fastq.gz",
		"sample8_S8_L001_R2_001.fastq.gz",
		"sample8_S8_L002_R1_001.fastq.gz",
		"sample8_S8_L002_R2_001.fastq.gz",
		"sample9_S9_L001_R1_001.fastq.gz",
		"sample9_S9_L001_R2_001.fastq.gz",
		"sample9_S9_L002_R1_001.fastq.gz",
		"sample9_S9_L002_R2_001.fastq.gz",
		"sample10_S10_L001_R1_001.fastq.gz",
		"sample10_S10_L001_R2_001.fastq.gz",
		"sample10_S10_L002_R1_001.fastq.gz",
		"sample10_S10_L002_R2_001.fastq.gz",
		"sample11_S11_L001_R1_001.fastq.gz",
		"sample11_S11_L001_R2_001.fastq.gz",
		"sample11_S11_L002_R1_001.fastq.gz",
		"sample11_S11_L002_R2_001.fastq.gz",
		"sample12_S12_L001_R1_001.fastq.gz",
		"sample12_S12_L001_R2_001.fastq.gz",
		"sample12_S12_L002_R1_001.fastq.gz",
		"sample12_S12_L002_R2_001.fastq.gz",
	}
	/*
		incompleteFileNames := []string{
			"sample1_S1_L001_R1_001.fastq.gz",
			"sample1_S1_L001_R2_001.fastq.gz",
			"sample13_S13_L001_R1.fastq.gz",
			"sample14_S14_L001_R2_001.fastq",
			"sample15_S15_L001_001.fastq.gz",
			"sample16_S16_L001.fastq.gz",
		}
	*/
	// 파일 생성
	for _, fileName := range fileNames {
		filePath := fmt.Sprintf("%s/%s", path, fileName)
		_, err := os.Create(filePath)
		if err != nil {
			logger.Fatalf("Failed to create file %s: %v", filePath, err)
		} else {
			logger.Infof("Created file: %s", filePath)
		}
	}
}

func MakeTestFilesA(path string) {
	// 디렉토리 생성
	err := os.MkdirAll(path, os.ModePerm)
	if err != nil {
		logger.Fatalf("Failed to create directory %s: %v", path, err)
	}

	// 디렉토리 권한을 777로 설정 os.ModePerm 해줌.
	/*err = os.Chmod(path, 0777) //0o777 이 방식보다 0777 방식 사용
	if err != nil {
		log.Fatalf("Failed to set permissions for directory %s: %v", path, err)
	}*/

	// 테스트 파일 이름 목록

	fileNames := []string{
		"SRA_S1_L001_R1_001.fastq.gz",
		"SRA_S1_L001_R2_001.fastq.gz",
		"SRA_S1_L002_R1_001.fastq.gz",
		"SRA_S1_L002_R2_001.fastq.gz",
	}
	/*
		incompleteFileNames := []string{
			"sample1_S1_L001_R1_001.fastq.gz",
			"sample1_S1_L001_R2_001.fastq.gz",
			"sample13_S13_L001_R1.fastq.gz",
			"sample14_S14_L001_R2_001.fastq",
			"sample15_S15_L001_001.fastq.gz",
			"sample16_S16_L001.fastq.gz",
		}
	*/
	// 파일 생성
	for _, fileName := range fileNames {
		filePath := fmt.Sprintf("%s/%s", path, fileName)
		_, err := os.Create(filePath)
		if err != nil {
			logger.Fatalf("Failed to create file %s: %v", filePath, err)
		} else {
			logger.Infof("Created file: %s", filePath)
		}
	}
}
