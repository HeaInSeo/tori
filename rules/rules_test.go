package rules

import (
    "os"
    "path/filepath"
    "reflect"
    "testing"
)

func TestSplitFileName(t *testing.T) {
    got := splitFileName("sample1_S1_L001_R1_001.fastq.gz", []string{"_", ".fastq.gz"})
    want := []string{"sample1", "S1", "L001", "R1", "001"}
    if !reflect.DeepEqual(got, want) {
        t.Errorf("splitFileName mismatch. got %v want %v", got, want)
    }
}

func TestGroupFilesAndFilterGroups(t *testing.T) {
    files := []string{
        "sample1_S1_L001_R1_001.fastq.gz",
        "sample1_S1_L002_R1_001.fastq.gz",
        "sample2_S2_L001_R1_001.fastq.gz",
    }
    rs := RuleSet{
        Delimiter: []string{"_", ".fastq.gz"},
        RowRules:    RowRules{MatchParts: []int{0,1}},
        ColumnRules: ColumnRules{MatchParts: []int{2}},
    }
    grouped, err := GroupFiles(files, rs)
    if err != nil {
        t.Fatalf("GroupFiles returned error: %v", err)
    }
    valid, invalid := FilterGroups(grouped, 2)
    if len(valid) != 1 {
        t.Errorf("expected 1 valid group, got %d", len(valid))
    }
    if len(invalid) != 1 {
        t.Errorf("expected 1 invalid group, got %d", len(invalid))
    }
}

func TestIsValidRuleSet(t *testing.T) {
    rs := RuleSet{
        RowRules:    RowRules{MatchParts: []int{0,1}},
        ColumnRules: ColumnRules{MatchParts: []int{1}},
    }
    if IsValidRuleSet(rs) {
        t.Errorf("expected rule set to be invalid due to duplicate index")
    }
}

func TestListFilesExclude(t *testing.T) {
    dir := t.TempDir()
    os.WriteFile(filepath.Join(dir, "keep.txt"), []byte(""), 0644)
    os.WriteFile(filepath.Join(dir, "skip.json"), []byte(""), 0644)
    os.WriteFile(filepath.Join(dir, "invalid_files"), []byte(""), 0644)

    files, err := ListFilesExclude(dir, []string{"*.json", "invalid_files"})
    if err != nil {
        t.Fatalf("ListFilesExclude error: %v", err)
    }
    if len(files) != 1 || files[0] != "keep.txt" {
        t.Errorf("unexpected files: %v", files)
    }
}

