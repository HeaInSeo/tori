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

	globallog "github.com/seoyhaein/tori/log"
	"github.com/seoyhaein/utils"
)

var logger = globallog.Log

// --- RuleSet 및 관련 타입 정의 -----------------------------------------

type RuleSet struct {
	Version     string      `json:"version"`
	Delimiter   []string    `json:"delimiter"`
	Header      []string    `json:"header"`
	RowRules    RowRules    `json:"rowRules"`
	ColumnRules ColumnRules `json:"columnRules"`
	SizeRules   SizeRules   `json:"sizeRules"`
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

	data, err := os.ReadFile(jsonFile)
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

// GroupFiles 이름으로 바꿀 예정 파일 목록을 RuleSet 에 따라 행·열 구조로 묶어서 반환
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
		var rowKeyParts []string
		for _, idx := range ruleSet.RowRules.MatchParts {
			if idx < len(parts) {
				rowKeyParts = append(rowKeyParts, parts[idx])
			}
		}
		rowKey := strings.Join(rowKeyParts, "_")

		if _, found := rowMap[rowKey]; !found {
			rowMap[rowKey] = nextRowIdx
			result[nextRowIdx] = make(map[string]string)
			nextRowIdx++
		}
		rowIdx := rowMap[rowKey]

		// 2) Column 키 생성
		var colKeyParts []string
		for _, idx := range ruleSet.ColumnRules.MatchParts {
			if idx < len(parts) {
				colKeyParts = append(colKeyParts, parts[idx])
			}
		}
		colKey := strings.Join(colKeyParts, "_")

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

// FilterGroups GroupFiles 로 묶인 결과에서 expectedColCount 와 일치하는 그룹은 valid 에, 그렇지 않으면 invalid 에 담아 반환
func FilterGroups(resultMap map[int]map[string]string, expectedColCount int) (map[int]map[string]string, []map[string]string) {
	valid := make(map[int]map[string]string)
	invalid := make([]map[string]string, 0)
	nextRowIdx := 0

	for _, row := range resultMap {
		if len(row) == expectedColCount {
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
func SaveInvalidFiles(invalidRows []map[string]string, outputDir string) error {
	if len(invalidRows) == 0 {
		return nil
	}

	info, err := os.Stat(outputDir)
	if err != nil {
		return fmt.Errorf("failed to stat outputDir %s: %w", outputDir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("output path is not a directory: %s", outputDir)
	}

	ts := time.Now().Format("20060102150405")
	outFile := filepath.Join(outputDir, fmt.Sprintf("invalid_files_%s.txt", ts))
	f, err := os.Create(outFile)
	if err != nil {
		return fmt.Errorf("failed to create %s: %w", outFile, err)
	}
	defer func() {
		if errClose := f.Close(); errClose != nil && err == nil {
			err = fmt.Errorf("failed to close file: %w", errClose)
		}
	}()

	for _, row := range invalidRows {
		for _, fn := range row {
			if _, wErr := f.WriteString(fn + "\n"); wErr != nil && err == nil {
				err = fmt.Errorf("failed to write to %s: %w", outFile, wErr)
			}
		}
	}
	return err
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
		if _, ok := rowMap[header]; !ok {
			missing = append(missing, header)
		}
	}

	for key := range rowMap {
		if _, ok := headerSet[key]; !ok {
			extra = append(extra, key)
		}
	}

	sort.Strings(missing)
	sort.Strings(extra)
	return missing, extra
}

// ExportResultsCSV validRows(map[int]map[string]string) + headers → CSV 파일로 저장
func ExportResultsCSV(resultMap map[int]map[string]string, headers []string, outputDir string) error {
	path, err := utils.CheckPath(outputDir)
	if err != nil {
		return err
	}

	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("failed to stat %s: %w", path, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("output path is not a directory: %s", path)
	}

	csvFile := filepath.Join(path, "fileblock.csv")
	f, err := os.Create(csvFile)
	if err != nil {
		return fmt.Errorf("failed to create %s: %w", csvFile, err)
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
