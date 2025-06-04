package api

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// helper to create rule.json and sample files
func setupRuleDir(t *testing.T) (string, []string) {
	t.Helper()
	dir := t.TempDir()
	rs := map[string]any{
		"version":     "1",
		"delimiter":   []string{"_", ".txt"},
		"header":      []string{"H1", "H2"},
		"rowRules":    map[string]any{"matchParts": []int{0}},
		"columnRules": map[string]any{"matchParts": []int{1}},
		"sizeRules":   map[string]any{"minSize": 0, "maxSize": 1000},
	}
	b, _ := json.Marshal(rs)
	if err := os.WriteFile(filepath.Join(dir, "rule.json"), b, 0644); err != nil {
		t.Fatalf("write rule.json: %v", err)
	}
	files := []string{"r1_c1.txt", "r1_c2.txt"}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0644); err != nil {
			t.Fatalf("write file: %v", err)
		}
	}
	return dir, files
}

func TestGenerateFileBlock(t *testing.T) {
	dir, files := setupRuleDir(t)
	fb, err := GenerateFileBlock(dir, files)
	if err != nil {
		t.Fatalf("GenerateFileBlock error: %v", err)
	}
	if fb.BlockId != dir {
		t.Errorf("block id mismatch: %s", fb.BlockId)
	}
	if len(fb.Rows) != 1 {
		t.Errorf("expected 1 row, got %d", len(fb.Rows))
	}
}

func TestConvertFolderFilesToFileBlocks(t *testing.T) {
	dir, files := setupRuleDir(t)
	ff := [][]string{{dir}}
	ff[0] = append(ff[0], files...)
	fbs, err := ConvertFolderFilesToFileBlocks(ff, []string{"H1", "H2"})
	if err != nil {
		t.Fatalf("ConvertFolderFilesToFileBlocks error: %v", err)
	}
	if len(fbs) != 1 {
		t.Fatalf("expected 1 fileblock, got %d", len(fbs))
	}
	if fbs[0].BlockId != dir {
		t.Errorf("block id mismatch")
	}
}
