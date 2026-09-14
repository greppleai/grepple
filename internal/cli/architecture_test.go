package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestArchitectureDirectoryAndResolveAcrossLanguages(t *testing.T) {
	root := t.TempDir()
	writeArchitectureFixture(t, root, "grepple.json", `{"ignore":{"paths":["sandbox/**"]}}`)
	writeArchitectureFixture(t, root, "go.mod", "module example.com/project\n")
	writeArchitectureFixture(t, root, "parser/document.go", "package parser\ntype Document struct{}\n")
	writeArchitectureFixture(t, root, "web/component.ts", "export class Component { render() {} }\n")
	writeArchitectureFixture(t, root, "sandbox/ignored.py", "class Ignored:\n    pass\n")
	chdirForConfigTest(t, root)

	output := captureStdout(t, func() {
		if err := runArchitecture([]string{"directory", "--json", "."}); err != nil {
			t.Fatal(err)
		}
	})
	var architecture directoryArchitecture
	if err := json.Unmarshal([]byte(output), &architecture); err != nil {
		t.Fatal(err)
	}
	if architecture.Schema != directoryArchitectureSchema || architecture.Files != 2 || len(architecture.Directories) != 3 {
		t.Fatalf("architecture = %+v", architecture)
	}
	for _, directory := range architecture.Directories {
		if strings.HasPrefix(directory.Path, "sandbox") {
			t.Fatalf("ignored directory included: %+v", directory)
		}
	}

	resolved := captureStdout(t, func() {
		if err := runArchitecture([]string{"resolve", "--symbol", "Document", "--compact", "."}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(resolved, "matches=1") || !strings.Contains(resolved, "parser/document.go:2") {
		t.Fatalf("resolve output:\n%s", resolved)
	}
}

func TestArchitectureWhyUsesResolvedCrossDirectoryCalls(t *testing.T) {
	root := t.TempDir()
	writeArchitectureFixture(t, root, "go.mod", "module example.com/project\n")
	writeArchitectureFixture(t, root, "search/request.go", "package search\nfunc ResolveRequest() {}\n")
	writeArchitectureFixture(t, root, "rulespec/rule.go", "package rulespec\nimport \"example.com/project/search\"\nfunc Validate() { search.ResolveRequest() }\n")
	chdirForConfigTest(t, root)

	output := captureStdout(t, func() {
		if err := runArchitecture([]string{"why", "rulespec", "search", "--compact", "."}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(output, "evidence=1") || !strings.Contains(output, "rulespec/rule.go:3 Validate -> ResolveRequest [import-resolved]") {
		t.Fatalf("why output:\n%s", output)
	}
}

func TestArchitectureHelpStopsBeforeAnalysis(t *testing.T) {
	output := captureStdout(t, func() {
		if err := runArchitecture([]string{"directory", "--help"}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(output, "--compact") || strings.Contains(output, "requires exactly one") {
		t.Fatalf("help output:\n%s", output)
	}
}

func writeArchitectureFixture(t testing.TB, root, relative, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
