package rules

import (
	"reflect"
	"sort"
	"testing"
)

func TestI11AStableSubjectCoordinate_FilePermutation(t *testing.T) {
	ruleSet := i11aRuleSet()
	files := []string{
		"alpha_L001_R1_001.fastq.gz",
		"beta_L001_R2_001.fastq.gz",
		"beta_L001_R1_001.fastq.gz",
		"alpha_L001_R2_001.fastq.gz",
	}
	permuted := []string{
		"beta_L001_R1_001.fastq.gz",
		"alpha_L001_R2_001.fastq.gz",
		"alpha_L001_R1_001.fastq.gz",
		"beta_L001_R2_001.fastq.gz",
	}

	first, err := GroupFilesStructured(files, ruleSet)
	if err != nil {
		t.Fatalf("GroupFilesStructured first: %v", err)
	}
	second, err := GroupFilesStructured(permuted, ruleSet)
	if err != nil {
		t.Fatalf("GroupFilesStructured second: %v", err)
	}

	if got, want := i11aCoordinateFacts(first), i11aCoordinateFacts(second); !reflect.DeepEqual(got, want) {
		t.Fatalf("coordinate facts changed under filename permutation:\ngot  %#v\nwant %#v", got, want)
	}
}

func TestI11AStableSubjectCoordinate_UnrelatedInsertionDoesNotChangeExistingCoordinates(t *testing.T) {
	ruleSet := i11aRuleSet()
	base := []string{
		"alpha_L001_R1_001.fastq.gz",
		"alpha_L001_R2_001.fastq.gz",
		"beta_L001_R1_001.fastq.gz",
		"beta_L001_R2_001.fastq.gz",
	}
	withInsertedSubject := []string{
		"aaa_L001_R1_001.fastq.gz",
		"aaa_L001_R2_001.fastq.gz",
		"alpha_L001_R1_001.fastq.gz",
		"alpha_L001_R2_001.fastq.gz",
		"beta_L001_R1_001.fastq.gz",
		"beta_L001_R2_001.fastq.gz",
	}

	baseGrouping, err := GroupFilesStructured(base, ruleSet)
	if err != nil {
		t.Fatalf("GroupFilesStructured base: %v", err)
	}
	insertedGrouping, err := GroupFilesStructured(withInsertedSubject, ruleSet)
	if err != nil {
		t.Fatalf("GroupFilesStructured inserted: %v", err)
	}

	baseFacts := i11aCoordinateFacts(baseGrouping)
	insertedFacts := i11aCoordinateFacts(insertedGrouping)
	for key, want := range baseFacts {
		if got := insertedFacts[key]; !reflect.DeepEqual(got, want) {
			t.Fatalf("existing coordinate %q changed after unrelated subject insertion:\ngot  %#v\nwant %#v", key, got, want)
		}
	}
}

func TestI11AStableSubjectCoordinate_UnderscoreJoinCollisionRemainsDistinct(t *testing.T) {
	ruleSet := RuleSet{
		Delimiter:   []string{"."},
		Header:      []string{"R1", "R2"},
		RowRules:    RowRules{MatchParts: []int{0, 1}},
		ColumnRules: ColumnRules{MatchParts: []int{2}},
	}
	files := []string{
		"a_b.c.R1.fastq",
		"a.b_c.R1.fastq",
	}

	grouping, err := GroupFilesStructured(files, ruleSet)
	if err != nil {
		t.Fatalf("GroupFilesStructured: %v", err)
	}

	if len(grouping.Groups) != 2 {
		t.Fatalf("expected underscore-colliding components to remain distinct, got %#v", grouping.Groups)
	}
	gotComponents := [][]string{
		grouping.Groups[0].Coordinate.Components,
		grouping.Groups[1].Coordinate.Components,
	}
	wantComponents := [][]string{{"a", "b_c"}, {"a_b", "c"}}
	if !reflect.DeepEqual(gotComponents, wantComponents) {
		t.Fatalf("unexpected structured coordinates:\ngot  %#v\nwant %#v", gotComponents, wantComponents)
	}
	if grouping.Groups[0].Coordinate.stableKey() == grouping.Groups[1].Coordinate.stableKey() {
		t.Fatalf("stable keys collided for distinct structured coordinates")
	}
}

