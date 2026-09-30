package rules

import "sort"

// TDI-I3M: exported, deterministic subject facts for publication.
//
// Publication needs the I11A stable subject coordinate and the normalized member/role
// facts of every healthy subject, keyed by coordinate rather than by the legacy row
// number (which depends on encounter order). The internal grouping types stay
// unexported; this file only exposes the facts a semantic manifest freezes.

// PublishedMember is one grouped member of a subject.
type PublishedMember struct {
	// ObservedKey is the observed column/role key derived from the file name.
	ObservedKey string `json:"observedKey"`
	// NormalizedRole is the role the RuleSet maps ObservedKey to, or "" when unresolved.
	NormalizedRole string `json:"normalizedRole"`
	// FileName is the member file name.
	FileName string `json:"fileName"`
}

// PublishedSubject is one healthy subject under its stable coordinate.
type PublishedSubject struct {
	// SubjectKey is the I11A length-prefixed stable coordinate key.
	SubjectKey string `json:"subjectKey"`
	// Components are the coordinate components in authored matchParts order.
	Components []string `json:"components"`
	// Members are sorted by ObservedKey.
	Members []PublishedMember `json:"members"`
}

// PublicationSubjects groups fileNames under ruleSet and returns the healthy subjects
// sorted by SubjectKey, plus the I12 conflicted subjects. A conflicted subject is never
// returned as a healthy subject and never gets an arbitrary winner. The result does not
// depend on the order of fileNames.
func PublicationSubjects(fileNames []string, ruleSet RuleSet) ([]PublishedSubject, []SubjectConflict) {
	isolated := GroupFilesIsolated(fileNames, ruleSet)
	conflicted := make(map[string]struct{}, len(isolated.Conflicts))
	for _, c := range isolated.Conflicts {
		conflicted[c.SubjectKey] = struct{}{}
	}

	type accum struct {
		coordinate subjectCoordinate
		members    map[string]string
	}
	subjects := make(map[string]*accum)
	for _, fn := range fileNames {
		parts := splitFileName(fn, ruleSet.Delimiter)
		coordinate := deriveSubjectCoordinate(parts, ruleSet.RowRules.MatchParts)
		key := coordinate.stableKey()
		if _, bad := conflicted[key]; bad {
			continue
		}
		a, ok := subjects[key]
		if !ok {
			a = &accum{coordinate: coordinate, members: make(map[string]string)}
			subjects[key] = a
		}
		a.members[deriveObservedRoleKey(parts, ruleSet.ColumnRules.MatchParts)] = fn
	}

	out := make([]PublishedSubject, 0, len(subjects))
	for key, a := range subjects {
		members := make([]PublishedMember, 0, len(a.members))
		for observed, file := range a.members {
			role, _ := NormalizeRoleKey(observed, ruleSet)
			members = append(members, PublishedMember{ObservedKey: observed, NormalizedRole: role, FileName: file})
		}
		sort.Slice(members, func(i, j int) bool { return members[i].ObservedKey < members[j].ObservedKey })
		out = append(out, PublishedSubject{
			SubjectKey: key,
			Components: append([]string(nil), a.coordinate.components...),
			Members:    members,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SubjectKey < out[j].SubjectKey })
	return out, isolated.Conflicts
}
