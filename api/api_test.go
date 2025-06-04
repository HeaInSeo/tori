package api

import (
    "os"
    "path/filepath"
    "testing"
)

func TestFileExistsExact(t *testing.T) {
    dir := t.TempDir()
    name := "test.txt"
    path := filepath.Join(dir, name)
    if err := os.WriteFile(path, []byte("data"), 0644); err != nil {
        t.Fatalf("failed to create file: %v", err)
    }
    exists, err := FileExistsExact(dir, name)
    if err != nil {
        t.Fatalf("FileExistsExact error: %v", err)
    }
    if !exists {
        t.Errorf("expected file to exist")
    }
}

func TestSearchFilesByPattern(t *testing.T) {
    dir := t.TempDir()
    os.WriteFile(filepath.Join(dir, "a.txt"), []byte(""), 0644)
    os.WriteFile(filepath.Join(dir, "b.txt"), []byte(""), 0644)

    files, err := SearchFilesByPattern(dir, "*.txt")
    if err != nil {
        t.Fatalf("SearchFilesByPattern error: %v", err)
    }
    if len(files) != 2 {
        t.Errorf("expected 2 files, got %d", len(files))
    }
}

func TestDeleteFilesByPattern(t *testing.T) {
    dir := t.TempDir()
    f1 := filepath.Join(dir, "a.txt")
    f2 := filepath.Join(dir, "b.txt")
    os.WriteFile(f1, []byte(""), 0644)
    os.WriteFile(f2, []byte(""), 0644)

    if err := DeleteFilesByPattern(dir, "*.txt"); err != nil {
        t.Fatalf("DeleteFilesByPattern error: %v", err)
    }
    if _, err := os.Stat(f1); !os.IsNotExist(err) {
        t.Errorf("expected %s to be removed", f1)
    }
    if _, err := os.Stat(f2); !os.IsNotExist(err) {
        t.Errorf("expected %s to be removed", f2)
    }
}

