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
