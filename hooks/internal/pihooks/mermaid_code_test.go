package pihooks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	mermaidcode "github.com/greppleai/grepple/extract"
)

func TestAnalyzeMermaidSchemasClassAndFlowMismatches(t *testing.T) {
	root := t.TempDir()
	writeHookTestFile(t, filepath.Join(root, "code.go"), "package sample\ntype Worker struct { Count int }\nfunc start() {}\n")
	writeHookTestFile(t, filepath.Join(root, "worker.structure.mmd"), "classDiagram\n class Worker {\n  +Count: string\n }\n <<struct>> Worker\n")
	writeHookTestFile(t, filepath.Join(root, "calls.flow.mmd"), "flowchart TD\n start[\"start\"]\n finish[\"finish\"]\n start --> finish\n")
	diagnostics, err := AnalyzeMermaidSchemas(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 2 {
		t.Fatalf("got diagnostics: %+v", diagnostics)
	}
	messages := diagnostics[0].Failure + "\n" + diagnostics[1].Failure
	if !strings.Contains(messages, "Expected public Count: string") || !strings.Contains(messages, "Missing code symbol 'finish'") {
		t.Fatalf("missing class/flow mismatches: %s", messages)
	}
	for _, diagnostic := range diagnostics {
		if diagnostic.RuleName != "mermaid-code" || diagnostic.Position.Start.Line == 0 {
			t.Fatalf("invalid diagnostic: %+v", diagnostic)
		}
	}
}

func TestAnalyzeMermaidSchemasNoSchemaDoesNotDiscoverSources(t *testing.T) {
	root := t.TempDir()
	writeHookTestFile(t, filepath.Join(root, "broken.go"), "package broken\ntype Broken struct {")
	writeHookTestFile(t, filepath.Join(root, "generated", "ignored.class.mmd"), "not Mermaid")
	diagnostics, err := AnalyzeMermaidSchemas(root)
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("no-schema result: %+v, %v", diagnostics, err)
	}
}

func TestAnalyzeMermaidSchemasSurfacesSyntaxErrors(t *testing.T) {
	root := t.TempDir()
	writeHookTestFile(t, filepath.Join(root, "code.ts"), "class Good {}\n")
	path := filepath.Join(root, "broken.class.mmd")
	writeHookTestFile(t, path, "classDiagram\n class Broken {\n")
	_, err := AnalyzeMermaidSchemas(root)
	if err == nil || !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "missing '}'") {
		t.Fatalf("unexpected syntax error: %v", err)
	}
}

