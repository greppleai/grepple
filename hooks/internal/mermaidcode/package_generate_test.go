package mermaidcode

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGeneratePackageDiagramIsCompleteDeterministicAndValid(t *testing.T) {
	root := t.TempDir()
	writeScopeFile(t, filepath.Join(root, "go.mod"), "module example.com/project\n")
	packageDir := filepath.Join(root, "service")
	writeScopeFile(t, filepath.Join(packageDir, "model.go"), `package service
	import "github.com/gofiber/fiber/v2"

//grepple:filelocal
type localID string

type ID string
type Alias = ID

type Store interface { Get(ID) (*Record, error) }
type Record struct { ID ID `+"`json:\"id\"`"+` }

type Handler struct{}
func (h *Handler) Register(app *fiber.App) { app.Get("/records", h.List) }
func (h *Handler) List(*fiber.Ctx) error { return nil }
func Build(id ID) *Record { return &Record{ID: id} }
`)
	writeScopeFile(t, filepath.Join(packageDir, "record_methods.go"), "package service\nfunc (Record) Valid() bool { return true }\n")
	writeScopeFile(t, filepath.Join(packageDir, "model_test.go"), "package service_test\ntype Ignored struct{}\n")

	first, err := GeneratePackageDiagram(packageDir)
	if err != nil {
		t.Fatal(err)
	}
	second, err := GeneratePackageDiagram(packageDir)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("package generation was not deterministic")
	}
	for _, want := range []string{
		"%% grepple:complete-package example.com/project/service",
		"<<alias>> Alias", "%% grepple:underlying Alias ID",
		"<<type>> ID", "<<interface>> Store", "<<exact>> Store",
		"<<struct>> Record", "<<exact>> Record", "+Valid(): bool",
		`%% grepple:struct-tag Record ID "json:\"id\""`,
		"%% grepple:filelocal localID",
		"<<function>> Build", "<<export>> Build",
		"%% grepple:route GET /records Handler.List",
		"Build --> ID", "Build --> Record",
	} {
		if !strings.Contains(first, want) {
			t.Errorf("generated diagram missing %q:\n%s", want, first)
		}
	}
	if strings.Contains(first, "Ignored") || strings.Contains(first, "consume") {
		t.Fatalf("generated diagram included a test type or unexported function:\n%s", first)
	}

	sources, err := loadPackageSources(packageDir)
	if err != nil {
		t.Fatal(err)
	}
	diagnostics, err := CheckClassDiagram(first, sources)
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("roundtrip validation: %v %+v", err, diagnostics)
	}
}

func TestGeneratePackageDiagramErrors(t *testing.T) {
	tests := []struct {
		name string
		make func(*testing.T, string)
		want string
	}{
		{"empty", func(_ *testing.T, _ string) {}, "no non-test Go source files"},
		{"typescript only", func(t *testing.T, root string) {
			writeScopeFile(t, filepath.Join(root, "index.ts"), "export class Item {}")
		}, "TypeScript-only"},
		{"multiple packages", func(t *testing.T, root string) {
			writeScopeFile(t, filepath.Join(root, "a.go"), "package first\ntype A struct{}\n")
			writeScopeFile(t, filepath.Join(root, "b.go"), "package second\ntype B struct{}\n")
		}, "exactly one Go package"},
		{"missing module", func(t *testing.T, root string) {
			writeScopeFile(t, filepath.Join(root, "a.go"), "package value\ntype A struct{}\n")
		}, "cannot resolve Go import path"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root, err := os.MkdirTemp("", "mermaid-package-test-")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(root)
			test.make(t, root)
			_, err = GeneratePackageDiagram(root)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("got %v, want error containing %q", err, test.want)
			}
		})
	}
}
