package rules

import (
	"reflect"
	"testing"
)

func TestI3M_PublicationSubjects_OrderIndependentAndConflictExcluded(t *testing.T) {
	rs := i12RuleSet()
	files := []string{
		"A_S1_L001_R1_001.fastq.gz",
		"A_S1_L001_R2_001.fastq.gz",
		"B_S1_L001_R1_001.fastq.gz",
		"B_S1_L001_R2_001.fastq.gz",
	}
	reversed := []string{files[3], files[2], files[1], files[0]}

	got, conflicts := PublicationSubjects(files, rs)
	again, _ := PublicationSubjects(reversed, rs)
	if len(conflicts) != 0 {
		t.Fatalf("unexpected conflicts: %+v", conflicts)
	}
	if !reflect.DeepEqual(got, again) {
		t.Fatalf("file order changed subjects:\n%+v\n%+v", got, again)
	}
	if len(got) != 2 || len(got[0].Members) != 2 || got[0].Members[0].ObservedKey >= got[0].Members[1].ObservedKey {
		t.Fatalf("subjects not grouped/sorted: %+v", got)
	}

	// A duplicate role in subject B isolates B only; A is still published.
	withDup := append(append([]string(nil), files...), "B_S1_L001_R1_001.fastq.gz.copy")
	healthy, conflicts := PublicationSubjects(withDup, rs)
	if len(conflicts) != 1 || len(healthy) != 1 || healthy[0].SubjectKey != got[0].SubjectKey {
		t.Fatalf("healthy=%+v conflicts=%+v", healthy, conflicts)
	}
}

// Codex P1 (schema validity): a conflict-free subject whose roles are not exactly the
// RuleSet header is excluded, as in the canonical FileBlock path (FilterGroupsByHeaders).
func TestI3M_PublicationSubjects_SchemaInvalidExcluded(t *testing.T) {
	rs := i12RuleSet()
	valid := []string{"A_S1_L001_R1_001.fastq.gz", "A_S1_L001_R2_001.fastq.gz"}
	invalid := map[string][]string{
		"missing role":    {"M_S1_L001_R1_001.fastq.gz"},
		"extra role":      {"X_S1_L001_R1_001.fastq.gz", "X_S1_L001_R2_001.fastq.gz", "X_S1_L001_R3_001.fastq.gz"},
		"unresolved role": {"U_S1_L001_R1_001.fastq.gz", "U_S1_L001_I1_001.fastq.gz"},
	}
	want, _ := PublicationSubjects(valid, rs)
	if len(want) != 1 {
		t.Fatalf("valid subject not published: %+v", want)
	}
	all := append([]string(nil), valid...)
	for name, files := range invalid {
		if got, _ := PublicationSubjects(append(append([]string(nil), valid...), files...), rs); !reflect.DeepEqual(got, want) {
			t.Fatalf("%s: subjects = %+v, want only the valid subject %+v", name, got, want)
		}
		all = append(all, files...)
	}
	got, conflicts := PublicationSubjects(all, rs)
	if len(conflicts) != 0 || !reflect.DeepEqual(got, want) {
		t.Fatalf("all invalid kinds: subjects = %+v conflicts = %+v, want only %+v", got, conflicts, want)
	}
}
