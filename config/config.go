package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	globallog "github.com/seoyhaein/tori/log"
)

type Config struct {
	RootDir           string   `json:"rootDir"`           // lustre-client 마운트된 폴더로 사용할 예정.
	FoldersExclusions []string `json:"foldersExclusions"` // 제외할 폴더들.
	FilesExclusions   []string `json:"filesExclusions"`   // ["*.json", "invalid_files", "*.csv", "*.pb"]
}

var (
	GlobalConfig *Config
	logger       = globallog.Log
)

func init() {
	wd, err := os.Getwd()
	if err != nil {
		fmt.Printf("failed to get working directory: %v\n", err)
	} else {
		fmt.Printf("Current working directory: %s\n", wd)
	}

	// config 설정 TODO 추후 배포시에 설정해줘야 함.
	cfgFile := os.Getenv("CONFIG_FILE")
	if cfgFile == "" {
		cfgFile = defaultConfigPath()
	}
	config, err := LoadConfig(cfgFile)
	// Important 기억하자. os.Exit(1) 로만 하지 말고 Log.Fatalf 를 써서 오류 사항을 명확히 하자. 자체적으로 os.Exit(1) 처리됨.
	if err != nil {
		logger.Fatalf("failed to load config file %v", err)
	}
	GlobalConfig = config
}

func LoadConfig(filename string) (cfg *Config, err error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	// defer 내에서 err 가 이미 설정되어 있지 않은 경우에만 파일 닫기 에러를 처리
	defer func() {
		if cErr := file.Close(); cErr != nil && err == nil {
			logger.Warnf("failed to close file: %v", cErr)
		}
	}()

	decoder := json.NewDecoder(file)
	var config Config
	if err := decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("failed to decode configuration: %w", err)
	}

	// 필수 항목 검증
	if config.RootDir == "" {
		return nil, fmt.Errorf("missing 'rootDir' in configuration")
	}

	// Exclusions 가 비어있으면 기본값 설정
	if len(config.FilesExclusions) == 0 {
		config.FilesExclusions = []string{"*.json", "invalid_files", "*.csv", "*.pb"}
	}

	return &config, nil
}

// defaultConfigPath 는 config.go 파일 기준으로 config.json 파일의 경로를 유추한다.
func defaultConfigPath() string {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		// Caller 실패 시 기본값 반환
		return "config.json"
	}
	return filepath.Join(filepath.Dir(filename), "config.json")
}
