package rules

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/HeaInSeo/tori/internal/utils"
	globallog "github.com/HeaInSeo/tori/log"
)

var logger = globallog.Log

// --- RuleSet 및 관련 타입 정의 -----------------------------------------

type RuleSet struct {
	Version           string            `json:"version"`
	Delimiter         []string          `json:"delimiter"`
	Header            []string          `json:"header"`
	RowRules          RowRules          `json:"rowRules"`
	ColumnRules       ColumnRules       `json:"columnRules"`
	SizeRules         SizeRules         `json:"sizeRules"`
	RoleNormalization map[string]string `json:"roleNormalization,omitempty"`
}

type RowRules struct {
	MatchParts []int `json:"matchParts"`
}

type ColumnRules struct {
	MatchParts []int `json:"matchParts"`
}

type SizeRules struct {
	MinSize int `json:"minSize"`
	MaxSize int `json:"maxSize"`
}

type DuplicateReportEntry struct {
	// ReasonCode is the v0.1 minimal classifier for duplicate detection.
	ReasonCode string
	// RowKey is the current grouping key derived from rowRules.matchParts.
	RowKey string
	// RoleKey is the current column key derived from columnRules.matchParts.
	RoleKey string
	// Candidates is the deterministic duplicate candidate set for the row/role collision.
	Candidates []string
	// SourceFileNames preserves the source filename context; in v0.1 it may match Candidates.
	SourceFileNames []string
	// Diagnostic is optional in v0.1 and may be empty.
	Diagnostic string
}

type DuplicateCollisionError struct {
	Entries []DuplicateReportEntry
}

func (e *DuplicateCollisionError) Error() string {
	return fmt.Sprintf("duplicate collision detected: %d entries", len(e.Entries))
}

type subjectCoordinate struct {
	components []string
}

type structuredGroup struct {
	coordinate      subjectCoordinate
	observedMembers map[string]string
	outcome         subjectOutcome
	legacyRowNumber int
}

type structuredGroupingResult struct {
	groups []structuredGroup
}

type subjectOutcome struct {
	observedMembers           map[string]string
	missingRequiredRoles      []string
	extraObservedMembers      map[string]string
	unresolvedObservedMembers map[string]string
	facts                     []subjectOutcomeFact
}

type subjectOutcomeFact struct {
	reasonCode     string
	role           string
	observedKey    string
	normalizedRole string
	fileName       string
}

type RoleNormalizationPreviewEntry struct {
	ObservedKey    string
	NormalizedRole string
	Resolved       bool
	FileName       string
}

type RowPreview struct {
	RowIndex          int
	ObservedRoles     []string
	RoleNormalization []RoleNormalizationPreviewEntry
}

type ResolverPreview struct {
	SourceFileCount     int
	RowCount            int
	ObservedRoleCount   int
	UnresolvedRoleCount int
	Rows                []RowPreview
}

type SchemaValidationPreviewEntry struct {
	ReasonCode     string
	RowIndex       int
	Role           string
	ObservedKey    string
	NormalizedRole string
	FileName       string
}

type SchemaValidationPreview struct {
	EntryCount                  int
	MissingRequiredRoleCount    int
	UnresolvedObservedRoleCount int
	ExtraObservedRoleCount      int
	Entries                     []SchemaValidationPreviewEntry
}

func NormalizeRoleKey(observedKey string, ruleSet RuleSet) (normalizedRole string, found bool) {
	if ruleSet.RoleNormalization == nil {
		return "", false
	}
	normalizedRole, found = ruleSet.RoleNormalization[observedKey]
	return normalizedRole, found
}

