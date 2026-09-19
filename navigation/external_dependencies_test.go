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

func TestQualifyExternalDependenciesFromNPMLockVersions(t *testing.T) {
	tests := []struct {
		name string
		lock string
	}{
		{name: "v1-legacy-dependencies", lock: `{"lockfileVersion":1,"dependencies":{"@acme/widgets":{"version":"1.2.3","resolved":"https://registry.npmjs.org/@acme/widgets/-/widgets-1.2.3.tgz","integrity":"sha512-v1"}}}`},
		{name: "v2-packages", lock: `{"lockfileVersion":2,"packages":{"node_modules/@acme/widgets":{"version":"1.2.3","resolved":"https://registry.npmjs.org/@acme/widgets/-/widgets-1.2.3.tgz","integrity":"sha512-v2"}}}`},
		{name: "v2-legacy-fallback", lock: `{"lockfileVersion":2,"dependencies":{"@acme/widgets":{"version":"1.2.3","resolved":"https://registry.npmjs.org/@acme/widgets/-/widgets-1.2.3.tgz","integrity":"sha512-v2-fallback"}}}`},
		{name: "v3-packages", lock: `{"lockfileVersion":3,"packages":{"node_modules/@acme/widgets":{"version":"1.2.3","resolved":"https://registry.npmjs.org/@acme/widgets/-/widgets-1.2.3.tgz","integrity":"sha512-v3"}}}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			mustWriteDependencyFile(t, filepath.Join(root, "package.json"), `{"dependencies":{"@acme/widgets":"^1.2.3"}}`)
			mustWriteDependencyFile(t, filepath.Join(root, "package-lock.json"), test.lock)
			mustWriteDependencyFile(t, filepath.Join(root, "main.ts"), "export {}")
			results := npmExternalDependencyResults()
			if err := QualifyExternalDependencies(results, root); err != nil {
				t.Fatal(err)
			}
			reference := results[0].Related[0].External
			if reference.Module != "@acme/widgets" || reference.Package != "@acme/widgets/subpath" || reference.Version != "1.2.3" || reference.Integrity == "" {
				t.Fatalf("npm reference = %#v", reference)
			}
		})
	}
}

func TestQualifyExternalDependenciesFromNPMShrinkwrapVersions(t *testing.T) {
	tests := []struct {
		name string
		lock string
	}{
		{name: "v1-legacy-dependencies", lock: `{"lockfileVersion":1,"dependencies":{"@acme/widgets":{"version":"1.2.3","resolved":"https://registry.npmjs.org/@acme/widgets/-/widgets-1.2.3.tgz","integrity":"sha512-v1"}}}`},
		{name: "v2-packages", lock: `{"lockfileVersion":2,"packages":{"node_modules/@acme/widgets":{"version":"1.2.3","resolved":"https://registry.npmjs.org/@acme/widgets/-/widgets-1.2.3.tgz","integrity":"sha512-v2"}}}`},
		{name: "v2-legacy-fallback", lock: `{"lockfileVersion":2,"dependencies":{"@acme/widgets":{"version":"1.2.3","resolved":"https://registry.npmjs.org/@acme/widgets/-/widgets-1.2.3.tgz","integrity":"sha512-v2-fallback"}}}`},
		{name: "v3-packages", lock: `{"lockfileVersion":3,"packages":{"node_modules/@acme/widgets":{"version":"1.2.3","resolved":"https://registry.npmjs.org/@acme/widgets/-/widgets-1.2.3.tgz","integrity":"sha512-v3"}}}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			mustWriteDependencyFile(t, filepath.Join(root, "package.json"), `{"dependencies":{"@acme/widgets":"^1.2.3"}}`)
			mustWriteDependencyFile(t, filepath.Join(root, "npm-shrinkwrap.json"), test.lock)
			mustWriteDependencyFile(t, filepath.Join(root, "main.ts"), "export {}")
			results := npmExternalDependencyResults()
			if err := QualifyExternalDependencies(results, root); err != nil {
				t.Fatal(err)
			}
			reference := results[0].Related[0].External
			if reference.Module != "@acme/widgets" || reference.Package != "@acme/widgets/subpath" || reference.Version != "1.2.3" || reference.Integrity == "" {
				t.Fatalf("npm shrinkwrap reference = %#v", reference)
			}
		})
	}
}

func TestQualifyExternalDependenciesPrefersNPMShrinkwrap(t *testing.T) {
	root := t.TempDir()
	mustWriteDependencyFile(t, filepath.Join(root, "package.json"), `{"dependencies":{"@acme/widgets":"^1.0.0"}}`)
	mustWriteDependencyFile(t, filepath.Join(root, "package-lock.json"), `{"lockfileVersion":3,"packages":{"node_modules/@acme/widgets":{"version":"1.0.0","integrity":"sha512-package-lock"}}}`)
	mustWriteDependencyFile(t, filepath.Join(root, "npm-shrinkwrap.json"), `{"lockfileVersion":3,"packages":{"node_modules/@acme/widgets":{"version":"2.0.0","integrity":"sha512-shrinkwrap"}}}`)
	mustWriteDependencyFile(t, filepath.Join(root, "main.ts"), "export {}")
	results := npmExternalDependencyResults()
	if err := QualifyExternalDependencies(results, root); err != nil {
		t.Fatal(err)
	}
	if reference := results[0].Related[0].External; reference.Version != "2.0.0" || reference.Integrity != "sha512-shrinkwrap" {
		t.Fatalf("npm shrinkwrap did not take precedence: %#v", reference)
	}
}

