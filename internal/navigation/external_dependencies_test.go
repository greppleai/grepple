package navigation

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/dependency"
)

type externalDependencyTestResult struct {
	Path, Repo, Language string
	Related              []RelatedSymbol
}

func (result externalDependencyTestResult) ExternalDependencyData() ExternalDependencyData {
	return ExternalDependencyData{Path: result.Path, Repo: result.Repo, Language: result.Language, Related: result.Related}
}

func TestQualifyExternalDependenciesFromGoModule(t *testing.T) {
	root := t.TempDir()
	mustWriteDependencyFile(t, filepath.Join(root, "go.mod"), `module example.com/service
require github.com/gofiber/fiber/v3 v3.5.0
`)
	mustWriteDependencyFile(t, filepath.Join(root, "go.sum"), "github.com/gofiber/fiber/v3 v3.5.0 h1:exact\n")
	mustWriteDependencyFile(t, filepath.Join(root, "service.go"), "package service\n")
	results := []externalDependencyTestResult{{Path: "service.go", Related: []RelatedSymbol{{External: &ExternalReference{ID: "ctx", Language: "go", ImportPath: "github.com/gofiber/fiber/v3", Symbol: "Ctx", Kind: "type"}}}}}

	if err := qualifyLocalDependencies(results, root); err != nil {
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
	results := []externalDependencyTestResult{{Path: "service.go", Related: []RelatedSymbol{{External: &ExternalReference{ID: "ctx", Language: "go", ImportPath: "github.com/gofiber/fiber/v3", Symbol: "Ctx", Kind: "type"}}}}}

	if err := qualifyLocalDependencies(results, root); err != nil {
		t.Fatal(err)
	}
	if reference := results[0].Related[0].External; reference.Module != "" || reference.Version != "" {
		t.Fatalf("local replacement was qualified as exact: %#v", reference)
	}
}

func TestQualifyExternalDependenciesHonorsCorepackPackageManager(t *testing.T) {
	validHash := strings.Repeat("a", 56)
	tests := []struct {
		name           string
		packageManager string
		qualified      bool
	}{
		{name: "absent", qualified: true},
		{name: "exact npm", packageManager: "npm@10.8.2", qualified: true},
		{name: "exact npm with corepack hash", packageManager: "npm@10.8.2+sha224." + validHash, qualified: true},
		{name: "yarn selected", packageManager: "yarn@4.5.0"},
		{name: "pnpm selected", packageManager: "pnpm@9.12.0"},
		{name: "bun selected", packageManager: "bun@1.1.29"},
		{name: "floating npm major", packageManager: "npm@10"},
		{name: "floating npm tag", packageManager: "npm@latest"},
		{name: "missing npm version", packageManager: "npm@"},
		{name: "malformed corepack hash", packageManager: "npm@10.8.2+sha224.short"},
		{name: "surrounding whitespace", packageManager: " npm@10.8.2"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			manifest := map[string]any{"dependencies": map[string]string{"@acme/widgets": "^1.2.3"}}
			if test.packageManager != "" {
				manifest["packageManager"] = test.packageManager
			}
			content, err := json.Marshal(manifest)
			if err != nil {
				t.Fatal(err)
			}
			mustWriteDependencyFile(t, filepath.Join(root, "package.json"), string(content))
			mustWriteDependencyFile(t, filepath.Join(root, "package-lock.json"), `{"lockfileVersion":3,"packages":{"node_modules/@acme/widgets":{"version":"1.2.3","resolved":"https://registry.npmjs.org/@acme/widgets/-/widgets-1.2.3.tgz","integrity":"sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="}}}`)
			mustWriteDependencyFile(t, filepath.Join(root, "yarn.lock"), "stale conflicting lock")
			mustWriteDependencyFile(t, filepath.Join(root, "pnpm-lock.yaml"), "lockfileVersion: '9.0'")
			mustWriteDependencyFile(t, filepath.Join(root, "main.ts"), "export {}")
			results := npmExternalDependencyResults()
			if err := qualifyLocalDependencies(results, root); err != nil {
				t.Fatal(err)
			}
			reference := results[0].Related[0].External
			if got := reference.Module != ""; got != test.qualified {
				t.Fatalf("packageManager %q qualified=%v, reference=%#v", test.packageManager, got, reference)
			}
		})
	}
}

