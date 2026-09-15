package rules

import (
	"reflect"
	"sort"
	"testing"
)

func TestI11BSubjectOutcome_R1OnlySubjectSurvivesWithMissingR2(t *testing.T) {
	ruleSet := i11bPairEndRuleSet()

	grouping, err := groupFilesStructured([]string{"alpha_L001_R1_001.fastq.gz"}, ruleSet)
	if err != nil {
		t.Fatalf("groupFilesStructured: %v", err)
	}
	if len(grouping.groups) != 1 {
		t.Fatalf("expected incomplete subject to survive, got %#v", grouping.groups)
	}

	group := grouping.groups[0]
	if got, want := group.coordinate.stableKey(), "5:alpha|4:L001|3:001|5:fastq|2:gz"; got != want {
		t.Fatalf("unexpected stable subject coordinate: got %q want %q", got, want)
	}
	if got, want := group.outcome.observedMembers["R1"], "alpha_L001_R1_001.fastq.gz"; got != want {
		t.Fatalf("expected R1 observation to survive: got %q want %q", got, want)
	}
	if !reflect.DeepEqual(group.outcome.missingRequiredRoles, []string{"R2"}) {
		t.Fatalf("expected missing R2 fact, got %#v", group.outcome.missingRequiredRoles)
	}

	legacyRows := legacyGroupsFromStructured(grouping)
	valid, invalid := FilterGroupsByHeaders(legacyRows, ruleSet.Header)
	if len(valid) != 0 || len(invalid) != 1 {
		t.Fatalf("legacy projection should still filter incomplete row: valid=%#v invalid=%#v", valid, invalid)
	}
}

func TestI11BSubjectOutcome_ExtraObservedRolesRemainAttachedToSubject(t *testing.T) {
	ruleSet := i11bPairEndRuleSet()
	files := []string{
		"alpha_L001_R1_001.fastq.gz",
		"alpha_L001_R2_001.fastq.gz",
		"alpha_L001_I1_001.fastq.gz",
		"alpha_L001_I2_001.fastq.gz",
	}

	grouping, err := groupFilesStructured(files, ruleSet)
	if err != nil {
		t.Fatalf("groupFilesStructured: %v", err)
	}
	group := onlyI11BGroup(t, grouping)

	gotExtra := sortedMapKeys(group.outcome.extraObservedMembers)
	if !reflect.DeepEqual(gotExtra, []string{"I1", "I2"}) {
		t.Fatalf("expected multiple extra observations to survive, got %#v", group.outcome.extraObservedMembers)
	}
	if group.outcome.extraObservedMembers["I1"] != "alpha_L001_I1_001.fastq.gz" {
		t.Fatalf("expected extra I1 file context to survive, got %#v", group.outcome.extraObservedMembers)
	}
	if len(group.outcome.missingRequiredRoles) != 0 {
		t.Fatalf("did not expect missing facts on complete R1/R2 subject, got %#v", group.outcome.missingRequiredRoles)
	}
}

func TestI11BSubjectOutcome_UnresolvedNormalizationIsDistinctFromMissing(t *testing.T) {
	ruleSet := i11bPairEndRuleSet()
	files := []string{
		"alpha_L001_R1_001.fastq.gz",
		"alpha_L001_RX_001.fastq.gz",
	}

	grouping, err := groupFilesStructured(files, ruleSet)
	if err != nil {
		t.Fatalf("groupFilesStructured: %v", err)
	}
	group := onlyI11BGroup(t, grouping)

	if !reflect.DeepEqual(group.outcome.missingRequiredRoles, []string{"R2"}) {
		t.Fatalf("expected missing R2 to remain separate, got %#v", group.outcome.missingRequiredRoles)
	}
	if got := sortedMapKeys(group.outcome.unresolvedObservedMembers); !reflect.DeepEqual(got, []string{"RX"}) {
		t.Fatalf("expected unresolved RX observation, got %#v", group.outcome.unresolvedObservedMembers)
	}
	if got := sortedMapKeys(group.outcome.extraObservedMembers); !reflect.DeepEqual(got, []string{"RX"}) {
		t.Fatalf("expected unresolved RX also to remain an extra observed role, got %#v", group.outcome.extraObservedMembers)
	}

	reasonCodes := i11bReasonCodes(group.outcome.facts)
	if reasonCodes["missing_required_role"] != 1 || reasonCodes["unresolved_observed_role"] != 1 {
		t.Fatalf("missing and unresolved facts were not separately represented: %#v", group.outcome.facts)
	}
}