func BuildRoleNormalizationPreview(rowMap map[string]string, ruleSet RuleSet) []RoleNormalizationPreviewEntry {
	keys := make([]string, 0, len(rowMap))
	for key := range rowMap {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	preview := make([]RoleNormalizationPreviewEntry, 0, len(keys))
	for _, key := range keys {
		normalized, found := NormalizeRoleKey(key, ruleSet)
		preview = append(preview, RoleNormalizationPreviewEntry{
			ObservedKey:    key,
			NormalizedRole: normalized,
			Resolved:       found,
			FileName:       rowMap[key],
		})
	}
	return preview
}

func BuildRowPreview(rowIndex int, rowMap map[string]string, ruleSet RuleSet) RowPreview {
	observedRoles := make([]string, 0, len(rowMap))
	for key := range rowMap {
		observedRoles = append(observedRoles, key)
	}
	sort.Strings(observedRoles)

	return RowPreview{
		RowIndex:          rowIndex,
		ObservedRoles:     observedRoles,
		RoleNormalization: BuildRoleNormalizationPreview(rowMap, ruleSet),
	}
}

func BuildResolverPreview(resultMap map[int]map[string]string, ruleSet RuleSet) ResolverPreview {
	return buildResolverPreview(resultMap, ruleSet, countGroupedSourceFiles(resultMap))
}

func buildResolverPreview(resultMap map[int]map[string]string, ruleSet RuleSet, sourceFileCount int) ResolverPreview {
	rowIndexes := make([]int, 0, len(resultMap))
	for rowIndex := range resultMap {
		rowIndexes = append(rowIndexes, rowIndex)
	}
	sort.Ints(rowIndexes)

	rows := make([]RowPreview, 0, len(rowIndexes))
	observedRoleCount := 0
	unresolvedRoleCount := 0
	for _, rowIndex := range rowIndexes {
		rowPreview := BuildRowPreview(rowIndex, resultMap[rowIndex], ruleSet)
		rows = append(rows, rowPreview)
		observedRoleCount += len(rowPreview.ObservedRoles)
		for _, entry := range rowPreview.RoleNormalization {
			if !entry.Resolved {
				unresolvedRoleCount++
			}
		}
	}
	return ResolverPreview{
		SourceFileCount:     sourceFileCount,
		RowCount:            len(rows),
		ObservedRoleCount:   observedRoleCount,
		UnresolvedRoleCount: unresolvedRoleCount,
		Rows:                rows,
	}
}

func countGroupedSourceFiles(resultMap map[int]map[string]string) int {
	count := 0
	for _, row := range resultMap {
		count += len(row)
	}
	return count
}

func GenerateResolverPreview(fileNames []string, ruleSet RuleSet) (ResolverPreview, error) {
	resultMap, err := GroupFiles(fileNames, ruleSet)
	if err != nil {
		return ResolverPreview{}, err
	}
	return buildResolverPreview(resultMap, ruleSet, len(fileNames)), nil
}

func GenerateResolverPreviewFromDir(dirPath string) (ResolverPreview, error) {
	ruleSet, err := LoadRuleSetFromFile(dirPath)
	if err != nil {
		return ResolverPreview{}, fmt.Errorf("failed to load rule set: %w", err)
	}
	if !IsValidRuleSet(ruleSet) {
		return ResolverPreview{}, fmt.Errorf("rule set has conflicts or unused parts")
	}

	exclusions := []string{"rule.json", "invalid_files", "invalid_files_*", "fileblock.csv", "*.pb"}
	fileNames, err := ListFilesExclude(dirPath, exclusions)
	if err != nil {
		return ResolverPreview{}, fmt.Errorf("failed to list preview files: %w", err)
	}
	return GenerateResolverPreview(fileNames, ruleSet)
}

func BuildSchemaValidationPreview(resolverPreview ResolverPreview, ruleSet RuleSet) SchemaValidationPreview {
	entries := make([]SchemaValidationPreviewEntry, 0)
	missingRequiredRoleCount := 0
	unresolvedObservedRoleCount := 0
	extraObservedRoleCount := 0
	headerSet := make(map[string]struct{}, len(ruleSet.Header))
	for _, header := range ruleSet.Header {
		headerSet[header] = struct{}{}
	}

	for _, row := range resolverPreview.Rows {
		observedSet := make(map[string]RoleNormalizationPreviewEntry, len(row.RoleNormalization))
		for _, entry := range row.RoleNormalization {
			observedSet[entry.ObservedKey] = entry
			if !entry.Resolved {
				unresolvedObservedRoleCount++
				entries = append(entries, SchemaValidationPreviewEntry{
					ReasonCode:  "unresolved_observed_role",
					RowIndex:    row.RowIndex,
					ObservedKey: entry.ObservedKey,
					FileName:    entry.FileName,
				})
			}
		}

		for _, header := range ruleSet.Header {
			entry, ok := observedSet[header]
			if !ok || entry.FileName == "" {
				missingRequiredRoleCount++
				entries = append(entries, SchemaValidationPreviewEntry{
					ReasonCode: "missing_required_role",
					RowIndex:   row.RowIndex,
					Role:       header,
				})
			}
		}

		for _, observedRole := range row.ObservedRoles {
			if _, ok := headerSet[observedRole]; ok {
				continue
			}
			entry := observedSet[observedRole]
			extraObservedRoleCount++
			entries = append(entries, SchemaValidationPreviewEntry{
				ReasonCode:     "extra_observed_role",
				RowIndex:       row.RowIndex,
				ObservedKey:    observedRole,
				NormalizedRole: entry.NormalizedRole,
				FileName:       entry.FileName,
			})
		}
	}

	return SchemaValidationPreview{
		EntryCount:                  len(entries),
		MissingRequiredRoleCount:    missingRequiredRoleCount,
		UnresolvedObservedRoleCount: unresolvedObservedRoleCount,
		ExtraObservedRoleCount:      extraObservedRoleCount,
		Entries:                     entries,
	}
}

func BuildTypedRoleValidationPreview(resolverPreview ResolverPreview, requiredRoles []string) SchemaValidationPreview {
	entries := make([]SchemaValidationPreviewEntry, 0)
	missingRequiredRoleCount := 0
	unresolvedObservedRoleCount := 0
	extraObservedRoleCount := 0
	requiredRoleSet := make(map[string]struct{}, len(requiredRoles))
	for _, role := range requiredRoles {
		requiredRoleSet[role] = struct{}{}
	}

	for _, row := range resolverPreview.Rows {
		typedRoleSet := make(map[string]RoleNormalizationPreviewEntry, len(row.RoleNormalization))
		for _, entry := range row.RoleNormalization {
			if !entry.Resolved {
				unresolvedObservedRoleCount++
				entries = append(entries, SchemaValidationPreviewEntry{
					ReasonCode:  "unresolved_observed_role",
					RowIndex:    row.RowIndex,
					ObservedKey: entry.ObservedKey,
					FileName:    entry.FileName,
				})
				continue
			}

			typedRoleSet[entry.NormalizedRole] = entry
			if _, ok := requiredRoleSet[entry.NormalizedRole]; ok {
				continue
			}
			extraObservedRoleCount++
			entries = append(entries, SchemaValidationPreviewEntry{
				ReasonCode:     "extra_observed_role",
				RowIndex:       row.RowIndex,
				Role:           entry.NormalizedRole,
				ObservedKey:    entry.ObservedKey,
				NormalizedRole: entry.NormalizedRole,
				FileName:       entry.FileName,
			})
		}

		for _, role := range requiredRoles {
			entry, ok := typedRoleSet[role]
			if !ok || entry.FileName == "" {
				missingRequiredRoleCount++
				entries = append(entries, SchemaValidationPreviewEntry{
					ReasonCode: "missing_required_role",
					RowIndex:   row.RowIndex,
					Role:       role,
				})
			}
		}
	}

	return SchemaValidationPreview{
		EntryCount:                  len(entries),
		MissingRequiredRoleCount:    missingRequiredRoleCount,
		UnresolvedObservedRoleCount: unresolvedObservedRoleCount,
		ExtraObservedRoleCount:      extraObservedRoleCount,
		Entries:                     entries,
	}
}

func BuildPrimaryIndexPairingPreview(resolverPreview ResolverPreview, primaryRole string, indexRole string) SchemaValidationPreview {
	entries := make([]SchemaValidationPreviewEntry, 0)

	for _, row := range resolverPreview.Rows {
		hasPrimary := false
		var indexEntry RoleNormalizationPreviewEntry
		hasIndex := false

		for _, entry := range row.RoleNormalization {
			if !entry.Resolved {
				continue
			}
			if entry.NormalizedRole == primaryRole {
				hasPrimary = true
			}
			if entry.NormalizedRole == indexRole {
				hasIndex = true
				indexEntry = entry
			}
		}

		if hasIndex && !hasPrimary {
			entries = append(entries, SchemaValidationPreviewEntry{
				ReasonCode:     "unpaired_index_role",
				RowIndex:       row.RowIndex,
				Role:           indexRole,
				ObservedKey:    indexEntry.ObservedKey,
				NormalizedRole: indexEntry.NormalizedRole,
				FileName:       indexEntry.FileName,
			})
		}
	}

	return SchemaValidationPreview{
		EntryCount: len(entries),
		Entries:    entries,
	}
}

// ----------------------------------------------------------------------

// LoadRuleSetFromFile JSON 파일에서 RuleSet 을 읽어옴
func LoadRuleSetFromFile(dirPath string) (RuleSet, error) {
	// utils.CheckPath 으로 경로 유효성 검사
	path, err := utils.CheckPath(dirPath)
	if err != nil {
		return RuleSet{}, err
	}

	info, err := os.Stat(path)
	if err != nil {
		return RuleSet{}, fmt.Errorf("failed to access path: %w", err)
	}
	if !info.IsDir() {
		return RuleSet{}, fmt.Errorf("path is not a directory: %s", path)
	}

	jsonFile := filepath.Join(path, "rule.json")
	exists, _, err := utils.FileExists(jsonFile)
	if err != nil {
		return RuleSet{}, fmt.Errorf("failed to check rule.json: %w", err)
	}
	if !exists {
		return RuleSet{}, fmt.Errorf("rule.json not found in: %s", path)
	}

	data, err := os.ReadFile(jsonFile) //nolint:gosec // jsonFile is derived from an operator-supplied directory path, not external input
	if err != nil {
		return RuleSet{}, fmt.Errorf("failed to read rule.json: %w", err)
	}

	var ruleSet RuleSet
	if err := json.Unmarshal(data, &ruleSet); err != nil {
		return RuleSet{}, fmt.Errorf("failed to unmarshal rule.json: %w", err)
	}
	return ruleSet, nil
}

// splitFileName 파일명(fileName)을 주어진 구분자(delimiters)로 치환한 뒤 공백으로 분리
func splitFileName(fileName string, delimiters []string) []string {
	for _, delim := range delimiters {
		fileName = strings.ReplaceAll(fileName, delim, " ")
	}
	return strings.Fields(fileName)
}

// FilesToMap 파일명 리스트 → (RowIdx → (ColumnKey → 파일명)) 구조 생성

func (c subjectCoordinate) stableKey() string {
	parts := make([]string, 0, len(c.components))
	for _, component := range c.components {
		parts = append(parts, fmt.Sprintf("%d:%s", len(component), component))
	}
	return strings.Join(parts, "|")
}

func deriveSubjectCoordinate(parts []string, matchParts []int) subjectCoordinate {
	components := make([]string, 0, len(matchParts))
	for _, idx := range matchParts {
		if idx >= 0 && idx < len(parts) {
			components = append(components, parts[idx])
		}
	}
	return subjectCoordinate{components: components}
}

func deriveObservedRoleKey(parts []string, matchParts []int) string {
	components := make([]string, 0, len(matchParts))
	for _, idx := range matchParts {
		if idx >= 0 && idx < len(parts) {
			components = append(components, parts[idx])
		}
	}
	return strings.Join(components, "_")
}

func buildSubjectOutcome(observedMembers map[string]string, ruleSet RuleSet) subjectOutcome {
	observed := make(map[string]string, len(observedMembers))
	extra := make(map[string]string)
	unresolved := make(map[string]string)
	facts := make([]subjectOutcomeFact, 0, len(observedMembers)*3+len(ruleSet.Header))

	headerSet := make(map[string]struct{}, len(ruleSet.Header))
	for _, header := range ruleSet.Header {
		headerSet[header] = struct{}{}
	}

	observedKeys := make([]string, 0, len(observedMembers))
	for observedKey := range observedMembers {
		observedKeys = append(observedKeys, observedKey)
	}
	sort.Strings(observedKeys)

	for _, observedKey := range observedKeys {
		fileName := observedMembers[observedKey]
		observed[observedKey] = fileName

		normalizedRole, resolved := NormalizeRoleKey(observedKey, ruleSet)
		facts = append(facts, subjectOutcomeFact{
			reasonCode:     "observed_member",
			observedKey:    observedKey,
			normalizedRole: normalizedRole,
			fileName:       fileName,
		})

		if !resolved {
			unresolved[observedKey] = fileName
			facts = append(facts, subjectOutcomeFact{
				reasonCode:  "unresolved_observed_role",
				observedKey: observedKey,
				fileName:    fileName,
			})
		}

		if _, ok := headerSet[observedKey]; !ok {
			extra[observedKey] = fileName
			facts = append(facts, subjectOutcomeFact{
				reasonCode:     "extra_observed_role",
				role:           normalizedRole,
				observedKey:    observedKey,
				normalizedRole: normalizedRole,
				fileName:       fileName,
			})
		}
	}

	missing := make([]string, 0)
	for _, header := range ruleSet.Header {
		fileName, ok := observedMembers[header]
		if ok && fileName != "" {
			continue
		}
		missing = append(missing, header)
		facts = append(facts, subjectOutcomeFact{
			reasonCode: "missing_required_role",
			role:       header,
		})
	}

	return subjectOutcome{
		observedMembers:           observed,
		missingRequiredRoles:      missing,
		extraObservedMembers:      extra,
		unresolvedObservedMembers: unresolved,
		facts:                     facts,
	}
}

// groupFilesStructured groups files by an internal stable subject coordinate.
// The coordinate keeps structured row components and uses a length-prefixed
// internal key so components that collide under "_" joining remain distinct.
func groupFilesStructured(fileNames []string, ruleSet RuleSet) (structuredGroupingResult, error) {
	rowMap := make(map[string]int) // stable coordinate key → encounter rowIndex
	nextRowIdx := 0
	result := make(map[int]structuredGroup)
	type duplicateKey struct {
		rowKey  string
		roleKey string
	}
	duplicateMap := make(map[duplicateKey]*DuplicateReportEntry)
	duplicateOrder := make([]duplicateKey, 0)

	for _, fn := range fileNames {
		parts := splitFileName(fn, ruleSet.Delimiter)

		// 1) Stable subject coordinate 생성
		coordinate := deriveSubjectCoordinate(parts, ruleSet.RowRules.MatchParts)
		stableKey := coordinate.stableKey()
		rowKey := strings.Join(coordinate.components, "_")

		if _, found := rowMap[stableKey]; !found {
			rowMap[stableKey] = nextRowIdx
			result[nextRowIdx] = structuredGroup{
				coordinate:      coordinate,
				observedMembers: make(map[string]string),
				legacyRowNumber: nextRowIdx,
			}
			nextRowIdx++
		}
		rowIdx := rowMap[stableKey]
		group := result[rowIdx]

		// 2) Column 키 생성
		colKey := deriveObservedRoleKey(parts, ruleSet.ColumnRules.MatchParts)

		// 3) 결과에 추가
		if existing, exists := group.observedMembers[colKey]; exists && existing != fn {
			key := duplicateKey{rowKey: stableKey, roleKey: colKey}
			entry, found := duplicateMap[key]
			if !found {
				entry = &DuplicateReportEntry{
					ReasonCode: "duplicate_role_in_row",
					RowKey:     rowKey,
					RoleKey:    colKey,
				}
				duplicateMap[key] = entry
				duplicateOrder = append(duplicateOrder, key)
			}
			entry.Candidates = appendUniqueString(entry.Candidates, existing)
			entry.Candidates = appendUniqueString(entry.Candidates, fn)
			entry.SourceFileNames = appendUniqueString(entry.SourceFileNames, existing)
			entry.SourceFileNames = appendUniqueString(entry.SourceFileNames, fn)
			continue
		}
		group.observedMembers[colKey] = fn
		result[rowIdx] = group
	}

	if len(duplicateOrder) > 0 {
		entries := make([]DuplicateReportEntry, 0, len(duplicateOrder))
		for _, key := range duplicateOrder {
			entry := duplicateMap[key]
			sort.Strings(entry.Candidates)
			sort.Strings(entry.SourceFileNames)
			entries = append(entries, *entry)
		}
		return structuredGroupingResult{}, &DuplicateCollisionError{Entries: entries}
	}

	for rowIdx, group := range result {
		group.outcome = buildSubjectOutcome(group.observedMembers, ruleSet)
		result[rowIdx] = group
	}

	groups := make([]structuredGroup, 0, len(result))
	for _, group := range result {
		groups = append(groups, group)
	}
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].coordinate.stableKey() < groups[j].coordinate.stableKey()
	})

	return structuredGroupingResult{groups: groups}, nil
}

