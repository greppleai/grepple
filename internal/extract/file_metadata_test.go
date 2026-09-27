package extract

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestClassFileMetadataReportsWrongDeclarationFile(t *testing.T) {
	root := t.TempDir()
	writeScopeFile(t, filepath.Join(root, "go.mod"), "module example.com/app\n")
	source := Source{filepath.Join(root, "internal", "shard", "shard_http.go"), "package shard\ntype shard struct{}\n"}
	diagram := "classDiagram\n class shard\n <<struct>> shard\n <<go>> shard\n %% grepple:package shard example.com/app/internal/shard\n %% grepple:file shard internal/shard/shard.go\n"

	diagnostics, err := CheckClassDiagram(diagram, []Source{source})
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 1 || diagnostics[0].Line != 6 || diagnostics[0].Message != "Expected 'shard' in file 'internal/shard/shard.go'; found 'internal/shard/shard_http.go'." {
		t.Fatalf("wrong-file diagnostic = %+v", diagnostics)
	}
}

func TestExactGoFileMetadataRequiresMembersInDeclarationFile(t *testing.T) {
	root := t.TempDir()
	writeScopeFile(t, filepath.Join(root, "go.mod"), "module example.com/app\n")
	sources := []Source{
		{filepath.Join(root, "worker.go"), "package app\ntype Worker struct{}\n"},
		{filepath.Join(root, "worker_methods.go"), "package app\nfunc (Worker) Run() {}\n"},
	}
	diagram := "classDiagram\n class Worker {\n  +Run()\n }\n <<struct>> Worker\n <<go>> Worker\n <<exact>> Worker\n %% grepple:package Worker example.com/app\n %% grepple:file Worker worker.go\n"

	diagnostics, err := CheckClassDiagram(diagram, sources)
	if err != nil || len(diagnostics) != 1 || !strings.Contains(diagnostics[0].Message, "found not found") {
		t.Fatalf("cross-file method diagnostic: %v %+v", err, diagnostics)
	}
}

func TestExactGoFileMetadataRejectsUnexpectedCrossFileMethod(t *testing.T) {
	root := t.TempDir()
	writeScopeFile(t, filepath.Join(root, "go.mod"), "module example.com/app\n")
	sources := []Source{
		{filepath.Join(root, "worker.go"), "package app\ntype Worker struct{}\n"},
		{filepath.Join(root, "worker_methods.go"), "package app\nfunc (Worker) Extra() {}\n"},
	}
	diagram := "classDiagram\n class Worker\n <<struct>> Worker\n <<go>> Worker\n <<exact>> Worker\n %% grepple:package Worker example.com/app\n %% grepple:file Worker worker.go\n"

	diagnostics, err := CheckClassDiagram(diagram, sources)
	if err != nil || len(diagnostics) != 1 || !strings.Contains(diagnostics[0].Message, "Unexpected public Extra()") {
		t.Fatalf("cross-file exact diagnostic: %v %+v", err, diagnostics)
	}
}

func TestGeneratedClassUsesCanonicalMetadataAndFile(t *testing.T) {
	root := t.TempDir()
	writeScopeFile(t, filepath.Join(root, "go.mod"), "module example.com/app\n")
	source := Source{filepath.Join(root, "model", "user.go"), "package model\ntype User struct { Name string }\n"}

	diagram, err := GenerateClassDiagram("User", source, []Source{source}, GenerateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"%% grepple:generated entry User",
		"%% grepple:package User example.com/app/model",
		"%% grepple:file User model/user.go",
	} {
		if !strings.Contains(diagram, expected) {
			t.Fatalf("generated output missing %q:\n%s", expected, diagram)
		}
	}
	if strings.Contains(diagram, "%% pi:") {
		t.Fatalf("generated legacy metadata:\n%s", diagram)
	}
}

func TestLegacyPiMetadataStillParses(t *testing.T) {
	diagram := "classDiagram\n class User\n <<typescript>> User\n %% pi:module User user.ts\n"
	if _, err := ParseClassDiagram(diagram); err != nil {
		t.Fatalf("legacy metadata: %v", err)
	}
}
