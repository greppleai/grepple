package dependency

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultRegistryProvidesManagerResolvers(t *testing.T) {
	registry := DefaultRegistry()
	tests := []struct {
		language string
		manifest string
		wantID   string
	}{
		{language: "go", manifest: "go.mod", wantID: "go-mod"},
		{language: "typescript", manifest: "package.json", wantID: "npm"},
		{language: "rust", manifest: "Cargo.toml", wantID: "cargo"},
		{language: "kotlin", manifest: "pom.xml", wantID: "maven"},
		{language: "php", manifest: "composer.json", wantID: "composer"},
	}
	for _, test := range tests {
		t.Run(test.wantID, func(t *testing.T) {
			directory := t.TempDir()
			if err := os.WriteFile(filepath.Join(directory, test.manifest), []byte("{}"), 0o600); err != nil {
				t.Fatal(err)
			}
			projects := registry.Projects(test.language, directory)
			if len(projects) != 1 || projects[0].Resolver.ID() != test.wantID {
				t.Fatalf("projects = %#v", projects)
			}
		})
	}
}

func TestNPMResolverOnlyClaimsNPMProjects(t *testing.T) {
	resolver := NewNPMResolver()
	for _, test := range []struct {
		name       string
		manager    string
		applicable bool
	}{
		{name: "implicit npm", applicable: true},
		{name: "exact npm", manager: "npm@10.8.2", applicable: true},
		{name: "malformed npm remains authoritative", manager: "npm@latest", applicable: true},
		{name: "malformed spaced npm remains authoritative", manager: " npm@10.8.2", applicable: true},
		{name: "pnpm", manager: "pnpm@9.1.0", applicable: false},
		{name: "bun", manager: "bun@1.1.0", applicable: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			manifest := `{"packageManager":"` + test.manager + `"}`
			path := filepath.Join(directory, "package.json")
			if err := os.WriteFile(path, []byte(manifest), 0o600); err != nil {
				t.Fatal(err)
			}
			resolution, err := resolver.Resolve(path)
			if err != nil {
				t.Fatal(err)
			}
			if resolution.Applicable != test.applicable {
				t.Fatalf("applicable = %v, want %v", resolution.Applicable, test.applicable)
			}
		})
	}
}

func TestRegistryAcceptsResolverWithoutCoordinatorChanges(t *testing.T) {
	directory := t.TempDir()
	manifestPath := filepath.Join(directory, "deps.lock")
	if err := os.WriteFile(manifestPath, []byte("locked"), 0o600); err != nil {
		t.Fatal(err)
	}
	registry := NewRegistry(fakeDependencyResolver{})
	projects := registry.Projects("custom", directory)
	if len(projects) != 1 || projects[0].Key() != "custom-manager\x00"+manifestPath {
		t.Fatalf("projects = %#v", projects)
	}
	resolution, err := projects[0].Resolve()
	if err != nil {
		t.Fatal(err)
	}
	match := projects[0].Resolver.Match("example/module/subpath", resolution.Dependencies)
	if match.Exact == nil || match.Exact.Version != "1.2.3" {
		t.Fatalf("match = %#v", match)
	}
}

type fakeDependencyResolver struct{}

func (fakeDependencyResolver) ID() string              { return "custom-manager" }
func (fakeDependencyResolver) Languages() []string     { return []string{"custom"} }
func (fakeDependencyResolver) ManifestNames() []string { return []string{"deps.lock"} }
func (fakeDependencyResolver) Resolve(string) (Resolution, error) {
	return Resolution{Applicable: true, Dependencies: []Evidence{{Ecosystem: "custom", ImportName: "example/module", Module: "example/module", Version: "1.2.3"}}}, nil
}
func (fakeDependencyResolver) Match(importPath string, dependencies []Evidence) Match {
	return exactMatch(importPath, dependencies, nil)
}
func (fakeDependencyResolver) DiscoverModule(string, string) (*ArtifactModule, error) {
	return nil, nil
}
