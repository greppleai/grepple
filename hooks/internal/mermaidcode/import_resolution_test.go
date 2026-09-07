package mermaidcode

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGoModuleImportResolutionUsesExactImportPath(t *testing.T) {
	root := t.TempDir()
	appPath := filepath.Join(root, "cmd", "app", "app.go")
	libraryPath := filepath.Join(root, "v2", "library.go")
	appText := "package app\nimport widget \"example.com/project/v2\"\nfunc start() { widget.Finish() }\n"
	libraryText := "package widgetpkg\nfunc Finish() {}\n"
	writeTestFile(t, filepath.Join(root, "go.mod"), "module example.com/project\n")
	writeTestFile(t, appPath, appText)
	writeTestFile(t, libraryPath, libraryText)
	sources := []Source{{appPath, appText}, {libraryPath, libraryText}}
	flow := `flowchart TD
 start["start"]
 finish["Finish"]
 %% grepple:language start go
 %% grepple:package start app
 %% grepple:symbol finish Finish
 %% grepple:language finish go
 %% grepple:package finish widgetpkg
 start --> finish
`
	diagnostics, err := CheckFlowchart(flow, sources)
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("exact module resolution: %v %+v", err, diagnostics)
	}
	generated, err := GenerateFlowchart("start", appPath, sources, GenerateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(generated, "%% grepple:package Finish example.com/project/v2") || !strings.Contains(generated, "start --> Finish") {
		t.Fatalf("generated flow omitted exact imported call:\n%s", generated)
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
