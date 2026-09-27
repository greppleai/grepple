package api_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestInternalPackagesDoNotImportPublicAPI keeps implementation and adapters
// independent of the outward-facing facade, including test-only imports.
func TestInternalPackagesDoNotImportPublicAPI(t *testing.T) {
	if err := filepath.WalkDir(filepath.Join("..", "internal"), checkInternalImport(t)); err != nil {
		t.Fatal(err)
	}
}

func checkInternalImport(t *testing.T) fs.WalkDirFunc {
	return func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == "testdata" || entry.Name() == "vendor" {
				return fs.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		return checkGoFileImports(t, path)
	}
}

func checkGoFileImports(t *testing.T, path string) error {
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
	if err != nil {
		return err
	}
	for _, declaration := range file.Imports {
		if err := checkImportPath(t, path, declaration.Path.Value); err != nil {
			return err
		}
	}
	return nil
}

func checkImportPath(t *testing.T, path, literal string) error {
	importPath, err := strconv.Unquote(literal)
	if err != nil {
		return err
	}
	const facade = "github.com/greppleai/grepple/api"
	if importPath == facade || strings.HasPrefix(importPath, facade+"/") {
		t.Errorf("%s imports outward-facing facade %s", path, importPath)
	}
	return nil
}