func legacyGroupsFromStructured(grouping structuredGroupingResult) map[int]map[string]string {
	result := make(map[int]map[string]string, len(grouping.groups))
	for _, group := range grouping.groups {
		members := make(map[string]string, len(group.observedMembers))
		for role, fileName := range group.observedMembers {
			members[role] = fileName
		}
		result[group.legacyRowNumber] = members
	}
	return result
}

// --- TDI-I12 subject-scoped conflict isolation ------------------------------
//
// I12 supersedes the whole-group fail-fast posture: a duplicate role-in-row is a
// STRUCTURAL CONFLICT scoped to ONE stable subject coordinate. A conflict in one
// subject must not abort healthy sibling subjects in the same batch; the collided
// subject must stay visible as conflict evidence (never downgraded to a healthy
// grouped member and never resolved by an arbitrary winning candidate).
// See docs/duplicate_policy_contract_v0.2.md.

// DuplicateRoleEvidence is the deterministic, source-only evidence for one role
// collision inside a single subject coordinate. Candidates and SourceFileNames are
// sorted so the evidence is canonical regardless of input order.
type DuplicateRoleEvidence struct {
	// ReasonCode is fixed at "duplicate_role_in_row" for a duplicate role collision.
	ReasonCode string
	// Role is the observed column/role key that collided.
	Role string
	// Candidates are the distinct colliding source file names, sorted.
	Candidates []string
	// SourceFileNames preserves the source filename context; it mirrors Candidates.
	SourceFileNames []string
}

