package rules

import (
	"encoding/json"
	"testing"
)

// TDI-I4F acceptance tests T01–T03: deterministic classification-semantics revision
// identity over the parsed+validated RuleSet behavior (not Version, path, mtime, or
// raw JSON formatting).

func mustUnmarshalRuleSet(t *testing.T, s string) RuleSet {
	t.Helper()
	var rs RuleSet
	if err := json.Unmarshal([]byte(s), &rs); err != nil {
		t.Fatalf("unmarshal rule set: %v", err)
	}
	return rs
}

func mustFreeze(t *testing.T, rs RuleSet) string {
	t.Helper()
	_, rev, err := FreezeRuleSet(rs)
	if err != nil {
		t.Fatalf("FreezeRuleSet: %v", err)
	}
	return rev
}

// I4F-T01: equivalent parsed semantics with JSON whitespace/key-order (and role-map
// order) differences produce the SAME revision identity.
func TestI4F_T01_DeterministicSemanticIdentity(t *testing.T) {
	a := `{"version":"1","delimiter":["_","."],"header":["R1","R2"],` +
		`"rowRules":{"matchParts":[0,1,2,4,5,6]},"columnRules":{"matchParts":[3]},` +
		`"sizeRules":{"minSize":0,"maxSize":1000},"roleNormalization":{"x":"X","y":"Y"}}`
	b := `{
		"columnRules": { "matchParts": [3] },
		"sizeRules":   { "maxSize": 1000, "minSize": 0 },
		"header":      ["R1", "R2"],
		"roleNormalization": { "y": "Y", "x": "X" },
		"delimiter":   ["_", "."],
		"rowRules":    { "matchParts": [0, 1, 2, 4, 5, 6] },
		"version":     "1"
	}`
	revA := mustFreeze(t, mustUnmarshalRuleSet(t, a))
	revB := mustFreeze(t, mustUnmarshalRuleSet(t, b))
	if revA != revB {
		t.Fatalf("equivalent semantics produced different revisions: %s vs %s", revA, revB)
	}
}

// I4F-T02: changing a grouping/role-affecting field produces a distinct revision.
func TestI4F_T02_SemanticChangeChangesRevision(t *testing.T) {
	base := RuleSet{
		Delimiter:   []string{"_", "."},
		Header:      []string{"R1", "R2"},
		RowRules:    RowRules{MatchParts: []int{0, 1, 2, 4, 5, 6}},
		ColumnRules: ColumnRules{MatchParts: []int{3}},
		SizeRules:   SizeRules{MinSize: 0, MaxSize: 1000},
	}
	baseRev := mustFreeze(t, base)

	cases := map[string]func(RuleSet) RuleSet{
		"delimiter":         func(r RuleSet) RuleSet { r.Delimiter = []string{"-"}; return r },
		"header order":      func(r RuleSet) RuleSet { r.Header = []string{"R2", "R1"}; return r },
		"rowRules":          func(r RuleSet) RuleSet { r.RowRules = RowRules{MatchParts: []int{0, 1}}; return r },
		"columnRules":       func(r RuleSet) RuleSet { r.ColumnRules = ColumnRules{MatchParts: []int{7}}; return r },
		"sizeRules":         func(r RuleSet) RuleSet { r.SizeRules = SizeRules{MinSize: 1, MaxSize: 1000}; return r },
		"roleNormalization": func(r RuleSet) RuleSet { r.RoleNormalization = map[string]string{"a": "B"}; return r },
	}
	for name, mut := range cases {
		t.Run(name, func(t *testing.T) {
			changed := mut(RuleSet{
				Delimiter:   append([]string(nil), base.Delimiter...),
				Header:      append([]string(nil), base.Header...),
				RowRules:    RowRules{MatchParts: append([]int(nil), base.RowRules.MatchParts...)},
				ColumnRules: ColumnRules{MatchParts: append([]int(nil), base.ColumnRules.MatchParts...)},
				SizeRules:   base.SizeRules,
			})
			rev := mustFreeze(t, changed)
			if rev == baseRev {
				t.Fatalf("changing %s did not change the revision (%s)", name, rev)
			}
		})
	}
}

// I4F-T03: Version is not the identity authority. Two different semantic payloads with
// the SAME Version must not collapse to one revision; the same semantics under a
// DIFFERENT Version must stay one revision.
func TestI4F_T03_VersionIsNotSoleAuthority(t *testing.T) {
	semA := RuleSet{
		Version:     "1",
		Delimiter:   []string{"_"},
		Header:      []string{"R1", "R2"},
		ColumnRules: ColumnRules{MatchParts: []int{3}},
	}
	semB := semA
	semB.Header = []string{"R2", "R1"} // different semantics, same Version "1"
	if mustFreeze(t, semA) == mustFreeze(t, semB) {
		t.Fatal("different semantics with equal Version collapsed to one revision")
	}

	sameSemHigherVersion := semA
	sameSemHigherVersion.Version = "999" // different Version, identical semantics
	if mustFreeze(t, semA) != mustFreeze(t, sameSemHigherVersion) {
		t.Fatal("identical semantics under a different Version changed the revision")
	}
}
