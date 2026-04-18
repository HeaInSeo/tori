package main

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

const externalAPIProtosImport = "github.com/seoyhaein/api-protos/gen/go/datablock/ichthys"

func TestExternalAPIProtosImportGuardrail(t *testing.T) {
	t.Helper()

	fset := token.NewFileSet()
	var found []string

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

	if len(found) > 0 {
		t.Fatalf("external api-protos import must be fully removed, found in %v", found)
	}
}