func TestQualifyExternalDependenciesFromNPMLockVersions(t *testing.T) {
	tests := []struct {
		name string
		lock string
	}{
		{name: "v1-legacy-dependencies", lock: `{"lockfileVersion":1,"dependencies":{"@acme/widgets":{"version":"1.2.3","resolved":"https://registry.npmjs.org/@acme/widgets/-/widgets-1.2.3.tgz","integrity":"sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="}}}`},
		{name: "v2-packages", lock: `{"lockfileVersion":2,"packages":{"node_modules/@acme/widgets":{"version":"1.2.3","resolved":"https://registry.npmjs.org/@acme/widgets/-/widgets-1.2.3.tgz","integrity":"sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="}}}`},
		{name: "v2-legacy-fallback", lock: `{"lockfileVersion":2,"dependencies":{"@acme/widgets":{"version":"1.2.3","resolved":"https://registry.npmjs.org/@acme/widgets/-/widgets-1.2.3.tgz","integrity":"sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="}}}`},
		{name: "v3-packages", lock: `{"lockfileVersion":3,"packages":{"node_modules/@acme/widgets":{"version":"1.2.3","resolved":"https://registry.npmjs.org/@acme/widgets/-/widgets-1.2.3.tgz","integrity":"sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="}}}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			mustWriteDependencyFile(t, filepath.Join(root, "package.json"), `{"dependencies":{"@acme/widgets":"^1.2.3"}}`)
			mustWriteDependencyFile(t, filepath.Join(root, "package-lock.json"), test.lock)
			mustWriteDependencyFile(t, filepath.Join(root, "main.ts"), "export {}")
			results := npmExternalDependencyResults()
			if err := qualifyLocalDependencies(results, root); err != nil {
				t.Fatal(err)
			}
			reference := results[0].Related[0].External
			if reference.Module != "@acme/widgets" || reference.Package != "@acme/widgets/subpath" || reference.Version != "1.2.3" || reference.Integrity == "" || reference.Source != dependency.NPMRegistrySource {
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
		{name: "v1-legacy-dependencies", lock: `{"lockfileVersion":1,"dependencies":{"@acme/widgets":{"version":"1.2.3","resolved":"https://registry.npmjs.org/@acme/widgets/-/widgets-1.2.3.tgz","integrity":"sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="}}}`},
		{name: "v2-packages", lock: `{"lockfileVersion":2,"packages":{"node_modules/@acme/widgets":{"version":"1.2.3","resolved":"https://registry.npmjs.org/@acme/widgets/-/widgets-1.2.3.tgz","integrity":"sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="}}}`},
		{name: "v2-legacy-fallback", lock: `{"lockfileVersion":2,"dependencies":{"@acme/widgets":{"version":"1.2.3","resolved":"https://registry.npmjs.org/@acme/widgets/-/widgets-1.2.3.tgz","integrity":"sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="}}}`},
		{name: "v3-packages", lock: `{"lockfileVersion":3,"packages":{"node_modules/@acme/widgets":{"version":"1.2.3","resolved":"https://registry.npmjs.org/@acme/widgets/-/widgets-1.2.3.tgz","integrity":"sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="}}}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			mustWriteDependencyFile(t, filepath.Join(root, "package.json"), `{"dependencies":{"@acme/widgets":"^1.2.3"}}`)
			mustWriteDependencyFile(t, filepath.Join(root, "npm-shrinkwrap.json"), test.lock)
			mustWriteDependencyFile(t, filepath.Join(root, "main.ts"), "export {}")
			results := npmExternalDependencyResults()
			if err := qualifyLocalDependencies(results, root); err != nil {
				t.Fatal(err)
			}
			reference := results[0].Related[0].External
			if reference.Module != "@acme/widgets" || reference.Package != "@acme/widgets/subpath" || reference.Version != "1.2.3" || reference.Integrity == "" || reference.Source != dependency.NPMRegistrySource {
				t.Fatalf("npm shrinkwrap reference = %#v", reference)
			}
		})
	}
}