func TestAnalyzeMermaidSchemasDeterministicOrdering(t *testing.T) {
	root := t.TempDir()
	writeHookTestFile(t, filepath.Join(root, "code.go"), "package sample\n")
	writeHookTestFile(t, filepath.Join(root, "z.class.mmd"), "classDiagram\n class Zed\n <<struct>> Zed\n")
	writeHookTestFile(t, filepath.Join(root, "a.flow.mmd"), "flowchart TD\n absent[\"absent\"]\n")
	first, err := AnalyzeMermaidSchemas(root)
	if err != nil {
		t.Fatal(err)
	}
	second, err := AnalyzeMermaidSchemas(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 2 || len(second) != 2 {
		t.Fatalf("unexpected diagnostics: %+v", first)
	}
	for index := range first {
		if first[index].Position.Start.Filename != second[index].Position.Start.Filename || first[index].Failure != second[index].Failure {
			t.Fatal("ordering changed")
		}
	}
	if filepath.Base(first[0].Position.Start.Filename) != "a.flow.mmd" {
		t.Fatalf("not path sorted: %+v", first)
	}
}

func TestLintEvaluationAppendsMermaidDiagnostics(t *testing.T) {
	root := t.TempDir()
	writeHookTestFile(t, filepath.Join(root, "code.go"), "package sample\n")
	writeHookTestFile(t, filepath.Join(root, "missing.class.mmd"), "classDiagram\n class Missing\n <<struct>> Missing\n")
	output := evaluateLintResult(commandResult{stdout: "[]"}, root, filepath.Join("..", ".."), nil)
	if !strings.Contains(string(output), "mermaid-code") || !strings.Contains(string(output), "Missing struct") {
		t.Fatalf("schema diagnostic not in Stop feedback: %s", output)
	}
}

func TestAnalyzeMermaidSchemasSurfacesSourceErrors(t *testing.T) {
	root := t.TempDir()
	writeHookTestFile(t, filepath.Join(root, "broken.go"), "package broken\ntype Broken struct {")
	writeHookTestFile(t, filepath.Join(root, "broken.class.mmd"), "classDiagram\n class Broken\n <<struct>> Broken\n")
	_, err := AnalyzeMermaidSchemas(root)
	if err == nil || !strings.Contains(err.Error(), "broken.go") || !strings.Contains(err.Error(), "malformed syntax") {
		t.Fatalf("unexpected source error: %v", err)
	}
}

func TestAnalyzeMermaidSchemasPackageBundles(t *testing.T) {
	testAnalyzeBundleSchemas(t, makeHookPackageBundle, "bundle", "structure.mmd", "")
}

func TestAnalyzeMermaidSchemasWorkspaceBundles(t *testing.T) {
	testAnalyzeBundleSchemas(t, makeHookWorkspaceBundle, "workspace bundle", "overview.mmd", "canonical workspace bundle mismatch")
}

func TestAnalyzeMermaidSchemasBundleAndLegacyCoexistence(t *testing.T) {
	root := t.TempDir()
	writeHookTestFile(t, filepath.Join(root, "go.mod"), "module example.com/coexist\n")
	packageSource := filepath.Join(root, "model")
	writeHookTestFile(t, filepath.Join(packageSource, "model.go"), "package model\ntype Item struct{}\n")

	packageBundle, err := mermaidcode.GeneratePackageBundle(packageSource)
	if err != nil {
		t.Fatal(err)
	}
	packageDirectory := filepath.Join(root, ".grepple", "model.package")
	if err := mermaidcode.WritePackageBundle(packageDirectory, packageBundle); err != nil {
		t.Fatal(err)
	}
	workspaceBundle, err := mermaidcode.GenerateWorkspaceBundle(root)
	if err != nil {
		t.Fatal(err)
	}
	workspaceDirectory := filepath.Join(root, ".grepple", "project.workspace")
	if err := mermaidcode.WriteWorkspaceBundle(workspaceDirectory, workspaceBundle); err != nil {
		t.Fatal(err)
	}

	writeHookTestFile(t, filepath.Join(root, "missing.class.mmd"), "classDiagram\n class Missing\n <<struct>> Missing\n")
	writeHookTestFile(t, filepath.Join(packageDirectory, "overview.mmd"), "package drift")
	writeHookTestFile(t, filepath.Join(workspaceDirectory, "overview.mmd"), "workspace drift")
	diagnostics, err := AnalyzeMermaidSchemas(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 3 {
		t.Fatalf("coexisting schema diagnostics: %+v", diagnostics)
	}
	messages := diagnostics[0].Failure + "\n" + diagnostics[1].Failure + "\n" + diagnostics[2].Failure
	for _, expected := range []string{"canonical package bundle mismatch", "canonical workspace bundle mismatch", "Missing struct 'Missing'"} {
		if !strings.Contains(messages, expected) {
			t.Fatalf("missing %q in diagnostics: %+v", expected, diagnostics)
		}
	}
}

type hookBundleMaker func(*testing.T) (string, string)

func makeHookPackageBundle(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	writeHookTestFile(t, filepath.Join(root, "go.mod"), "module example.com/hookbundle\n")
	source := filepath.Join(root, "internal", "model")
	writeHookTestFile(t, filepath.Join(source, "model.go"), "package model\ntype Item struct { Name string }\n")
	bundle, err := mermaidcode.GeneratePackageBundle(source)
	if err != nil {
		t.Fatal(err)
	}
	bundleDirectory := filepath.Join(root, ".grepple", "model.package")
	if err := mermaidcode.WritePackageBundle(bundleDirectory, bundle); err != nil {
		t.Fatal(err)
	}
	return root, bundleDirectory
}

func makeHookWorkspaceBundle(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	writeHookTestFile(t, filepath.Join(root, "go.mod"), "module example.com/hookworkspace\n")
	writeHookTestFile(t, filepath.Join(root, "main.go"), "package main\nfunc main() {}\n")
	bundle, err := mermaidcode.GenerateWorkspaceBundle(root)
	if err != nil {
		t.Fatal(err)
	}
	bundleDirectory := filepath.Join(root, ".grepple", "project.workspace")
	if err := mermaidcode.WriteWorkspaceBundle(bundleDirectory, bundle); err != nil {
		t.Fatal(err)
	}
	return root, bundleDirectory
}

func testAnalyzeBundleSchemas(t *testing.T, makeBundle hookBundleMaker, label, generatedFile, requiredFailure string) {
	t.Helper()
	t.Run("valid", func(t *testing.T) {
		root, _ := makeBundle(t)
		diagnostics, err := AnalyzeMermaidSchemas(root)
		if err != nil || len(diagnostics) != 0 {
			t.Fatalf("valid %s: %+v, %v", label, diagnostics, err)
		}
	})
	mutations := []struct {
		name, file string
		mutate     func(string) error
	}{
		{"changed", "overview.mmd", func(path string) error { return os.WriteFile(path, []byte("changed"), 0o644) }},
		{"missing manifest", "manifest.json", os.Remove},
		{"missing", generatedFile, os.Remove},
		{"extra", "notes.txt", func(path string) error { return os.WriteFile(path, []byte("extra"), 0o644) }},
	}
	for _, test := range mutations {
		t.Run(test.name, func(t *testing.T) {
			testAnalyzeBundleMutation(t, makeBundle, label, requiredFailure, test.file, test.mutate)
		})
	}
	t.Run("no double validation", func(t *testing.T) {
		root, bundleDirectory := makeBundle(t)
		writeHookTestFile(t, filepath.Join(bundleDirectory, "bad.structure.mmd"), "not a diagram")
		diagnostics, err := AnalyzeMermaidSchemas(root)
		if err != nil || len(diagnostics) != 1 || !strings.Contains(diagnostics[0].Failure, "unexpected file") {
			t.Fatalf("%s double validation result: %+v, %v", label, diagnostics, err)
		}
	})
}

func testAnalyzeBundleMutation(t *testing.T, makeBundle hookBundleMaker, label, requiredFailure, file string, mutate func(string) error) {
	t.Helper()
	root, bundleDirectory := makeBundle(t)
	if err := mutate(filepath.Join(bundleDirectory, file)); err != nil {
		t.Fatal(err)
	}
	diagnostics, err := AnalyzeMermaidSchemas(root)
	if err != nil || len(diagnostics) != 1 || !strings.Contains(diagnostics[0].Failure, file) {
		t.Fatalf("%s diagnostic: %+v, %v", label, diagnostics, err)
	}
	if requiredFailure != "" && !strings.Contains(diagnostics[0].Failure, requiredFailure) {
		t.Fatalf("%s diagnostic: %+v", label, diagnostics[0])
	}
	if diagnostics[0].Position.Start.Filename != filepath.Join(bundleDirectory, "manifest.json") {
		t.Fatalf("diagnostic anchor: %+v", diagnostics[0])
	}
}

func writeHookTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