// SubjectConflict is a structural conflict scoped to exactly one stable subject
// coordinate. It records every role collision observed for that subject. It carries
// only source/grouping facts — it does not decide runnability, authorization, or any
// Run/Auto-Run policy (that boundary belongs to downstream consumers, not Tori).
type SubjectConflict struct {
	// SubjectKey is the internal stable coordinate key (length-prefixed, collision-free).
	SubjectKey string
	// SubjectComponents are the coordinate components in authored matchParts order.
	SubjectComponents []string
	// RowKey is the human-readable "_"-joined coordinate (matches legacy row key text).
	RowKey string
	// Roles are the per-role collision evidences, sorted by Role.
	Roles []DuplicateRoleEvidence
	// SourceFileNames are ALL distinct source files that mapped to this subject, sorted,
	// so the conflicted subject never disappears from downstream evidence/reporting.
	SourceFileNames []string
}

// IsolatedGroupingResult is the canonical grouping outcome under I12 conflict
// isolation: Healthy carries the subjects with no role collision (legacy row-index →
// role → file), Conflicts carries the collided subjects as deterministic evidence.
// Healthy and Conflicts are disjoint by subject coordinate.
type IsolatedGroupingResult struct {
	Healthy   map[int]map[string]string
	Conflicts []SubjectConflict
}

