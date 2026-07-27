package db

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// TestGetSubFolders verifies sub directory listing with exclusions.
func TestGetSubFolders(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "a"), 0750); err != nil {
		t.Fatalf("mkdir a: %v", err)
	}
	if err := os.Mkdir(filepath.Join(root, "b"), 0750); err != nil {
		t.Fatalf("mkdir b: %v", err)
	}
	if err := os.Mkdir(filepath.Join(root, "skip"), 0750); err != nil {
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
	if err := os.Mkdir(sub, 0750); err != nil {
		t.Fatalf("mkdir sub: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sub, "f1.txt"), []byte("abc"), 0600); err != nil {
		t.Fatalf("write f1.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sub, "f2.csv"), []byte("d"), 0600); err != nil {
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
	if err := os.Mkdir(sub, 0750); err != nil {
		t.Fatalf("mkdir dir: %v", err)
	}
	// create file to give size
	if err := os.WriteFile(filepath.Join(sub, "f1.txt"), []byte("hi"), 0600); err != nil {
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
	if err := os.WriteFile(filepath.Join(folder, "f1.txt"), []byte("abc"), 0600); err != nil {
		t.Fatalf("write f1.txt: %v", err)
	}
	// insert folder and file in DB — created_time must match disk mtime for no-change detection
	res, err := db.Exec("INSERT INTO folders(path,total_size,file_count) VALUES(?,?,?)", folder, int64(3), int64(1))
	if err != nil {
		t.Fatalf("insert folder: %v", err)
	}
	fid, _ := res.LastInsertId()
	info, err := os.Stat(filepath.Join(folder, "f1.txt"))
	if err != nil {
		t.Fatalf("stat f1.txt: %v", err)
	}
	mtime := info.ModTime().Format("2006-01-02 15:04:05")
	_, err = db.Exec("INSERT INTO files(folder_id,name,size,created_time) VALUES(?,?,?,?)", fid, "f1.txt", int64(3), mtime)
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
