package main

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const externalAPIProtosImport = "github.com/seoyhaein/api-protos/gen/go/datablock/ichthys"

// forbiddenModulePrefix is checked against go.mod and go.sum verbatim.
const forbiddenModulePrefix = "github.com/seoyhaein/api-protos"

func TestExternalAPIProtosImportGuardrail(t *testing.T) {
	t.Helper()

	fset := token.NewFileSet()
	var found []string
	scanned := 0

	err := filepath.Walk(".", func(path string, info fs.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() && info.Name() == ".git" {
			return filepath.SkipDir
		}
		if info.IsDir() || filepath.Ext(path) != ".go" {
			return nil
		}

		// skip this file itself — the constant above is intentionally the forbidden string
		if filepath.ToSlash(path) == "external_api_protos_import_guardrail_test.go" {
			return nil
		}

		scanned++
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imp := range file.Imports {
			if strings.Trim(imp.Path.Value, "\"") == externalAPIProtosImport {
				found = append(found, filepath.ToSlash(path))
				break
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk repo for import guardrail: %v", err)
	}
	if scanned == 0 {
		t.Fatal("guardrail scanned 0 Go files — working directory is likely wrong")
	}
	if len(found) > 0 {
		t.Fatalf("external api-protos import must be fully removed, found in %v", found)
	}
}

func TestExternalAPIProtosModGuardrail(t *testing.T) {
	t.Helper()

	for _, name := range []string{"go.mod", "go.sum"} {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if strings.Contains(string(data), forbiddenModulePrefix) {
			t.Errorf("%s still references forbidden module %s", name, forbiddenModulePrefix)
		}
	}
}