func TestI11AStableSubjectCoordinate_GroupFilesPreservesLegacyUnderscoreCollision(t *testing.T) {
	ruleSet := RuleSet{
		Delimiter:   []string{"."},
		Header:      []string{"R1", "R2"},
		RowRules:    RowRules{MatchParts: []int{0, 1}},
		ColumnRules: ColumnRules{MatchParts: []int{2}},
	}
	files := []string{
		"a_b.c.R1.fastq",
		"a.b_c.R2.fastq",
	}

	legacy, err := GroupFiles(files, ruleSet)
	if err != nil {
		t.Fatalf("GroupFiles: %v", err)
	}
	structured, err := GroupFilesStructured(files, ruleSet)
	if err != nil {
		t.Fatalf("GroupFilesStructured: %v", err)
	}

	if len(legacy) != 1 {
		t.Fatalf("legacy GroupFiles should keep historical underscore-joined row grouping, got %#v", legacy)
	}
	if legacy[0]["R1"] != "a_b.c.R1.fastq" || legacy[0]["R2"] != "a.b_c.R2.fastq" {
		t.Fatalf("legacy GroupFiles row contents changed: %#v", legacy)
	}
	if len(structured.Groups) != 2 {
		t.Fatalf("structured grouping should keep stable subjects distinct, got %#v", structured.Groups)
	}
}

func TestI11AStableSubjectCoordinate_LegacyRowNumberMayDiffer(t *testing.T) {
	ruleSet := i11aRuleSet()
	base := []string{
		"alpha_L001_R1_001.fastq.gz",
		"alpha_L001_R2_001.fastq.gz",
		"beta_L001_R1_001.fastq.gz",
		"beta_L001_R2_001.fastq.gz",
	}
	withPriorSubject := []string{
		"aaa_L001_R1_001.fastq.gz",
		"aaa_L001_R2_001.fastq.gz",
		"alpha_L001_R1_001.fastq.gz",
		"alpha_L001_R2_001.fastq.gz",
		"beta_L001_R1_001.fastq.gz",
		"beta_L001_R2_001.fastq.gz",
	}

	baseGrouping, err := GroupFilesStructured(base, ruleSet)
	if err != nil {
		t.Fatalf("GroupFilesStructured base: %v", err)
	}
	priorGrouping, err := GroupFilesStructured(withPriorSubject, ruleSet)
	if err != nil {
		t.Fatalf("GroupFilesStructured prior: %v", err)
	}

	baseAlpha, ok := i11aGroupByCoordinate(baseGrouping, "5:alpha|4:L001|3:001|5:fastq|2:gz")
	if !ok {
		t.Fatalf("alpha coordinate missing from base grouping")
	}
	priorAlpha, ok := i11aGroupByCoordinate(priorGrouping, "5:alpha|4:L001|3:001|5:fastq|2:gz")
	if !ok {
		t.Fatalf("alpha coordinate missing from prior-subject grouping")
	}
	if baseAlpha.LegacyRowNumber == priorAlpha.LegacyRowNumber {
		t.Fatalf("expected legacy row number to differ after prior subject insertion")
	}
	if !reflect.DeepEqual(baseAlpha.Coordinate.Components, priorAlpha.Coordinate.Components) {
		t.Fatalf("stable coordinate changed while legacy row number changed")
	}
}

