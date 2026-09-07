package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGoCLIContracts(t *testing.T) {
	directory := t.TempDir()
	source := filepath.Join(directory, "service.go")
	diagram := filepath.Join(directory, "service.mmd")
	output := filepath.Join(directory, "generated.mmd")
	goCode := `package service

type Service struct { Name string }
func (s *Service) Run() { helper() }
func helper() {}
`
	classDiagram := `classDiagram
 class Service {
  +Name: string
  +Run()
 }
 <<struct>> Service
 <<go>> Service
`
	if err := os.WriteFile(source, []byte(goCode), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(diagram, []byte(classDiagram), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := run([]string{"check", "class", diagram, source}); code != 0 {
		t.Fatalf("Go class check returned %d", code)
	}
	if code := run([]string{"generate", "flow", source, "Service.Run", "--source", source, "--output", output}); code != 0 {
		t.Fatalf("Go flow generation returned %d", code)
	}
	if content, err := os.ReadFile(output); err != nil || len(content) == 0 {
		t.Fatalf("generated flow: %q, %v", content, err)
	}
}

func TestGeneratePackageCLI(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/cli\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	packageDir := filepath.Join(root, "model")
	if err := os.Mkdir(packageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packageDir, "model.go"), []byte("package model\ntype Item struct { Name string }\nfunc New() Item { return Item{} }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(root, "schema", "model.mmd")
	if code := run([]string{"generate", "package", packageDir, "--output", output}); code != 0 {
		t.Fatalf("package generation returned %d", code)
	}
	first, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if code := run([]string{"generate", "package", packageDir, "--output", output}); code != 0 {
		t.Fatalf("second package generation returned %d", code)
	}
	second, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatal("CLI package output was not deterministic")
	}
	if code := run([]string{"check", "class", output, packageDir}); code != 0 {
		t.Fatalf("generated package diagram did not roundtrip; code %d", code)
	}
	bundle := filepath.Join(root, "schema", "bundle")
	if code := run([]string{"generate", "package", packageDir, "--format", "bundle", "--output", bundle}); code != 0 {
		t.Fatalf("package bundle generation returned %d", code)
	}
	for _, name := range []string{"manifest.json", "overview.mmd", "structure.mmd"} {
		if _, err := os.Stat(filepath.Join(bundle, name)); err != nil {
			t.Fatalf("bundle artifact %s: %v", name, err)
		}
	}
	if code := run([]string{"check", "package", bundle, packageDir}); code != 0 {
		t.Fatalf("package bundle check returned %d", code)
	}
	if code := run([]string{"check", "package", bundle}); code != 0 {
		t.Fatalf("self-locating package bundle check returned %d", code)
	}
}

func TestWorkspaceCLI(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/workspacecli\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(root, "docs", "project.workspace")
	if code := run([]string{"generate", "workspace", root, "--output", output}); code != 0 {
		t.Fatalf("workspace generation returned %d", code)
	}
	for _, name := range []string{"manifest.json", "overview.mmd"} {
		if _, err := os.Stat(filepath.Join(output, name)); err != nil {
			t.Fatalf("workspace artifact %s: %v", name, err)
		}
	}
	if code := run([]string{"check", "workspace", output}); code != 0 {
		t.Fatalf("self-locating workspace check returned %d", code)
	}
	if code := run([]string{"generate", "workspace", root}); code != 2 {
		t.Fatalf("workspace generation without output returned %d", code)
	}
}

func TestGeneratePackageCLIErrors(t *testing.T) {
	typeScript := t.TempDir()
	if err := os.WriteFile(filepath.Join(typeScript, "index.ts"), []byte("export class Item {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := run([]string{"generate", "package", typeScript}); code != 2 {
		t.Fatalf("TypeScript-only package generation returned %d", code)
	}
	if code := run([]string{"generate", "package"}); code != 2 {
		t.Fatalf("missing directory returned %d", code)
	}
	if code := run([]string{"generate", "package", typeScript, "--format", "bundle"}); code != 2 {
		t.Fatalf("bundle without output returned %d", code)
	}
}
