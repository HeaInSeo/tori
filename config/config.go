package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	globallog "github.com/HeaInSeo/tori/log"
)

type Config struct {
	RootDir           string   `json:"rootDir"`           // lustre-client 마운트된 폴더로 사용할 예정.
	FoldersExclusions []string `json:"foldersExclusions"` // 제외할 폴더들.
	FilesExclusions   []string `json:"filesExclusions"`   // ["*.json", "invalid_files", "invalid_files_*", "*.csv", "*.pb"]
}

var (
	GlobalConfig *Config
	logger       = globallog.Log
)

// InitConfig loads the configuration from CONFIG_FILE env or the default path
// and assigns it to GlobalConfig. Must be called explicitly before use.
func InitConfig() error {
	cfgFile := os.Getenv("CONFIG_FILE")
	if cfgFile == "" {
		cfgFile = defaultConfigPath()
	}
	config, err := LoadConfig(cfgFile)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	GlobalConfig = config
	return nil
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
		// "invalid_files_*" 는 projection 층이 만드는 invalid_files_<ts>.txt 리포트를
		// 관측에서 제외한다. 정확 일치 항목 "invalid_files" 는 타임스탬프 이름이
		// 생기기 전 값이라 단독으로는 리포트를 걸러내지 못한다.
		config.FilesExclusions = []string{"*.json", "invalid_files", "invalid_files_*", "*.csv", "*.pb"}
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
