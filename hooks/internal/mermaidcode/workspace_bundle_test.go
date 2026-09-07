package mermaidcode

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkspaceBundleNestedModulesImportsRoutesAndCanonicalCheck(t *testing.T) {
	root := t.TempDir()
	writeScopeFile(t, filepath.Join(root, "go.mod"), `module example.com/root
require (
 github.com/gofiber/fiber/v2 v2.0.0
 github.com/acme/lib v1.0.0
 github.com/acme/lib/sub v1.0.0
)
`)
	writeScopeFile(t, filepath.Join(root, "main.go"), `// Package main starts the command. This is not copied.
package main
import (
 "fmt"
 "example.com/root/service"
 "github.com/acme/lib/sub/client"
)
func main() { fmt.Println(service.Name) }
`)
	writeScopeFile(t, filepath.Join(root, "service", "service.go"), `// Package service provides exact things. More text.
package service
import "github.com/gofiber/fiber/v2"
const Name = "service"
type Handler struct{}
func Exported() {}
func (h *Handler) Register(app *fiber.App) { app.Get("/things", h.List) }
func (h *Handler) List(*fiber.Ctx) error { return nil }
`)
	writeScopeFile(t, filepath.Join(root, "service", "service_test.go"), "package service\nfunc TestIgnored() {}\n")
	writeScopeFile(t, filepath.Join(root, "nested", "go.mod"), "module example.com/nested\nrequire github.com/acme/lib v1.0.0\n")
	writeScopeFile(t, filepath.Join(root, "nested", "model", "model.go"), "package model\nimport _ \"github.com/acme/lib/plugin\"\ntype Model struct{}\n")

	first, err := GenerateWorkspaceBundle(root)
	if err != nil {
		t.Fatal(err)
	}
	second, err := GenerateWorkspaceBundle(root)
	if err != nil {
		t.Fatal(err)
	}
	if string(first.Manifest) != string(second.Manifest) || string(first.Overview) != string(second.Overview) {
		t.Fatal("workspace bundle is not deterministic")
	}
	var manifest WorkspaceIR
	if err := json.Unmarshal(first.Manifest, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Summary.Modules != 2 || manifest.Summary.Packages != 3 || manifest.Summary.FiberRoutes != 1 {
		t.Fatalf("summary = %+v", manifest.Summary)
	}
	if manifest.Modules[0].Directory != "." || manifest.Modules[1].Directory != "nested" {
		t.Fatalf("modules = %+v", manifest.Modules)
	}
	mainPackage := manifest.Modules[0].Packages[0]
	if !mainPackage.Main || mainPackage.Documentation != "Package main starts the command." || strings.Join(mainPackage.StandardLibraryImports, ",") != "fmt" || strings.Join(mainPackage.LocalImports, ",") != "example.com/root/service" {
		t.Fatalf("main package = %+v", mainPackage)
	}
	if len(mainPackage.ExternalImports) != 1 || mainPackage.ExternalImports[0].ModulePath != "github.com/acme/lib/sub" {
		t.Fatalf("longest external mapping = %+v", mainPackage.ExternalImports)
	}
	if !strings.Contains(string(first.Overview), "flowchart LR") || !strings.Contains(string(first.Overview), "class package_0 entrypoint") || !strings.Contains(string(first.Overview), "GET /things -&gt; Handler.List") {
		t.Fatalf("overview facts missing:\n%s", first.Overview)
	}
	output := filepath.Join(root, "docs", "project.workspace")
	if err := WriteWorkspaceBundle(output, first); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(output)
	if err != nil || len(entries) != 2 {
		t.Fatalf("bundle entries = %v, %v", entries, err)
	}
	if err := CheckWorkspaceBundle(output, ""); err != nil {
		t.Fatalf("self-locating check: %v", err)
	}
	if err := os.WriteFile(filepath.Join(output, "overview.mmd"), append(first.Overview, ' '), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CheckWorkspaceBundle(output, root); err == nil || !strings.Contains(err.Error(), "overview.mmd") {
		t.Fatalf("drift error = %v", err)
	}
}

func TestWorkspaceBundleSafePathsAndStrictEntries(t *testing.T) {
	root := t.TempDir()
	writeScopeFile(t, filepath.Join(root, "go.mod"), "module example.com/safe\n")
	writeScopeFile(t, filepath.Join(root, "safe.go"), "package safe\n")
	bundle, err := GenerateWorkspaceBundle(root)
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(root, "workspace")
	if err := os.Symlink(root, output); err != nil {
		t.Fatal(err)
	}
	if err := WriteWorkspaceBundle(output, bundle); err == nil {
		t.Fatal("symlink output accepted")
	}
	if err := os.Remove(output); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(output, 0o755); err != nil {
		t.Fatal(err)
	}
	writeScopeFile(t, filepath.Join(output, "notes.txt"), "user content")
	if err := WriteWorkspaceBundle(output, bundle); err == nil || !strings.Contains(err.Error(), "unexpected") {
		t.Fatalf("unexpected-entry error = %v", err)
	}
}
