package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRemoveDBFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "test.db")
	if err := os.WriteFile(f, []byte("data"), 0644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}
	if err := RemoveDBFile(f); err != nil {
		t.Fatalf("RemoveDBFile error: %v", err)
	}
	if _, err := os.Stat(f); !os.IsNotExist(err) {
		t.Errorf("file should be removed")
	}
}