type isolationSubjectAccum struct {
	coordinate   subjectCoordinate
	rowKey       string
	encounterIdx int
	firstSeen    map[string]string   // role -> first-seen file (provisional healthy member)
	roleFiles    map[string][]string // role -> distinct files, encounter order
	allFiles     []string            // distinct files for the subject, encounter order
}

// GroupFilesIsolated is the CANONICAL grouping path for publication/projection. It
// never aborts the batch on a duplicate: a subject with any duplicate role collision
// is returned as a SubjectConflict (visible evidence, no arbitrary winner) while every
// healthy sibling subject is grouped normally. Output ordering is deterministic:
// conflicts are sorted by stable subject key, role evidence by role, and all file
// lists are sorted.
func GroupFilesIsolated(fileNames []string, ruleSet RuleSet) IsolatedGroupingResult {
	subjects := make(map[string]*isolationSubjectAccum)
	order := make([]string, 0, len(fileNames))

	for _, fn := range fileNames {
		parts := splitFileName(fn, ruleSet.Delimiter)
		coordinate := deriveSubjectCoordinate(parts, ruleSet.RowRules.MatchParts)
		stableKey := coordinate.stableKey()
		colKey := deriveObservedRoleKey(parts, ruleSet.ColumnRules.MatchParts)

		accum, ok := subjects[stableKey]
		if !ok {
			accum = &isolationSubjectAccum{
				coordinate:   coordinate,
				rowKey:       strings.Join(coordinate.components, "_"),
				encounterIdx: len(order),
				firstSeen:    make(map[string]string),
				roleFiles:    make(map[string][]string),
			}
			subjects[stableKey] = accum
			order = append(order, stableKey)
		}

		accum.allFiles = appendUniqueString(accum.allFiles, fn)
		accum.roleFiles[colKey] = appendUniqueString(accum.roleFiles[colKey], fn)
		if _, seen := accum.firstSeen[colKey]; !seen {
			accum.firstSeen[colKey] = fn
		}
	}

	result := IsolatedGroupingResult{
		Healthy:   make(map[int]map[string]string),
		Conflicts: make([]SubjectConflict, 0),
	}

	for _, stableKey := range order {
		accum := subjects[stableKey]

		conflictedRoles := make([]string, 0)
		for role, files := range accum.roleFiles {
			if len(files) > 1 {
				conflictedRoles = append(conflictedRoles, role)
			}
		}

		if len(conflictedRoles) == 0 {
			members := make(map[string]string, len(accum.firstSeen))
			for role, file := range accum.firstSeen {
				members[role] = file
			}
			result.Healthy[accum.encounterIdx] = members
			continue
		}

		sort.Strings(conflictedRoles)
		evidences := make([]DuplicateRoleEvidence, 0, len(conflictedRoles))
		for _, role := range conflictedRoles {
			candidates := append([]string(nil), accum.roleFiles[role]...)
			sort.Strings(candidates)
			source := append([]string(nil), candidates...)
			evidences = append(evidences, DuplicateRoleEvidence{
				ReasonCode:      "duplicate_role_in_row",
				Role:            role,
				Candidates:      candidates,
				SourceFileNames: source,
			})
		}

		subjectFiles := append([]string(nil), accum.allFiles...)
		sort.Strings(subjectFiles)
		result.Conflicts = append(result.Conflicts, SubjectConflict{
			SubjectKey:        stableKey,
			SubjectComponents: append([]string(nil), accum.coordinate.components...),
			RowKey:            accum.rowKey,
			Roles:             evidences,
			SourceFileNames:   subjectFiles,
		})
	}

	sort.Slice(result.Conflicts, func(i, j int) bool {
		return result.Conflicts[i].SubjectKey < result.Conflicts[j].SubjectKey
	})

	return result
}

