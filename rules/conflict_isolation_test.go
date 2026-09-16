package rules

import (
	"reflect"
	"testing"
)

// TDI-I12 conflict isolation minimum contract tests.
//
// Contract (central-supplied):
//  1. Duplicate collision is a structural CONFLICT.
//  2. CONFLICT scope is ONE stable subject coordinate.
//  3. A duplicate in one subject MUST NOT abort healthy sibling subjects in the batch.
//  4. The collided subject MUST remain visible as conflict evidence.
//  5. No arbitrary duplicate candidate may become the winning healthy member.
//  6. Duplicate evidence preserves (deterministically): reason_code = duplicate_role_in_row,
//     subject coordinate, role, candidates, source filenames.
//  7. Tori emits source/grouping facts only.

func i12RuleSet() RuleSet {
	return RuleSet{
		Delimiter:   []string{"_", "."},
		Header:      []string{"R1", "R2"},
		RowRules:    RowRules{MatchParts: []int{0, 1, 2, 4, 5, 6}},
		ColumnRules: ColumnRules{MatchParts: []int{3}},
	}
}

// i12HealthySubjectKeys returns the set of subject stable keys currently published as
// healthy, so a test can assert a conflicted coordinate never leaked into Healthy.
func i12HealthyRoleFiles(result IsolatedGroupingResult) []map[string]string {
	rows := make([]map[string]string, 0, len(result.Healthy))
	for _, members := range result.Healthy {
		rows = append(rows, members)
	}
	return rows
}

// I12-T01: a single subject with duplicate role candidates becomes a CONFLICT carrying
// duplicate_role_in_row evidence with candidates and source filenames preserved.
func TestI12_T01_IsolatedConflict(t *testing.T) {
	files := []string{
		"sample1_S1_L001_R1_001.fastq.gz",
		"sample1__S1_L001_R1_001.fastq.gz",
	}

	result := GroupFilesIsolated(files, i12RuleSet())

	if len(result.Healthy) != 0 {
		t.Fatalf("expected no healthy subjects, got %#v", result.Healthy)
	}
	if len(result.Conflicts) != 1 {
		t.Fatalf("expected exactly 1 conflicted subject, got %d: %#v", len(result.Conflicts), result.Conflicts)
	}

	conflict := result.Conflicts[0]
	if conflict.SubjectKey == "" || conflict.RowKey == "" {
		t.Fatalf("expected a populated subject coordinate, got %#v", conflict)
	}
	if len(conflict.SubjectComponents) == 0 {
		t.Fatalf("expected subject components to be preserved, got %#v", conflict)
	}
	if len(conflict.Roles) != 1 {
		t.Fatalf("expected exactly 1 role collision, got %#v", conflict.Roles)
	}

	evidence := conflict.Roles[0]
	if evidence.ReasonCode != "duplicate_role_in_row" {
		t.Fatalf("expected reason_code duplicate_role_in_row, got %q", evidence.ReasonCode)
	}
	if evidence.Role != "R1" {
		t.Fatalf("expected collided role R1, got %q", evidence.Role)
	}
	wantCandidates := []string{
		"sample1_S1_L001_R1_001.fastq.gz",
		"sample1__S1_L001_R1_001.fastq.gz",
	}
	if !reflect.DeepEqual(evidence.Candidates, wantCandidates) {
		t.Fatalf("unexpected candidates: got=%v want=%v", evidence.Candidates, wantCandidates)
	}
	if !reflect.DeepEqual(evidence.SourceFileNames, wantCandidates) {
		t.Fatalf("unexpected source filenames: got=%v want=%v", evidence.SourceFileNames, wantCandidates)
	}
	if !reflect.DeepEqual(conflict.SourceFileNames, wantCandidates) {
		t.Fatalf("expected subject source filenames preserved, got=%v want=%v", conflict.SourceFileNames, wantCandidates)
	}
}

// I12-T02: subject A is a duplicate conflict, subject B is healthy. A stays a conflict,
// B stays a normal healthy grouped result, with NO whole-batch abort.
func TestI12_T02_HealthySiblingSurvives(t *testing.T) {
	files := []string{
		// subject A: duplicate R1 -> conflict
		"sample1_S1_L001_R1_001.fastq.gz",
		"sample1__S1_L001_R1_001.fastq.gz",
		// subject B: healthy R1 + R2
		"sample2_S2_L001_R1_001.fastq.gz",
		"sample2_S2_L001_R2_001.fastq.gz",
	}

	result := GroupFilesIsolated(files, i12RuleSet())

	if len(result.Conflicts) != 1 {
		t.Fatalf("expected exactly 1 conflicted subject (A), got %d: %#v", len(result.Conflicts), result.Conflicts)
	}
	if result.Conflicts[0].Roles[0].Role != "R1" {
		t.Fatalf("expected subject A conflict on R1, got %#v", result.Conflicts[0])
	}

	if len(result.Healthy) != 1 {
		t.Fatalf("expected exactly 1 healthy sibling (B) to survive, got %#v", result.Healthy)
	}
	healthy := i12HealthyRoleFiles(result)[0]
	if healthy["R1"] != "sample2_S2_L001_R1_001.fastq.gz" || healthy["R2"] != "sample2_S2_L001_R2_001.fastq.gz" {
		t.Fatalf("expected healthy sibling B grouped with R1+R2, got %#v", healthy)
	}

	// Sibling B must not appear as conflict evidence, and A must not appear as healthy.
	for _, conflict := range result.Conflicts {
		for _, f := range conflict.SourceFileNames {
			if f == "sample2_S2_L001_R1_001.fastq.gz" || f == "sample2_S2_L001_R2_001.fastq.gz" {
				t.Fatalf("healthy sibling file leaked into conflict evidence: %q", f)
			}
		}
	}
}

