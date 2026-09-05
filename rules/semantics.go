package rules

// TDI-I4F: frozen classification semantics + revision identity.
//
// A legacy snapshot is accepted under some classification semantics. The accepted
// DB + compatibility projection must stay recoverable against the SAME semantics it
// was accepted under, even after the on-disk rule.json later changes. This file
// derives a deterministic, canonical representation of the classification-relevant
// behavior of a parsed+validated RuleSet, and a stable revision identity over it.
//
// Identity is over BEHAVIOR only. It deliberately excludes:
//   - RuleSet.Version (metadata, not behavior; a version bump alone is not a
//     semantic change, and two different semantic payloads must not collapse just
//     because Version matches — see I4F-T02/T03);
//   - source path / mtime;
//   - raw JSON whitespace / key order (identity is over the parsed struct, so
//     formatting differences that parse to the same semantics are the same
//     revision — see I4F-T01).
//
// I4F does NOT change legacy JSON parser acceptance to make it stricter; it only
// freezes the semantics of whatever the current parser already accepts.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
)

// RoleNormEntry is one canonical (observed-key -> role) mapping. RoleNormalization is
// the only map-valued semantic field, so it is emitted as key-sorted entries to make
// canonicalization deterministic regardless of Go map iteration order.
type RoleNormEntry struct {
	Key  string `json:"key"`
	Role string `json:"role"`
}

// ClassificationSemantics is the deterministic, canonical, classification-relevant
// projection of a RuleSet. Slice fields preserve their authored order (order is
// semantically meaningful for delimiter/header/matchParts); the role map is sorted.
// Every slice is normalized to non-nil so a nil-vs-empty difference cannot change
// identity.
type ClassificationSemantics struct {
	Delimiter         []string        `json:"delimiter"`
	Header            []string        `json:"header"`
	RowMatchParts     []int           `json:"rowMatchParts"`
	ColumnMatchParts  []int           `json:"columnMatchParts"`
	SizeMin           int             `json:"sizeMin"`
	SizeMax           int             `json:"sizeMax"`
	RoleNormalization []RoleNormEntry `json:"roleNormalization"`
}

// CanonicalizeSemantics extracts rs's classification-relevant semantics into a
// deterministic form. It does not validate rs (callers freeze only validated rules).
func CanonicalizeSemantics(rs RuleSet) ClassificationSemantics {
	sem := ClassificationSemantics{
		Delimiter:        canonStrings(rs.Delimiter),
		Header:           canonStrings(rs.Header),
		RowMatchParts:    canonInts(rs.RowRules.MatchParts),
		ColumnMatchParts: canonInts(rs.ColumnRules.MatchParts),
		SizeMin:          rs.SizeRules.MinSize,
		SizeMax:          rs.SizeRules.MaxSize,
	}
	entries := make([]RoleNormEntry, 0, len(rs.RoleNormalization))
	for k, v := range rs.RoleNormalization {
		entries = append(entries, RoleNormEntry{Key: k, Role: v})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Key < entries[j].Key })
	sem.RoleNormalization = entries
	return sem
}

// CanonicalJSON returns the deterministic canonical JSON encoding of the semantics.
// encoding/json emits struct fields in declaration order and slices in order, and the
// role entries are pre-sorted, so the byte output is stable for equal semantics.
func (c ClassificationSemantics) CanonicalJSON() ([]byte, error) {
	return json.Marshal(c)
}

// RevisionID is the classification-semantics revision identity: the sha256 (hex) of
// the canonical JSON. Internal identity only — not a public wire/id commitment.
func (c ClassificationSemantics) RevisionID() (string, error) {
	b, err := c.CanonicalJSON()
	if err != nil {
		return "", fmt.Errorf("failed to canonicalize classification semantics: %w", err)
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// FreezeRuleSet validates rs, then returns its canonical JSON and revision id. A rule
// set that fails IsValidRuleSet is not freezable: the caller must HOLD rather than pin
// an unusable basis.
func FreezeRuleSet(rs RuleSet) (canonicalJSON, revisionID string, err error) {
	if !IsValidRuleSet(rs) {
		return "", "", fmt.Errorf("rule set is invalid and cannot be frozen")
	}
	sem := CanonicalizeSemantics(rs)
	b, err := sem.CanonicalJSON()
	if err != nil {
		return "", "", fmt.Errorf("failed to canonicalize classification semantics: %w", err)
	}
	sum := sha256.Sum256(b)
	return string(b), hex.EncodeToString(sum[:]), nil
}

// RuleSetFromCanonical rebuilds a projection-equivalent RuleSet from canonical JSON
// produced by FreezeRuleSet. Version is intentionally empty: it is not part of the
// frozen semantics and does not affect projection. This is how the acceptance/reconcile
// path obtains a rule basis WITHOUT re-reading the mutable on-disk rule.json.
func RuleSetFromCanonical(canonicalJSON string) (RuleSet, error) {
	var sem ClassificationSemantics
	if err := json.Unmarshal([]byte(canonicalJSON), &sem); err != nil {
		return RuleSet{}, fmt.Errorf("failed to decode frozen classification semantics: %w", err)
	}
	rs := RuleSet{
		Delimiter:   sem.Delimiter,
		Header:      sem.Header,
		RowRules:    RowRules{MatchParts: sem.RowMatchParts},
		ColumnRules: ColumnRules{MatchParts: sem.ColumnMatchParts},
		SizeRules:   SizeRules{MinSize: sem.SizeMin, MaxSize: sem.SizeMax},
	}
	if len(sem.RoleNormalization) > 0 {
		rs.RoleNormalization = make(map[string]string, len(sem.RoleNormalization))
		for _, e := range sem.RoleNormalization {
			rs.RoleNormalization[e.Key] = e.Role
		}
	}
	return rs, nil
}

func canonStrings(in []string) []string {
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func canonInts(in []int) []int {
	out := make([]int, len(in))
	copy(out, in)
	return out
}
