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
	cfgPath := writeTempConfig(t, `{"rootDir":"/tmp","filesExclusions":["*.txt"]}`)
	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	if cfg.RootDir != "/tmp" {
		t.Errorf("RootDir mismatch: %s", cfg.RootDir)
	}
	if len(cfg.FilesExclusions) != 1 || cfg.FilesExclusions[0] != "*.txt" {
		t.Errorf("Exclusions mismatch: %v", cfg.FilesExclusions)
	}
}

func TestLoadConfig_DefaultExclusions(t *testing.T) {
	cfgPath := writeTempConfig(t, `{"rootDir":"/tmp"}`)
	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	if len(cfg.FilesExclusions) == 0 {
		t.Errorf("expected default exclusions")
	}
}

func TestLoadConfig_MissingRootDir(t *testing.T) {
	cfgPath := writeTempConfig(t, `{"filesExclusions":[]}`)
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
