package extract

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGeneratePackageSummaryUsesCanonicalIRAndPublicSurface(t *testing.T) {
	root := t.TempDir()
	writeSummaryFixture(t, filepath.Join(root, "go.mod"), "module example.com/project\n")
	writeSummaryFixture(t, filepath.Join(root, "api", "api.go"), `// Package api serves widgets.
package api

type Widget struct{ ID string }
type internalState struct{}
func NewWidget() *Widget { return &Widget{} }
`)
	first, err := GeneratePackageSummary(filepath.Join(root, "api"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := GeneratePackageSummary(filepath.Join(root, "api"))
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("package summary is not deterministic")
	}
	for _, expected := range []string{"# Package `example.com/project/api`", "Package api serves widgets.", "2 declarations (1 exported, 1 internal)", "`Widget` (struct)", "`NewWidget() *Widget` (function)", "api/api.go:"} {
		if !strings.Contains(first, expected) {
			t.Fatalf("package summary missing %q:\n%s", expected, first)
		}
	}
	if strings.Contains(first, "`internalState` (struct)") {
		t.Fatalf("package summary exposed internal type:\n%s", first)
	}
}

func TestPackageSummaryBoundsPublicSurface(t *testing.T) {
	ir := &PackageIR{Package: PackageIdentity{Name: "sample"}}
	for index := 0; index < maxPackageSummarySurfaceItems+5; index++ {
		ir.Declarations = append(ir.Declarations, PackageDeclaration{Name: fmt.Sprintf("Type%02d", index), Kind: "struct", File: "sample.go", Line: index + 1})
	}
	ir.Summary = packageSummary(ir)
	summary := renderPackageSummary(ir)
	if strings.Count(summary, " (struct) — ") != maxPackageSummarySurfaceItems || !strings.Contains(summary, "5 additional entries omitted") {
		t.Fatalf("unbounded package summary:\n%s", summary)
	}
}

func TestGenerateWorkspaceSummaryShowsBoundedOwnershipAndDependencies(t *testing.T) {
	root := t.TempDir()
	writeSummaryFixture(t, filepath.Join(root, "go.mod"), "module example.com/project\n")
	writeSummaryFixture(t, filepath.Join(root, "api", "api.go"), "package api\ntype Widget struct{}\n")
	writeSummaryFixture(t, filepath.Join(root, "service", "service.go"), "package service\nimport \"example.com/project/api\"\nfunc Load() api.Widget { return api.Widget{} }\n")
	summary, err := GenerateWorkspaceSummary(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"# Workspace `example.com/project`", "1 modules, 2 packages", "`api` — package `api`", "`service` — package `service`", "`service` → `api`"} {
		if !strings.Contains(summary, expected) {
			t.Fatalf("workspace summary missing %q:\n%s", expected, summary)
		}
	}
}

func writeSummaryFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