func TestI11BSubjectOutcome_LegacyProjectionCompatibility(t *testing.T) {
	ruleSet := i11bPairEndRuleSet()
	files := []string{
		"alpha_L001_R1_001.fastq.gz",
		"alpha_L001_R2_001.fastq.gz",
		"beta_L001_R1_001.fastq.gz",
		"beta_L001_RX_001.fastq.gz",
	}

	legacy, err := GroupFiles(files, ruleSet)
	if err != nil {
		t.Fatalf("GroupFiles: %v", err)
	}
	structured, err := groupFilesStructured(files, ruleSet)
	if err != nil {
		t.Fatalf("groupFilesStructured: %v", err)
	}

	legacyValid, legacyInvalid := FilterGroupsByHeaders(legacy, ruleSet.Header)
	projectedValid, projectedInvalid := FilterGroupsByHeaders(legacyGroupsFromStructured(structured), ruleSet.Header)
	if !reflect.DeepEqual(projectedValid, legacyValid) || !reflect.DeepEqual(projectedInvalid, legacyInvalid) {
		t.Fatalf("legacy projection changed:\nprojected valid=%#v invalid=%#v\nlegacy valid=%#v invalid=%#v", projectedValid, projectedInvalid, legacyValid, legacyInvalid)
	}
	if len(legacyValid) != 1 || len(legacyInvalid) != 1 {
		t.Fatalf("expected historical exact-header subset to remain one valid and one invalid row, valid=%#v invalid=%#v", legacyValid, legacyInvalid)
	}
}

func TestI11BSubjectOutcome_DoesNotEmitPipelineVerdicts(t *testing.T) {
	ruleSet := i11bPairEndRuleSet()
	grouping, err := groupFilesStructured([]string{
		"alpha_L001_R1_001.fastq.gz",
		"alpha_L001_RX_001.fastq.gz",
	}, ruleSet)
	if err != nil {
		t.Fatalf("groupFilesStructured: %v", err)
	}

	banned := map[string]struct{}{
		"runnable":              {},
		"binding_success":       {},
		"required_slot_failure": {},
		"authorization_failure": {},
		"compatibility_failure": {},
		"bound":                 {},
	}
	for _, fact := range onlyI11BGroup(t, grouping).outcome.facts {
		if _, found := banned[fact.reasonCode]; found {
			t.Fatalf("subject outcome fact emitted Pipeline verdict %q: %#v", fact.reasonCode, fact)
		}
	}
}

