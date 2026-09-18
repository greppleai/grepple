package navigation

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/greppleai/grepple/api"
)

func TestQualifyExternalDependenciesFromGoModule(t *testing.T) {
	root := t.TempDir()
	mustWriteDependencyFile(t, filepath.Join(root, "go.mod"), `module example.com/service
require github.com/gofiber/fiber/v3 v3.5.0
`)
	mustWriteDependencyFile(t, filepath.Join(root, "go.sum"), "github.com/gofiber/fiber/v3 v3.5.0 h1:exact\n")
	mustWriteDependencyFile(t, filepath.Join(root, "service.go"), "package service\n")
	results := []api.FileResult{{Path: "service.go", Related: []api.RelatedSymbol{{External: &api.ExternalNavigationReference{ID: "ctx", Language: "go", ImportPath: "github.com/gofiber/fiber/v3", Symbol: "Ctx", Kind: "type"}}}}}

	if err := QualifyExternalDependencies(results, root); err != nil {
		t.Fatal(err)
	}
	reference := results[0].Related[0].External
	if reference.Module != "github.com/gofiber/fiber/v3" || reference.Package != "github.com/gofiber/fiber/v3" || reference.Version != "v3.5.0" || reference.Integrity != "h1:exact" {
		t.Fatalf("qualified reference = %#v", reference)
	}
}

func TestQualifyExternalDependenciesLeavesLocalReplacementUnversioned(t *testing.T) {
	root := t.TempDir()
	mustWriteDependencyFile(t, filepath.Join(root, "go.mod"), `module example.com/service
require github.com/gofiber/fiber/v3 v3.5.0
replace github.com/gofiber/fiber/v3 => ../fiber
`)
	mustWriteDependencyFile(t, filepath.Join(root, "service.go"), "package service\n")
	results := []api.FileResult{{Path: "service.go", Related: []api.RelatedSymbol{{External: &api.ExternalNavigationReference{ID: "ctx", Language: "go", ImportPath: "github.com/gofiber/fiber/v3", Symbol: "Ctx", Kind: "type"}}}}}

	if err := QualifyExternalDependencies(results, root); err != nil {
		t.Fatal(err)
	}
	if reference := results[0].Related[0].External; reference.Module != "" || reference.Version != "" {
		t.Fatalf("local replacement was qualified as exact: %#v", reference)
	}
}

func mustWriteDependencyFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