func TestQualifyExternalDependenciesPrefersNPMShrinkwrap(t *testing.T) {
	root := t.TempDir()
	mustWriteDependencyFile(t, filepath.Join(root, "package.json"), `{"dependencies":{"@acme/widgets":"^1.0.0"}}`)
	mustWriteDependencyFile(t, filepath.Join(root, "package-lock.json"), `{"lockfileVersion":3,"packages":{"node_modules/@acme/widgets":{"version":"1.0.0","resolved":"https://registry.npmjs.org/@acme/widgets/-/widgets-1.0.0.tgz","integrity":"sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="}}}`)
	mustWriteDependencyFile(t, filepath.Join(root, "npm-shrinkwrap.json"), `{"lockfileVersion":3,"packages":{"node_modules/@acme/widgets":{"version":"2.0.0","resolved":"https://registry.npmjs.org/@acme/widgets/-/widgets-2.0.0.tgz","integrity":"sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="}}}`)
	mustWriteDependencyFile(t, filepath.Join(root, "main.ts"), "export {}")
	results := npmExternalDependencyResults()
	if err := qualifyLocalDependencies(results, root); err != nil {
		t.Fatal(err)
	}
	if reference := results[0].Related[0].External; reference.Version != "2.0.0" || reference.Integrity != "sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=" || reference.Source != dependency.NPMRegistrySource {
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
	if err := qualifyLocalDependencies(results, root); err != nil {
		t.Fatal(err)
	}
	if reference := results[0].Related[0].External; reference.Module != "" || reference.Version != "" || reference.Integrity != "" {
		t.Fatalf("invalid shrinkwrap fell back to package-lock.json: %#v", reference)
	}
}

func TestQualifyExternalDependenciesFromNPMAliases(t *testing.T) {
	tests := []struct {
		name        string
		dependency  string
		lockVersion int
		entry       string
		module      string
		version     string
		packageName string
	}{
		{name: "v1-unscoped", dependency: `"widget-alias":"npm:acme-widget@^1.2.0"`, lockVersion: 1, entry: `"widget-alias":{"version":"npm:acme-widget@1.2.3","resolved":"https://registry.npmjs.org/acme-widget/-/acme-widget-1.2.3.tgz","integrity":"sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="}`, module: "acme-widget", version: "1.2.3", packageName: "acme-widget/subpath"},
		{name: "v2-scoped", dependency: `"widget-alias":"npm:@acme/widgets@^2.0.0"`, lockVersion: 2, entry: `"node_modules/widget-alias":{"name":"@acme/widgets","version":"2.1.0","resolved":"https://registry.npmjs.org/@acme/widgets/-/widgets-2.1.0.tgz","integrity":"sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="}`, module: "@acme/widgets", version: "2.1.0", packageName: "@acme/widgets/subpath"},
		{name: "v3-unscoped", dependency: `"widget-alias":"npm:acme-widget@^3.0.0"`, lockVersion: 3, entry: `"node_modules/widget-alias":{"name":"acme-widget","version":"3.2.1","resolved":"https://registry.npmjs.org/acme-widget/-/acme-widget-3.2.1.tgz","integrity":"sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="}`, module: "acme-widget", version: "3.2.1", packageName: "acme-widget/subpath"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			mustWriteDependencyFile(t, filepath.Join(root, "package.json"), `{"dependencies":{`+test.dependency+`}}`)
			layout := `"packages":{` + test.entry + `}`
			if test.lockVersion == 1 {
				layout = `"dependencies":{` + test.entry + `}`
			}
			mustWriteDependencyFile(t, filepath.Join(root, "package-lock.json"), fmt.Sprintf(`{"lockfileVersion":%d,%s}`, test.lockVersion, layout))
			mustWriteDependencyFile(t, filepath.Join(root, "main.ts"), "export {}")
			results := npmExternalDependencyResultsFor("widget-alias/subpath")
			if err := qualifyLocalDependencies(results, root); err != nil {
				t.Fatal(err)
			}
			reference := results[0].Related[0].External
			if reference.ImportPath != "widget-alias/subpath" || reference.Module != test.module || reference.Version != test.version || reference.Package != test.packageName || reference.Source != dependency.NPMRegistrySource {
				t.Fatalf("npm alias reference = %#v", reference)
			}
		})
	}
}

