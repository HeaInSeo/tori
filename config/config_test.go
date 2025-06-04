package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTempConfig(t *testing.T, data string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg.json")
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	return path
}

func TestLoadConfig(t *testing.T) {
	cfgPath := writeTempConfig(t, `{"rootDir":"/tmp","exclusions":["*.txt"]}`)
	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	if cfg.RootDir != "/tmp" {
		t.Errorf("RootDir mismatch: %s", cfg.RootDir)
	}
	if len(cfg.Exclusions) != 1 || cfg.Exclusions[0] != "*.txt" {
		t.Errorf("Exclusions mismatch: %v", cfg.Exclusions)
	}
}

func TestLoadConfig_DefaultExclusions(t *testing.T) {
	cfgPath := writeTempConfig(t, `{"rootDir":"/tmp"}`)
	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	if len(cfg.Exclusions) == 0 {
		t.Errorf("expected default exclusions")
	}
}

func TestLoadConfig_MissingRootDir(t *testing.T) {
	cfgPath := writeTempConfig(t, `{"exclusions":[]}`)
	if _, err := LoadConfig(cfgPath); err == nil {
		t.Errorf("expected error for missing rootDir")
	}
}

func TestDefaultConfigPath(t *testing.T) {
	path := defaultConfigPath()
	if !strings.HasSuffix(path, filepath.Join("config", "config.json")) {
		t.Errorf("unexpected default path: %s", path)
	}
}
