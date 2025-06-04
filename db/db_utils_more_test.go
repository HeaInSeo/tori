package db

import (
	"os"
	"path/filepath"
	"testing"
)

// TestGetSubFolders verifies sub directory listing with exclusions.
func TestGetSubFolders(t *testing.T) {
	root := t.TempDir()
	os.Mkdir(filepath.Join(root, "a"), 0755)
	os.Mkdir(filepath.Join(root, "b"), 0755)
	os.Mkdir(filepath.Join(root, "skip"), 0755)
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
	os.Mkdir(sub, 0755)
	os.WriteFile(filepath.Join(sub, "f1.txt"), []byte("abc"), 0644)
	os.WriteFile(filepath.Join(sub, "f2.csv"), []byte("d"), 0644)
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
	defer db.Close()
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
	db.QueryRow("SELECT COUNT(*) FROM folders").Scan(&n)
	if n != 0 {
		t.Errorf("expected 0 rows, got %d", n)
	}
}

func TestCompareFoldersMatch(t *testing.T) {
	db := SetupInMemoryDB(t)
	defer db.Close()
	root := t.TempDir()
	sub := filepath.Join(root, "dir")
	os.Mkdir(sub, 0755)
	// create file to give size
	os.WriteFile(filepath.Join(sub, "f1.txt"), []byte("hi"), 0644)
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
	defer db.Close()
	root := t.TempDir()
	// folder path
	folder := root
	// create file on disk
	os.WriteFile(filepath.Join(folder, "f1.txt"), []byte("abc"), 0644)
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
