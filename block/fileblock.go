package block

import (
	"fmt"
	"path/filepath"

	pb "github.com/seoyhaein/api-protos/gen/go/datablock/ichthys"
	"github.com/seoyhaein/tori/protoio"
	"github.com/seoyhaein/tori/rules"
)

// GenerateFileBlockFromDir 디렉터리 경로를 받아서 FileBlock 객체를 생성하고, 바이너리 protobuf 파일로 저장
func GenerateFileBlockFromDir(dirPath string) (*pb.FileBlock, error) {
	// 1. 룰 로딩
	ruleSet, err := rules.LoadRuleSetFromFile(dirPath)
	if err != nil {
		return nil, fmt.Errorf("LoadRuleSetFromFile error: %w", err)
	}

	// 2. 룰 검증
	if !rules.IsValidRuleSet(ruleSet) {
		return nil, fmt.Errorf("rule set validation failed")
	}

	// 3. 디렉터리 내 파일 목록 읽기 (제외 패턴 지정)
	exclusions := []string{"rule.json", "invalid_files", "fileblock.csv", "*.pb"}
	fileNames, err := rules.ListFilesExclude(dirPath, exclusions)
	if err != nil {
		return nil, fmt.Errorf("ReadAllFileNames error: %w", err)
	}

	// 4. 룰 기준으로 map[int]map[string]string 생성
	resultMap, err := rules.GroupFiles(fileNames, ruleSet)
	if err != nil {
		return nil, fmt.Errorf("GroupFiles error: %w", err)
	}

	// 5. 유효/무효 행 분리
	validMap, invalidRows := rules.FilterGroups(resultMap, len(ruleSet.Header))

	// 6. validMap → CSV 파일로 저장
	if err := rules.ExportResultsCSV(validMap, ruleSet.Header, dirPath); err != nil {
		return nil, fmt.Errorf("SaveResultMapToCSV error: %w", err)
	}

	// 7. invalidRows → invalid 파일에 기록
	if err := rules.SaveInvalidFiles(invalidRows, dirPath); err != nil {
		return nil, fmt.Errorf("WriteInvalidFiles error: %w", err)
	}

	// 8. validMap + headers → FileBlock 객체 생성
	fb := ConvertMapToFileBlock(validMap, ruleSet.Header, dirPath)

	// 9. FileBlock → 바이너리 protobuf 파일로 저장
	outPath := filepath.Join(dirPath, filepath.Base(dirPath)+"files.pb")
	if err := protoio.SaveMessage(outPath, fb, 0o777); err != nil {
		return nil, fmt.Errorf("SaveMessage error: %w", err)
	}

	return fb, nil
}

// GenerateFileBlock 일단 이름 고침. filePath 는 rule.josn 이 있는 위치이자 fileblock.csv, invalid_files, *.pb 파일 등이 가 저장될 위치.
func GenerateFileBlock(filePath string, files []string) (*pb.FileBlock, error) {
	// Load the rule set
	ruleSet, err := rules.LoadRuleSetFromFile(filePath) // 이 메서드에서 filepath 의 검증을 해줌.
	if err != nil {
		return nil, fmt.Errorf("failed to load rule set: %w", err)
	}

	// Validate the rule set
	if !rules.IsValidRuleSet(ruleSet) {
		return nil, fmt.Errorf("rule set has conflicts or unused parts")
	}

	resultMap, err := rules.GroupFiles(files, ruleSet)
	if err != nil {
		return nil, fmt.Errorf("failed to blockify files: %w", err)
	}

	// Filter the result map into valid and invalid rows. 열의 갯수 기준으로 유효/무효 행을 분리
	validRows, invalidRows := rules.FilterGroups(resultMap, len(ruleSet.Header))

	// Save valid rows to a CSV file. 사용자에게 보여주기 위함.
	if err := rules.ExportResultsCSV(validRows, ruleSet.Header, filePath); err != nil {
		return nil, fmt.Errorf("failed to save result map to CSV: %w", err)
	}

	// Save invalid rows to a separate file
	if err := rules.SaveInvalidFiles(invalidRows, filePath); err != nil {
		return nil, fmt.Errorf("failed to write invalid files: %w", err)
	}

	// blockId 를 filePath 로 잡아둠.
	fbd := ConvertMapToFileBlock(validRows, ruleSet.Header, filePath)
	pbName := filepath.Join(filePath, fmt.Sprintf("%sfiles.pb", filepath.Base(filePath)))
	err = protoio.SaveMessage(pbName, fbd, 0777)
	if err != nil {
		return nil, fmt.Errorf("failed to save proto message: %w", err)
	}

	return fbd, nil
}