// InvalidRowsFromConflicts adapts conflicted subjects into the invalid-row shape used
// by SaveInvalidFiles, so a conflicted subject's source files remain visible on disk
// (in the invalid_files report) instead of silently disappearing. It never emits a
// healthy grouped member for a conflicted subject.
func InvalidRowsFromConflicts(conflicts []SubjectConflict) []map[string]string {
	rows := make([]map[string]string, 0, len(conflicts))
	for _, conflict := range conflicts {
		row := make(map[string]string, len(conflict.SourceFileNames))
		for _, fileName := range conflict.SourceFileNames {
			row[fileName] = fileName
		}
		rows = append(rows, row)
	}
	return rows
}

// GroupFiles groups files by row/column keys and, on any duplicate role-in-row
// collision, fails the WHOLE batch with a DuplicateCollisionError.
//
// LEGACY / NON-CANONICAL (TDI-I12). This whole-group fail-fast posture is the
// superseded v0.1 duplicate policy (see docs/duplicate_policy_contract_v0.1.md).
// It is NOT the canonical publication authority any more: the FileBlock publication
// path (block.GenerateFileBlock*, block.ProjectFileBlock) now groups through
// GroupFilesIsolated, which isolates a duplicate to its single subject coordinate
// and never aborts healthy siblings (docs/duplicate_policy_contract_v0.2.md).
//
// GroupFiles is retained only as a coarse diagnostic that loudly refuses a batch
// containing any collision: it never silently picks an arbitrary winner and never
// hides a collision. It is safe precisely because no publication path depends on it.
// Do not route new publication/projection work through GroupFiles.
func GroupFiles(fileNames []string, ruleSet RuleSet) (map[int]map[string]string, error) {
	rowMap := make(map[string]int) // rowKey → rowIndex
	nextRowIdx := 0
	result := make(map[int]map[string]string) // 최종 결과
	type duplicateKey struct {
		rowKey  string
		roleKey string
	}
	duplicateMap := make(map[duplicateKey]*DuplicateReportEntry)
	duplicateOrder := make([]duplicateKey, 0)

	for _, fn := range fileNames {
		parts := splitFileName(fn, ruleSet.Delimiter)

		// 1) Row 키 생성
		coordinate := deriveSubjectCoordinate(parts, ruleSet.RowRules.MatchParts)
		rowKey := strings.Join(coordinate.components, "_")

		if _, found := rowMap[rowKey]; !found {
			rowMap[rowKey] = nextRowIdx
			result[nextRowIdx] = make(map[string]string)
			nextRowIdx++
		}
		rowIdx := rowMap[rowKey]

		// 2) Column 키 생성
		colKey := deriveObservedRoleKey(parts, ruleSet.ColumnRules.MatchParts)

		// 3) 결과에 추가
		if existing, exists := result[rowIdx][colKey]; exists && existing != fn {
			key := duplicateKey{rowKey: rowKey, roleKey: colKey}
			entry, found := duplicateMap[key]
			if !found {
				entry = &DuplicateReportEntry{
					ReasonCode: "duplicate_role_in_row",
					RowKey:     rowKey,
					RoleKey:    colKey,
				}
				duplicateMap[key] = entry
				duplicateOrder = append(duplicateOrder, key)
			}
			entry.Candidates = appendUniqueString(entry.Candidates, existing)
			entry.Candidates = appendUniqueString(entry.Candidates, fn)
			entry.SourceFileNames = appendUniqueString(entry.SourceFileNames, existing)
			entry.SourceFileNames = appendUniqueString(entry.SourceFileNames, fn)
			continue
		}
		result[rowIdx][colKey] = fn
	}

	if len(duplicateOrder) > 0 {
		entries := make([]DuplicateReportEntry, 0, len(duplicateOrder))
		for _, key := range duplicateOrder {
			entry := duplicateMap[key]
			sort.Strings(entry.Candidates)
			sort.Strings(entry.SourceFileNames)
			entries = append(entries, *entry)
		}
		return nil, &DuplicateCollisionError{Entries: entries}
	}

	return result, nil
}

