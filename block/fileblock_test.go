package block

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/seoyhaein/tori/rules"
)

func TestGenerateFileBlock_PreservesDuplicateCollisionTypedError(t *testing.T) {
	dir := t.TempDir()
	ruleSet := rules.RuleSet{
		Delimiter:   []string{"_", "."},
		Header:      []string{"R1", "R2"},
		RowRules:    rules.RowRules{MatchParts: []int{0, 1, 2, 4, 5, 6}},
		ColumnRules: rules.ColumnRules{MatchParts: []int{3}},
	}

	data, err := json.Marshal(ruleSet)
	if err != nil {
		t.Fatalf("marshal rule set: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "rule.json"), data, 0644); err != nil {
		t.Fatalf("write rule.json: %v", err)
	}

	files := []string{
		"sample1_S1_L001_R1_001.fastq.gz",
		"sample1__S1_L001_R1_001.fastq.gz",
	}

	_, err = GenerateFileBlock(dir, files)
	if err == nil {
		t.Fatalf("expected duplicate collision error, got nil")
	}

	var dupErr *rules.DuplicateCollisionError
	if !errors.As(err, &dupErr) {
		t.Fatalf("expected DuplicateCollisionError via errors.As, got %T", err)
	}
	if len(dupErr.Entries) == 0 {
		t.Fatalf("expected non-empty duplicate entries")
	}

	entry := dupErr.Entries[0]
	if entry.ReasonCode != "duplicate_role_in_row" || entry.RoleKey != "R1" {
		t.Fatalf("expected duplicate structure information to survive wrapping: reason=%q role=%q", entry.ReasonCode, entry.RoleKey)
	}
}
