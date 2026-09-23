package cli

import (
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

const modulePath = "github.com/greppleai/grepple/"

func TestProductionImportPolicy(t *testing.T) {
	root := repositoryRoot(t)
	var violations []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == ".git" || entry.Name() == ".grepple" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, spec := range file.Imports {
			importPath, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return err
			}
			if reason := forbiddenImport(filepath.ToSlash(relative), importPath); reason != "" {
				violations = append(violations, fmt.Sprintf("%s imports %s: %s", filepath.ToSlash(relative), importPath, reason))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) > 0 {
		t.Fatalf("production import policy violations:\n%s", strings.Join(violations, "\n"))
	}
}

func forbiddenImport(file, imported string) string {
	directory := filepath.ToSlash(filepath.Dir(file))
	if strings.HasPrefix(directory, "internal/cli/") && imported == modulePath+"internal/cli" {
		return "command adapters must not import the composition package"
	}
	if strings.HasPrefix(directory, "internal/cli/") && strings.HasPrefix(imported, modulePath+"internal/cli/") {
		own := modulePath + directory
		if imported != own {
			return "command adapters must not import sibling command packages"
		}
	}
	if directory == "internal/cliruntime" && imported == modulePath+"internal/render" {
		return "the command protocol must remain renderer-neutral"
	}
	for _, reusable := range []string{"analysis", "api", "dependency", "extract", "gritql", "navigation", "parser", "rulespec", "search", "internal/boundaryanalysis", "internal/sources", "internal/sourcelocation"} {
		if (directory == reusable || strings.HasPrefix(directory, reusable+"/")) && strings.HasPrefix(imported, modulePath+"internal/cli") {
			return "reusable engines must not depend on CLI adapters"
		}
	}
	for _, domain := range []string{"analysis", "extract", "navigation", "parser", "rulespec", "search", "internal/boundaryanalysis", "internal/sources"} {
		if (directory == domain || strings.HasPrefix(directory, domain+"/")) && imported == modulePath+"api" {
			return "transport-decoupled domain packages must not import API DTOs"
		}
	}
	return ""
}

func TestForbiddenImportRules(t *testing.T) {
	tests := []struct {
		file, imported string
	}{
		{"internal/cli/ask/command.go", modulePath + "internal/cli/graph"},
		{"internal/cliruntime/context.go", modulePath + "internal/render"},
		{"analysis/graph.go", modulePath + "internal/cli"},
		{"navigation/external_dependencies.go", modulePath + "api"},
		{"search/result.go", modulePath + "api"},
		{"rulespec/rule.go", modulePath + "api"},
	}
	for _, test := range tests {
		if reason := forbiddenImport(test.file, test.imported); reason == "" {
			t.Errorf("%s -> %s was allowed", test.file, test.imported)
		}
	}
	if reason := forbiddenImport("internal/cli/ask/command.go", modulePath+"internal/aiprovider"); reason != "" {
		t.Fatalf("valid import rejected: %s", reason)
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve import policy test path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(current), "..", ".."))
}