func TestI11AStableSubjectCoordinate_LegacyCompatibilityAdapter(t *testing.T) {
	ruleSet := i11aRuleSet()
	files := []string{
		"beta_L001_R1_001.fastq.gz",
		"beta_L001_R2_001.fastq.gz",
		"alpha_L001_R1_001.fastq.gz",
		"alpha_L001_R2_001.fastq.gz",
	}

	legacy, err := GroupFiles(files, ruleSet)
	if err != nil {
		t.Fatalf("GroupFiles: %v", err)
	}
	structured, err := GroupFilesStructured(files, ruleSet)
	if err != nil {
		t.Fatalf("GroupFilesStructured: %v", err)
	}
	adapted := LegacyGroupsFromStructured(structured)

	if !reflect.DeepEqual(legacy, adapted) {
		t.Fatalf("legacy adapter changed GroupFiles behavior:\ngot  %#v\nwant %#v", adapted, legacy)
	}
	if legacy[0]["R1"] != "beta_L001_R1_001.fastq.gz" || legacy[1]["R1"] != "alpha_L001_R1_001.fastq.gz" {
		t.Fatalf("legacy row encounter ordering changed: %#v", legacy)
	}
}

func TestI11AStableSubjectCoordinate_EmptyCoordinateIsExplicit(t *testing.T) {
	ruleSet := RuleSet{
		Delimiter:   []string{"_", "."},
		Header:      []string{"R1"},
		RowRules:    RowRules{MatchParts: []int{99}},
		ColumnRules: ColumnRules{MatchParts: []int{1}},
	}

	grouping, err := GroupFilesStructured([]string{"alpha_R1.fastq.gz"}, ruleSet)
	if err != nil {
		t.Fatalf("GroupFilesStructured: %v", err)
	}
	if len(grouping.Groups) != 1 {
		t.Fatalf("expected one explicit empty-coordinate group, got %d", len(grouping.Groups))
	}
	if grouping.Groups[0].Coordinate.Components == nil || len(grouping.Groups[0].Coordinate.Components) != 0 {
		t.Fatalf("empty coordinate components should be explicit, got %#v", grouping.Groups[0].Coordinate.Components)
	}
	if grouping.Groups[0].Coordinate.stableKey() != "" {
		t.Fatalf("unexpected empty coordinate stable key %q", grouping.Groups[0].Coordinate.stableKey())
	}
}

func i11aRuleSet() RuleSet {
	return RuleSet{
		Delimiter:   []string{"_", "."},
		Header:      []string{"R1", "R2"},
		RowRules:    RowRules{MatchParts: []int{0, 1, 3, 4, 5}},
		ColumnRules: ColumnRules{MatchParts: []int{2}},
	}
}

func i11aCoordinateFacts(grouping StructuredGroupingResult) map[string]map[string]string {
	facts := make(map[string]map[string]string, len(grouping.Groups))
	for _, group := range grouping.Groups {
		members := make(map[string]string, len(group.ObservedMembers))
		for role, fileName := range group.ObservedMembers {
			members[role] = fileName
		}
		facts[group.Coordinate.stableKey()] = members
	}
	return facts
}

func i11aGroupByCoordinate(grouping StructuredGroupingResult, key string) (StructuredGroup, bool) {
	for _, group := range grouping.Groups {
		if group.Coordinate.stableKey() == key {
			return group, true
		}
	}
	return StructuredGroup{}, false
}

func TestI11AStableSubjectCoordinate_DeterministicOrdering(t *testing.T) {
	ruleSet := i11aRuleSet()
	files := []string{
		"charlie_L001_R1_001.fastq.gz",
		"alpha_L001_R1_001.fastq.gz",
		"beta_L001_R1_001.fastq.gz",
	}

	grouping, err := GroupFilesStructured(files, ruleSet)
	if err != nil {
		t.Fatalf("GroupFilesStructured: %v", err)
	}
	keys := make([]string, 0, len(grouping.Groups))
	for _, group := range grouping.Groups {
		keys = append(keys, group.Coordinate.stableKey())
	}
	sorted := append([]string(nil), keys...)
	sort.Strings(sorted)
	if !reflect.DeepEqual(keys, sorted) {
		t.Fatalf("structured groups are not deterministically sorted:\ngot  %#v\nwant %#v", keys, sorted)
	}
}