func appendUniqueString(items []string, value string) []string {
	for _, existing := range items {
		if existing == value {
			return items
		}
	}
	return append(items, value)
}

// FilterMap 각 Row에 컬럼 수가 expectedColCount와 같은 행만 valid, 나머지는 invalid로 분리
// // expectedColCount 와 일치하는 그룹은 valid에, 그렇지 않으면 invalid에 담아 반환

// FilterGroups GroupFiles 로 묶인 결과에서 expectedColCount 와 일치하는 그룹은 valid 에, 그렇지 않으면 invalid 에 담아 반환.
// 입력 맵의 키를 오름차순으로 정렬하여 처리 순서를 결정적으로 유지함.
func FilterGroups(resultMap map[int]map[string]string, expectedColCount int) (map[int]map[string]string, []map[string]string) {
	valid := make(map[int]map[string]string)
	invalid := make([]map[string]string, 0)
	nextRowIdx := 0

	keys := make([]int, 0, len(resultMap))
	for k := range resultMap {
		keys = append(keys, k)
	}
	sort.Ints(keys)

	for _, k := range keys {
		row := resultMap[k]
		if len(row) == expectedColCount {
			valid[nextRowIdx] = row
			nextRowIdx++
		} else {
			invalid = append(invalid, row)
		}
	}
	return valid, invalid
}

// FilterGroupsByHeaders GroupFiles 로 묶인 결과에서 headers 의 키와 정확히 일치하는 그룹만 valid 로 분류.
// column 수뿐 아니라 missing/extra key 도 검사하므로 FilterGroups 보다 엄격함.
func FilterGroupsByHeaders(resultMap map[int]map[string]string, headers []string) (map[int]map[string]string, []map[string]string) {
	valid := make(map[int]map[string]string)
	invalid := make([]map[string]string, 0)
	nextRowIdx := 0

	keys := make([]int, 0, len(resultMap))
	for k := range resultMap {
		keys = append(keys, k)
	}
	sort.Ints(keys)

	for _, k := range keys {
		row := resultMap[k]
		missing, extra := collectMissingAndExtraKeys(headers, row)
		if len(missing) == 0 && len(extra) == 0 {
			valid[nextRowIdx] = row
			nextRowIdx++
		} else {
			invalid = append(invalid, row)
		}
	}
	return valid, invalid
}

// WriteInvalidFiles invalid 행의 모든 파일명을 <outputDir>/invalid_files_YYYYMMDDhhmmss.txt 로 기록

// SaveInvalidFiles invalid 행의 모든 파일명을 <outputDir>/invalid_files_YYYYMMDDhhmmss.txt 로 기록
func SaveInvalidFiles(invalidRows []map[string]string, outputDir string) (err error) {
	if len(invalidRows) == 0 {
		return nil
	}

	info, statErr := os.Stat(outputDir)
	if statErr != nil {
		return fmt.Errorf("failed to stat outputDir %s: %w", outputDir, statErr)
	}
	if !info.IsDir() {
		return fmt.Errorf("output path is not a directory: %s", outputDir)
	}

	ts := time.Now().Format("20060102150405")
	outFile := filepath.Join(outputDir, fmt.Sprintf("invalid_files_%s.txt", ts))
	f, createErr := os.Create(outFile) //nolint:gosec // outFile is derived from an operator-supplied output directory, not external input
	if createErr != nil {
		return fmt.Errorf("failed to create %s: %w", outFile, createErr)
	}
	defer func() {
		if errClose := f.Close(); errClose != nil && err == nil {
			err = fmt.Errorf("failed to close file: %w", errClose)
		}
	}()

	for _, row := range invalidRows {
		// Sort the file names within each row deterministically before writing.
		// The invalid-row model is a map, so ranging it directly would emit a
		// non-deterministic ordering; the same conflict input must always produce
		// identical invalid-report content ordering.
		fileNames := make([]string, 0, len(row))
		for _, fn := range row {
			fileNames = append(fileNames, fn)
		}
		sort.Strings(fileNames)
		for _, fn := range fileNames {
			if _, wErr := f.WriteString(fn + "\n"); wErr != nil {
				err = fmt.Errorf("failed to write to %s: %w", outFile, wErr)
				return err
			}
		}
	}
	return nil
}

