package main

import (
	"io/fs"
	"go/parser"
	"go/token"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const externalAPIProtosImport = "github.com/seoyhaein/api-protos/gen/go/datablock/ichthys"

var allowedExternalAPIProtosImportFiles = []string{
	"block/fileblock.go",
	"block/fileblock_test.go",
	"block/merge.go",
	"block/proto.go",
	"protoio/protoio.go",
	"protoio/protoio_test.go",
	"service/service.go",
	"service/service_test.go",
	"transport/grpc/datablock_service.go",
	"transport/grpc/datablock_service_test.go",
	"transport/grpc/pb_seam.go",
}

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

	slices.Sort(found)

	for _, path := range found {
		if !slices.Contains(allowedExternalAPIProtosImportFiles, path) {
			t.Fatalf("unauthorized external api-protos import found in %s", path)
		}
	}

	for _, allowed := range allowedExternalAPIProtosImportFiles {
		if !slices.Contains(found, allowed) {
			t.Fatalf("guardrail baseline drift: expected allowed import file %s was not found", allowed)
		}
	}
}