func TestI11BSubjectOutcome_I11AStabilityRegression(t *testing.T) {
	ruleSet := i11bPairEndRuleSet()
	base := []string{
		"alpha_L001_R1_001.fastq.gz",
		"beta_L001_R1_001.fastq.gz",
		"alpha_L001_RX_001.fastq.gz",
	}
	permuted := []string{
		"alpha_L001_RX_001.fastq.gz",
		"alpha_L001_R1_001.fastq.gz",
		"beta_L001_R1_001.fastq.gz",
	}
	withUnrelated := []string{
		"aaa_L001_R1_001.fastq.gz",
		"aaa_L001_R2_001.fastq.gz",
		"alpha_L001_R1_001.fastq.gz",
		"beta_L001_R1_001.fastq.gz",
		"alpha_L001_RX_001.fastq.gz",
	}

	baseFacts := mustI11BOutcomeFactsByCoordinate(t, base, ruleSet)
	permutedFacts := mustI11BOutcomeFactsByCoordinate(t, permuted, ruleSet)
	repeatedFacts := mustI11BOutcomeFactsByCoordinate(t, base, ruleSet)
	insertedFacts := mustI11BOutcomeFactsByCoordinate(t, withUnrelated, ruleSet)

	if !reflect.DeepEqual(baseFacts, permutedFacts) {
		t.Fatalf("outcome facts changed under input permutation:\ngot  %#v\nwant %#v", permutedFacts, baseFacts)
	}
	if !reflect.DeepEqual(baseFacts, repeatedFacts) {
		t.Fatalf("outcome facts changed under repeated execution:\ngot  %#v\nwant %#v", repeatedFacts, baseFacts)
	}
	for coordinate, want := range baseFacts {
		if got := insertedFacts[coordinate]; !reflect.DeepEqual(got, want) {
			t.Fatalf("existing subject outcome changed after unrelated insertion for %q:\ngot  %#v\nwant %#v", coordinate, got, want)
		}
	}
}

func TestI11BSubjectOutcome_FactOrderingIsDeterministic(t *testing.T) {
	ruleSet := i11bPairEndRuleSet()
	files := []string{
		"alpha_L001_I2_001.fastq.gz",
		"alpha_L001_RX_001.fastq.gz",
		"alpha_L001_R1_001.fastq.gz",
	}

	first := mustI11BOutcomeFactsByCoordinate(t, files, ruleSet)
	second := mustI11BOutcomeFactsByCoordinate(t, files, ruleSet)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("outcome ordering changed across repeated evaluation:\ngot  %#v\nwant %#v", second, first)
	}
}

func TestI11BSubjectOutcome_EmptyObservedMemberSetIsNotReachable(t *testing.T) {
	grouping, err := groupFilesStructured(nil, i11bPairEndRuleSet())
	if err != nil {
		t.Fatalf("groupFilesStructured: %v", err)
	}
	if len(grouping.groups) != 0 {
		t.Fatalf("empty input should not synthesize an empty observed subject, got %#v", grouping.groups)
	}
}

func i11bPairEndRuleSet() RuleSet {
	return RuleSet{
		Delimiter:   []string{"_", "."},
		Header:      []string{"R1", "R2"},
		RowRules:    RowRules{MatchParts: []int{0, 1, 3, 4, 5}},
		ColumnRules: ColumnRules{MatchParts: []int{2}},
		RoleNormalization: map[string]string{
			"R1": "R1",
			"R2": "R2",
			"I1": "I1",
			"I2": "I2",
		},
	}
}

func onlyI11BGroup(t *testing.T, grouping structuredGroupingResult) structuredGroup {
	t.Helper()
	if len(grouping.groups) != 1 {
		t.Fatalf("expected one structured group, got %#v", grouping.groups)
	}
	return grouping.groups[0]
}

func mustI11BOutcomeFactsByCoordinate(t *testing.T, fileNames []string, ruleSet RuleSet) map[string][]subjectOutcomeFact {
	t.Helper()
	grouping, err := groupFilesStructured(fileNames, ruleSet)
	if err != nil {
		t.Fatalf("groupFilesStructured: %v", err)
	}
	factsByCoordinate := make(map[string][]subjectOutcomeFact, len(grouping.groups))
	for _, group := range grouping.groups {
		factsByCoordinate[group.coordinate.stableKey()] = append([]subjectOutcomeFact(nil), group.outcome.facts...)
	}
	return factsByCoordinate
}

func sortedMapKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func i11bReasonCodes(facts []subjectOutcomeFact) map[string]int {
	counts := make(map[string]int)
	for _, fact := range facts {
		counts[fact.reasonCode]++
	}
	return counts
}
