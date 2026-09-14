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

	first, err := groupFilesStructured(files, ruleSet)
	if err != nil {
		t.Fatalf("groupFilesStructured first: %v", err)
	}
	second, err := groupFilesStructured(permuted, ruleSet)
	if err != nil {
		t.Fatalf("groupFilesStructured second: %v", err)
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

	baseGrouping, err := groupFilesStructured(base, ruleSet)
	if err != nil {
		t.Fatalf("groupFilesStructured base: %v", err)
	}
	insertedGrouping, err := groupFilesStructured(withInsertedSubject, ruleSet)
	if err != nil {
		t.Fatalf("groupFilesStructured inserted: %v", err)
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

	grouping, err := groupFilesStructured(files, ruleSet)
	if err != nil {
		t.Fatalf("groupFilesStructured: %v", err)
	}

	if len(grouping.groups) != 2 {
		t.Fatalf("expected underscore-colliding components to remain distinct, got %#v", grouping.groups)
	}
	gotComponents := [][]string{
		grouping.groups[0].coordinate.components,
		grouping.groups[1].coordinate.components,
	}
	wantComponents := [][]string{{"a", "b_c"}, {"a_b", "c"}}
	if !reflect.DeepEqual(gotComponents, wantComponents) {
		t.Fatalf("unexpected structured coordinates:\ngot  %#v\nwant %#v", gotComponents, wantComponents)
	}
	if grouping.groups[0].coordinate.stableKey() == grouping.groups[1].coordinate.stableKey() {
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
	structured, err := groupFilesStructured(files, ruleSet)
	if err != nil {
		t.Fatalf("groupFilesStructured: %v", err)
	}

	if len(legacy) != 1 {
		t.Fatalf("legacy GroupFiles should keep historical underscore-joined row grouping, got %#v", legacy)
	}
	if legacy[0]["R1"] != "a_b.c.R1.fastq" || legacy[0]["R2"] != "a.b_c.R2.fastq" {
		t.Fatalf("legacy GroupFiles row contents changed: %#v", legacy)
	}
	if len(structured.groups) != 2 {
		t.Fatalf("structured grouping should keep stable subjects distinct, got %#v", structured.groups)
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

	baseGrouping, err := groupFilesStructured(base, ruleSet)
	if err != nil {
		t.Fatalf("groupFilesStructured base: %v", err)
	}
	priorGrouping, err := groupFilesStructured(withPriorSubject, ruleSet)
	if err != nil {
		t.Fatalf("groupFilesStructured prior: %v", err)
	}

	baseAlpha, ok := i11aGroupByCoordinate(baseGrouping, "5:alpha|4:L001|3:001|5:fastq|2:gz")
	if !ok {
		t.Fatalf("alpha coordinate missing from base grouping")
	}
	priorAlpha, ok := i11aGroupByCoordinate(priorGrouping, "5:alpha|4:L001|3:001|5:fastq|2:gz")
	if !ok {
		t.Fatalf("alpha coordinate missing from prior-subject grouping")
	}
	if baseAlpha.legacyRowNumber == priorAlpha.legacyRowNumber {
		t.Fatalf("expected legacy row number to differ after prior subject insertion")
	}
	if !reflect.DeepEqual(baseAlpha.coordinate.components, priorAlpha.coordinate.components) {
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
	structured, err := groupFilesStructured(files, ruleSet)
	if err != nil {
		t.Fatalf("groupFilesStructured: %v", err)
	}
	adapted := legacyGroupsFromStructured(structured)

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

	grouping, err := groupFilesStructured([]string{"alpha_R1.fastq.gz"}, ruleSet)
	if err != nil {
		t.Fatalf("groupFilesStructured: %v", err)
	}
	if len(grouping.groups) != 1 {
		t.Fatalf("expected one explicit empty-coordinate group, got %d", len(grouping.groups))
	}
	if grouping.groups[0].coordinate.components == nil || len(grouping.groups[0].coordinate.components) != 0 {
		t.Fatalf("empty coordinate components should be explicit, got %#v", grouping.groups[0].coordinate.components)
	}
	if grouping.groups[0].coordinate.stableKey() != "" {
		t.Fatalf("unexpected empty coordinate stable key %q", grouping.groups[0].coordinate.stableKey())
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

func i11aCoordinateFacts(grouping structuredGroupingResult) map[string]map[string]string {
	facts := make(map[string]map[string]string, len(grouping.groups))
	for _, group := range grouping.groups {
		members := make(map[string]string, len(group.observedMembers))
		for role, fileName := range group.observedMembers {
			members[role] = fileName
		}
		facts[group.coordinate.stableKey()] = members
	}
	return facts
}

func i11aGroupByCoordinate(grouping structuredGroupingResult, key string) (structuredGroup, bool) {
	for _, group := range grouping.groups {
		if group.coordinate.stableKey() == key {
			return group, true
		}
	}
	return structuredGroup{}, false
}

func TestI11AStableSubjectCoordinate_DeterministicOrdering(t *testing.T) {
	ruleSet := i11aRuleSet()
	files := []string{
		"charlie_L001_R1_001.fastq.gz",
		"alpha_L001_R1_001.fastq.gz",
		"beta_L001_R1_001.fastq.gz",
	}

	grouping, err := groupFilesStructured(files, ruleSet)
	if err != nil {
		t.Fatalf("groupFilesStructured: %v", err)
	}
	keys := make([]string, 0, len(grouping.groups))
	for _, group := range grouping.groups {
		keys = append(keys, group.coordinate.stableKey())
	}
	sorted := append([]string(nil), keys...)
	sort.Strings(sorted)
	if !reflect.DeepEqual(keys, sorted) {
		t.Fatalf("structured groups are not deterministically sorted:\ngot  %#v\nwant %#v", keys, sorted)
	}
}