// ValidateRuleSet 중복 인덱스 사용 여부 등을 점검

// IsValidRuleSet 중복 인덱스 사용 여부 등을 점검
func IsValidRuleSet(ruleSet RuleSet) bool {
	usage := make(map[int][]string)
	addUsage := func(indices []int, role string) {
		for _, idx := range indices {
			usage[idx] = append(usage[idx], role)
		}
	}
	addUsage(ruleSet.RowRules.MatchParts, "RowRules")
	addUsage(ruleSet.ColumnRules.MatchParts, "ColumnRules")

	hasConflict := false
	for idx, roles := range usage {
		if idx < 0 {
			logger.Infof("Negative index detected: %d", idx)
			hasConflict = true
			continue
		}
		if len(roles) > 1 {
			logger.Infof("Conflict detected: part %d used for %v", idx, roles)
			hasConflict = true
		}
	}
	return !hasConflict
}

// ReadAllFileNames 디렉토리에서 파일 목록을 읽되, exclusions 에 맞는 파일명은 제외

// ListFilesExclude 디렉토리에서 파일 목록을 읽되, exclusions 에 맞는 파일명은 제외
func ListFilesExclude(dirPath string, exclusions []string) ([]string, error) {
	path, err := utils.CheckPath(dirPath)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory %s: %w", path, err)
	}

	isExcluded := func(name string) bool {
		for _, ex := range exclusions {
			if strings.HasPrefix(ex, "*.") {
				ext := ex[1:] // "*.pb" -> ".pb"
				if strings.Contains(name, ext) {
					return true
				}
			} else if strings.HasSuffix(ex, "*") {
				prefix := ex[:len(ex)-1] // "invalid_files_*" -> "invalid_files_"
				if strings.HasPrefix(name, prefix) {
					return true
				}
			} else {
				if name == ex {
					return true
				}
			}
		}
		return false
	}

	var fileNames []string
	for _, entry := range entries {
		n := entry.Name()
		if isExcluded(n) {
			continue
		}
		fileNames = append(fileNames, n)
	}
	return fileNames, nil
}

// SaveResultMapToCSV validRows(map[int]map[string]string) + headers → CSV 파일로 저장

func collectMissingAndExtraKeys(headers []string, rowMap map[string]string) (missing []string, extra []string) {
	headerSet := make(map[string]struct{}, len(headers))
	for _, header := range headers {
		headerSet[header] = struct{}{}
	}

	for _, header := range headers {
		value, ok := rowMap[header]
		if !ok || value == "" {
			missing = append(missing, header)
		}
	}

	for key := range rowMap {
		if _, ok := headerSet[key]; !ok {
			extra = append(extra, key)
		}
	}

	sort.Strings(extra)
	return missing, extra
}

// ExportResultsCSV validRows(map[int]map[string]string) + headers → CSV 파일로 저장
func ExportResultsCSV(resultMap map[int]map[string]string, headers []string, outputDir string) (err error) {
	path, checkErr := utils.CheckPath(outputDir)
	if checkErr != nil {
		return checkErr
	}

	info, statErr := os.Stat(path)
	if statErr != nil {
		return fmt.Errorf("failed to stat %s: %w", path, statErr)
	}
	if !info.IsDir() {
		return fmt.Errorf("output path is not a directory: %s", path)
	}

	csvFile := filepath.Join(path, "fileblock.csv")
	f, createErr := os.Create(csvFile) //nolint:gosec // csvFile is derived from an operator-supplied output directory, not external input
	if createErr != nil {
		return fmt.Errorf("failed to create %s: %w", csvFile, createErr)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("failed to close file: %w", cerr)
		}
	}()

	writer := csv.NewWriter(f)
	defer writer.Flush()

	// 헤더 행 작성
	headerRow := append([]string{"Row"}, headers...)
	if wErr := writer.Write(headerRow); wErr != nil {
		return fmt.Errorf("failed to write header row: %w", wErr)
	}

	// 각 row 순서대로 CSV 에 작성
	for i := 0; i < len(resultMap); i++ {
		rowMap := resultMap[i]
		missing, _ := collectMissingAndExtraKeys(headers, rowMap)
		missingSet := make(map[string]struct{}, len(missing))
		for _, key := range missing {
			missingSet[key] = struct{}{}
		}
		record := make([]string, len(headers)+1)
		record[0] = fmt.Sprintf("Row%d", i)
		for j, colKey := range headers {
			if _, isMissing := missingSet[colKey]; isMissing {
				continue
			}
			if val, ok := rowMap[colKey]; ok {
				record[j+1] = val
			}
		}
		if wErr := writer.Write(record); wErr != nil {
			return fmt.Errorf("failed to write row %d: %w", i, wErr)
		}
	}
	return nil
}