// I12-T03: the same duplicate candidates in different input order yield canonical,
// deterministic conflict evidence with stable candidate ordering.
func TestI12_T03_DeterministicEvidence(t *testing.T) {
	orderA := []string{
		"sample1_S1_L001_R1_001.fastq.gz",
		"sample1__S1_L001_R1_001.fastq.gz",
		"sample1___S1_L001_R1_001.fastq.gz",
	}
	orderB := []string{
		"sample1___S1_L001_R1_001.fastq.gz",
		"sample1_S1_L001_R1_001.fastq.gz",
		"sample1__S1_L001_R1_001.fastq.gz",
	}

	resultA := GroupFilesIsolated(orderA, i12RuleSet())
	resultB := GroupFilesIsolated(orderB, i12RuleSet())

	if !reflect.DeepEqual(resultA.Conflicts, resultB.Conflicts) {
		t.Fatalf("expected deterministic conflict evidence across input orders:\nA=%#v\nB=%#v", resultA.Conflicts, resultB.Conflicts)
	}
	if len(resultA.Conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %#v", resultA.Conflicts)
	}
	want := []string{
		"sample1_S1_L001_R1_001.fastq.gz",
		"sample1__S1_L001_R1_001.fastq.gz",
		"sample1___S1_L001_R1_001.fastq.gz",
	}
	// deterministic canonical ordering is sorted, independent of input order.
	if got := resultA.Conflicts[0].Roles[0].Candidates; !reflect.DeepEqual(got, want) {
		t.Fatalf("expected canonical sorted candidate ordering, got %v", got)
	}
}

// I12-T04: two different subjects each with their own duplicate produce two
// independently-scoped conflicts, with no cross-subject evidence mixing, and a healthy
// sibling is preserved.
func TestI12_T04_MultipleIndependentConflicts(t *testing.T) {
	files := []string{
		// subject 1: duplicate R1
		"sample1_S1_L001_R1_001.fastq.gz",
		"sample1__S1_L001_R1_001.fastq.gz",
		// subject 2: duplicate R2
		"sample2_S2_L001_R2_001.fastq.gz",
		"sample2__S2_L001_R2_001.fastq.gz",
		// subject 3: healthy R1 + R2
		"sample3_S3_L001_R1_001.fastq.gz",
		"sample3_S3_L001_R2_001.fastq.gz",
	}

	result := GroupFilesIsolated(files, i12RuleSet())

	if len(result.Conflicts) != 2 {
		t.Fatalf("expected 2 independent conflicts, got %d: %#v", len(result.Conflicts), result.Conflicts)
	}
	if len(result.Healthy) != 1 {
		t.Fatalf("expected the healthy subject 3 to be preserved, got %#v", result.Healthy)
	}

	byRole := map[string][]string{}
	for _, conflict := range result.Conflicts {
		if len(conflict.Roles) != 1 {
			t.Fatalf("expected one role collision per subject, got %#v", conflict)
		}
		byRole[conflict.Roles[0].Role] = conflict.SourceFileNames
	}

	r1 := byRole["R1"]
	r2 := byRole["R2"]
	if r1 == nil || r2 == nil {
		t.Fatalf("expected one R1 and one R2 scoped conflict, got %#v", byRole)
	}
	// No cross-subject evidence mixing: subject 1's evidence holds only sample1 files, etc.
	for _, f := range r1 {
		if f != "sample1_S1_L001_R1_001.fastq.gz" && f != "sample1__S1_L001_R1_001.fastq.gz" {
			t.Fatalf("R1 conflict evidence mixed a foreign file: %q", f)
		}
	}
	for _, f := range r2 {
		if f != "sample2_S2_L001_R2_001.fastq.gz" && f != "sample2__S2_L001_R2_001.fastq.gz" {
			t.Fatalf("R2 conflict evidence mixed a foreign file: %q", f)
		}
	}

	// Conflicts are deterministically ordered by stable subject key.
	if result.Conflicts[0].SubjectKey >= result.Conflicts[1].SubjectKey {
		t.Fatalf("expected conflicts sorted by subject key, got %q then %q",
			result.Conflicts[0].SubjectKey, result.Conflicts[1].SubjectKey)
	}
}

// I12-T05: a conflicted subject with multiple candidates for one role never silently
// promotes a candidate to the healthy member; the subject is not downgraded to a
// grouped/healthy result and its conflict evidence stays visible.
func TestI12_T05_NoArbitraryWinner(t *testing.T) {
	files := []string{
		"sample1_S1_L001_R1_001.fastq.gz",
		"sample1__S1_L001_R1_001.fastq.gz",
		"sample1___S1_L001_R1_001.fastq.gz",
	}

	result := GroupFilesIsolated(files, i12RuleSet())

	if len(result.Healthy) != 0 {
		t.Fatalf("expected the conflicted subject NOT to be downgraded to healthy, got %#v", result.Healthy)
	}
	if len(result.Conflicts) != 1 {
		t.Fatalf("expected 1 visible conflict, got %#v", result.Conflicts)
	}

	evidence := result.Conflicts[0].Roles[0]
	if len(evidence.Candidates) != 3 {
		t.Fatalf("expected all 3 candidates preserved (no arbitrary winner), got %#v", evidence.Candidates)
	}

	// None of the candidates may appear as a published healthy member anywhere.
	for _, members := range i12HealthyRoleFiles(result) {
		for _, f := range members {
			for _, candidate := range evidence.Candidates {
				if f == candidate {
					t.Fatalf("a conflicted candidate silently became a healthy winner: %q", f)
				}
			}
		}
	}
}