func TestQualifyExternalDependenciesDoesNotFallbackFromInvalidNPMShrinkwrap(t *testing.T) {
	root := t.TempDir()
	mustWriteDependencyFile(t, filepath.Join(root, "package.json"), `{"dependencies":{"@acme/widgets":"^1.0.0"}}`)
	mustWriteDependencyFile(t, filepath.Join(root, "package-lock.json"), `{"lockfileVersion":3,"packages":{"node_modules/@acme/widgets":{"version":"1.0.0","integrity":"sha512-package-lock"}}}`)
	mustWriteDependencyFile(t, filepath.Join(root, "npm-shrinkwrap.json"), `{"lockfileVersion":3`)
	mustWriteDependencyFile(t, filepath.Join(root, "main.ts"), "export {}")
	results := npmExternalDependencyResults()
	if err := QualifyExternalDependencies(results, root); err != nil {
		t.Fatal(err)
	}
	if reference := results[0].Related[0].External; reference.Module != "" || reference.Version != "" || reference.Integrity != "" {
		t.Fatalf("invalid shrinkwrap fell back to package-lock.json: %#v", reference)
	}
}

func TestQualifyExternalDependenciesRejectsNonAuthoritativeNPMLocks(t *testing.T) {
	tests := []struct {
		name string
		lock string
	}{
		{name: "missing-version", lock: `{"packages":{"node_modules/@acme/widgets":{"version":"1.2.3"}}}`},
		{name: "unsupported-version", lock: `{"lockfileVersion":4,"packages":{"node_modules/@acme/widgets":{"version":"1.2.3"}}}`},
		{name: "malformed", lock: `{"lockfileVersion":3`},
		{name: "v1-packages-shape", lock: `{"lockfileVersion":1,"packages":{"node_modules/@acme/widgets":{"version":"1.2.3"}}}`},
		{name: "v3-legacy-shape", lock: `{"lockfileVersion":3,"dependencies":{"@acme/widgets":{"version":"1.2.3"}}}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			mustWriteDependencyFile(t, filepath.Join(root, "package.json"), `{"dependencies":{"@acme/widgets":"^1.2.3"}}`)
			mustWriteDependencyFile(t, filepath.Join(root, "package-lock.json"), test.lock)
			mustWriteDependencyFile(t, filepath.Join(root, "main.ts"), "export {}")
			results := npmExternalDependencyResults()
			if err := QualifyExternalDependencies(results, root); err != nil {
				t.Fatal(err)
			}
			reference := results[0].Related[0].External
			if reference.Module != "" || reference.Version != "" || reference.Integrity != "" {
				t.Fatalf("non-authoritative lock qualified dependency: %#v", reference)
			}
		})
	}
}

func npmExternalDependencyResults() []api.FileResult {
	return []api.FileResult{{Path: "main.ts", Language: "typescript", Related: []api.RelatedSymbol{{External: &api.ExternalNavigationReference{ID: "widget", Language: "typescript", ImportPath: "@acme/widgets/subpath", Symbol: "Widget", Kind: "type"}}}}}
}

func TestQualifyExternalDependenciesFromCargoLock(t *testing.T) {
	root := t.TempDir()
	mustWriteDependencyFile(t, filepath.Join(root, "Cargo.toml"), "[package]\nname='consumer'\nversion='0.1.0'\n[dependencies]\nacme_widgets={ package='acme-widgets', version='1' }\n")
	mustWriteDependencyFile(t, filepath.Join(root, "Cargo.lock"), "version = 3\n[[package]]\nname = 'acme-widgets'\nversion = '1.4.0'\nsource = 'registry+https://github.com/rust-lang/crates.io-index'\nchecksum = 'cargo-sum'\n")
	mustWriteDependencyFile(t, filepath.Join(root, "main.rs"), "fn main() {}")
	results := []api.FileResult{{Path: "main.rs", Language: "rust", Related: []api.RelatedSymbol{{External: &api.ExternalNavigationReference{ID: "widget", Language: "rust", ImportPath: "acme_widgets::Widget", Symbol: "Widget", Kind: "type"}}}}}
	if err := QualifyExternalDependencies(results, root); err != nil {
		t.Fatal(err)
	}
	reference := results[0].Related[0].External
	if reference.Module != "acme-widgets" || reference.Version != "1.4.0" || reference.Integrity != "cargo-sum" {
		t.Fatalf("cargo reference = %#v", reference)
	}
}

func TestQualifyExternalDependenciesPreservesMavenCandidates(t *testing.T) {
	root := t.TempDir()
	mustWriteDependencyFile(t, filepath.Join(root, "pom.xml"), `<project><modelVersion>4.0.0</modelVersion><groupId>consumer</groupId><artifactId>app</artifactId><version>1</version><dependencies><dependency><groupId>com.acme</groupId><artifactId>widgets</artifactId><version>2.3.0</version></dependency><dependency><groupId>com.acme</groupId><artifactId>support</artifactId><version>4.0.0</version></dependency></dependencies></project>`)
	mustWriteDependencyFile(t, filepath.Join(root, "Main.java"), "class Main {}")
	results := []api.FileResult{{Path: "Main.java", Language: "java", Related: []api.RelatedSymbol{{External: &api.ExternalNavigationReference{ID: "widget", Language: "java", ImportPath: "com.acme.widgets.Widget", Symbol: "Widget", Kind: "type"}}}}}
	if err := QualifyExternalDependencies(results, root); err != nil {
		t.Fatal(err)
	}
	reference := results[0].Related[0].External
	if reference.Module != "" || len(reference.Candidates) != 2 || reference.Candidates[0].Module != "com.acme:support" || reference.Candidates[1].Version != "2.3.0" {
		t.Fatalf("maven candidates = %#v", reference)
	}
	if got := ExternalDependencyReferences(results); len(got) != 1 || len(got[0].Candidates) != 2 {
		t.Fatalf("eligible references = %#v", got)
	}
}

func mustWriteDependencyFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