func TestQualifyExternalDependenciesRejectsMismatchedNPMAliasIdentity(t *testing.T) {
	root := t.TempDir()
	mustWriteDependencyFile(t, filepath.Join(root, "package.json"), `{"dependencies":{"widget-alias":"npm:@acme/widgets@^2.0.0"}}`)
	mustWriteDependencyFile(t, filepath.Join(root, "package-lock.json"), `{"lockfileVersion":3,"packages":{"node_modules/widget-alias":{"name":"@other/widgets","version":"2.1.0","integrity":"sha512-other"}}}`)
	mustWriteDependencyFile(t, filepath.Join(root, "main.ts"), "export {}")
	results := npmExternalDependencyResultsFor("widget-alias/subpath")
	if err := qualifyLocalDependencies(results, root); err != nil {
		t.Fatal(err)
	}
	if reference := results[0].Related[0].External; reference.Module != "" || reference.Version != "" || reference.Package != "" {
		t.Fatalf("mismatched npm alias was qualified: %#v", reference)
	}
}

func TestNormalizeNPMRegistryEvidence(t *testing.T) {
	validIntegrity := "sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
	tests := []struct {
		name      string
		module    string
		resolved  string
		integrity string
		valid     bool
	}{
		{name: "public scoped package", module: "@acme/widgets", resolved: "https://registry.npmjs.org/@acme/widgets/-/widgets-1.2.3.tgz", integrity: validIntegrity, valid: true},
		{name: "public unscoped package", module: "widget", resolved: "https://registry.npmjs.org/widget/-/widget-1.2.3.tgz", integrity: validIntegrity, valid: true},
		{name: "valid token after invalid token", module: "widget", resolved: "https://registry.npmjs.org/widget/-/widget-1.2.3.tgz", integrity: "sha1-AAAA " + validIntegrity, valid: true},
		{name: "custom registry", module: "widget", resolved: "https://npm.example.test/widget/-/widget-1.2.3.tgz", integrity: validIntegrity},
		{name: "insecure registry", module: "widget", resolved: "http://registry.npmjs.org/widget/-/widget-1.2.3.tgz", integrity: validIntegrity},
		{name: "registry credentials", module: "widget", resolved: "https://token@registry.npmjs.org/widget/-/widget-1.2.3.tgz", integrity: validIntegrity},
		{name: "registry port", module: "widget", resolved: "https://registry.npmjs.org:443/widget/-/widget-1.2.3.tgz", integrity: validIntegrity},
		{name: "query", module: "widget", resolved: "https://registry.npmjs.org/widget/-/widget-1.2.3.tgz?token=secret", integrity: validIntegrity},
		{name: "wrong package path", module: "widget", resolved: "https://registry.npmjs.org/other/-/other-1.2.3.tgz", integrity: validIntegrity},
		{name: "missing integrity", module: "widget", resolved: "https://registry.npmjs.org/widget/-/widget-1.2.3.tgz"},
		{name: "invalid base64", module: "widget", resolved: "https://registry.npmjs.org/widget/-/widget-1.2.3.tgz", integrity: "sha256-not-base64!"},
		{name: "wrong digest length", module: "widget", resolved: "https://registry.npmjs.org/widget/-/widget-1.2.3.tgz", integrity: "sha256-YQ=="},
		{name: "unsupported digest", module: "widget", resolved: "https://registry.npmjs.org/widget/-/widget-1.2.3.tgz", integrity: "sha1-AAAAAAAAAAAAAAAAAAAAAAAAAAA="},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source, valid := dependency.NormalizeNPMRegistryEvidence(test.module, "1.2.3", test.resolved, test.integrity)
			if valid != test.valid || (valid && source != dependency.NPMRegistrySource) || (!valid && source != "") {
				t.Fatalf("dependency.NormalizeNPMRegistryEvidence() = %q, %v", source, valid)
			}
		})
	}
}

func TestQualifyExternalDependenciesRejectsUnsupportedNPMSourceKinds(t *testing.T) {
	constraints := []string{
		"file:../widget",
		"link:../widget",
		"workspace:*",
		"portal:../widget",
		"patch:widget@npm%3A1.2.3#./widget.patch",
		"catalog:default",
		"git+https://github.com/acme/widget.git#0123456789abcdef",
		"github:acme/widget#0123456789abcdef",
		"acme/widget#0123456789abcdef",
		"https://packages.example.test/widget-1.2.3.tgz",
		"../widget-1.2.3.tgz",
		"widget-1.2.3.tgz",
		`C:\\packages\\widget`,
	}
	for _, constraint := range constraints {
		t.Run(constraint, func(t *testing.T) {
			root := t.TempDir()
			mustWriteDependencyFile(t, filepath.Join(root, "package.json"), fmt.Sprintf(`{"dependencies":{"widget":%q}}`, constraint))
			mustWriteDependencyFile(t, filepath.Join(root, "package-lock.json"), `{"lockfileVersion":3,"packages":{"node_modules/widget":{"version":"1.2.3","integrity":"sha512-registry-looking"}}}`)
			mustWriteDependencyFile(t, filepath.Join(root, "main.ts"), "export {}")
			results := npmExternalDependencyResultsFor("widget")
			if err := qualifyLocalDependencies(results, root); err != nil {
				t.Fatal(err)
			}
			if reference := results[0].Related[0].External; reference.Module != "" || reference.Version != "" || reference.Integrity != "" {
				t.Fatalf("unsupported npm source %q was qualified: %#v", constraint, reference)
			}
		})
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
			if err := qualifyLocalDependencies(results, root); err != nil {
				t.Fatal(err)
			}
			reference := results[0].Related[0].External
			if reference.Module != "" || reference.Version != "" || reference.Integrity != "" {
				t.Fatalf("non-authoritative lock qualified dependency: %#v", reference)
			}
		})
	}
}

func npmExternalDependencyResults() []externalDependencyTestResult {
	return npmExternalDependencyResultsFor("@acme/widgets/subpath")
}

func npmExternalDependencyResultsFor(importPath string) []externalDependencyTestResult {
	return []externalDependencyTestResult{{Path: "main.ts", Language: "typescript", Related: []RelatedSymbol{{External: &ExternalReference{ID: "widget", Language: "typescript", ImportPath: importPath, Symbol: "Widget", Kind: "type"}}}}}
}

func TestQualifyExternalDependenciesFromCargoLock(t *testing.T) {
	root := t.TempDir()
	mustWriteDependencyFile(t, filepath.Join(root, "Cargo.toml"), "[package]\nname='consumer'\nversion='0.1.0'\n[dependencies]\nacme_widgets={ package='acme-widgets', version='1' }\n")
	mustWriteDependencyFile(t, filepath.Join(root, "Cargo.lock"), "version = 3\n[[package]]\nname = 'acme-widgets'\nversion = '1.4.0'\nsource = 'registry+https://github.com/rust-lang/crates.io-index'\nchecksum = 'cargo-sum'\n")
	mustWriteDependencyFile(t, filepath.Join(root, "main.rs"), "fn main() {}")
	results := []externalDependencyTestResult{{Path: "main.rs", Language: "rust", Related: []RelatedSymbol{{External: &ExternalReference{ID: "widget", Language: "rust", ImportPath: "acme_widgets::Widget", Symbol: "Widget", Kind: "type"}}}}}
	if err := qualifyLocalDependencies(results, root); err != nil {
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
	results := []externalDependencyTestResult{{Path: "Main.java", Language: "java", Related: []RelatedSymbol{{External: &ExternalReference{ID: "widget", Language: "java", ImportPath: "com.acme.widgets.Widget", Symbol: "Widget", Kind: "type"}}}}}
	if err := qualifyLocalDependencies(results, root); err != nil {
		t.Fatal(err)
	}
	reference := results[0].Related[0].External
	if reference.Module != "" || len(reference.Candidates) != 2 || reference.Candidates[0].Module != "com.acme:support" || reference.Candidates[1].Version != "2.3.0" {
		t.Fatalf("maven candidates = %#v", reference)
	}
	if got := externalDependencyReferences(results); len(got) != 1 || len(got[0].Candidates) != 2 {
		t.Fatalf("eligible references = %#v", got)
	}
}

func mustWriteDependencyFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
