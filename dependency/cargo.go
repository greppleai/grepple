// Package dependency provides package-manager-specific resolution behind one registry.
package dependency

import (
	"os"
	"path/filepath"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
)

type cargoDependencyResolver struct{}

var _ Resolver = cargoDependencyResolver{}

// NewCargoResolver returns the resolver for Cargo manifests and lockfiles.
func NewCargoResolver() Resolver { return cargoDependencyResolver{} }

func (cargoDependencyResolver) ID() string              { return "cargo" }
func (cargoDependencyResolver) Languages() []string     { return []string{"rust"} }
func (cargoDependencyResolver) ManifestNames() []string { return []string{"Cargo.toml"} }

func (cargoDependencyResolver) Resolve(manifestPath string) (Resolution, error) {
	content, err := os.ReadFile(manifestPath)
	if err != nil {
		return Resolution{}, err
	}
	var manifest cargoManifest
	if err := toml.Unmarshal(content, &manifest); err != nil {
		return Resolution{}, err
	}
	lockContent, err := os.ReadFile(filepath.Join(filepath.Dir(manifestPath), "Cargo.lock"))
	if os.IsNotExist(err) {
		return Resolution{Applicable: true}, nil
	}
	if err != nil {
		return Resolution{}, err
	}
	var lock cargoLock
	if err := toml.Unmarshal(lockContent, &lock); err != nil {
		return Resolution{}, err
	}
	var dependencies []Evidence
	for alias, raw := range manifest.Dependencies {
		if dependency, ok := exactCargoDependency(alias, raw, lock.Package); ok {
			dependencies = append(dependencies, dependency)
		}
	}
	return Resolution{Applicable: true, Dependencies: dependencies}, nil
}

func (cargoDependencyResolver) Match(importPath string, dependencies []Evidence) Match {
	crate, _, _ := strings.Cut(importPath, "::")
	matches := namedDependencyMatches(crate, dependencies, func(name string) string {
		return strings.ReplaceAll(name, "-", "_")
	})
	return exactMatch(importPath, matches, nil)
}

func (cargoDependencyResolver) DiscoverModule(manifestPath, relativeRoot string) (*ArtifactModule, error) {
	content, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, err
	}
	var manifest cargoManifest
	if err := toml.Unmarshal(content, &manifest); err != nil {
		return nil, err
	}
	if manifest.Package.Name == "" || manifest.Package.Version == "" {
		return nil, nil
	}
	return &ArtifactModule{Ecosystem: "cargo", Module: manifest.Package.Name, Version: manifest.Package.Version, Root: relativeRoot}, nil
}

type cargoManifest struct {
	Package struct {
		Name    string `toml:"name"`
		Version string `toml:"version"`
	} `toml:"package"`
	Dependencies map[string]any `toml:"dependencies"`
}

type cargoLock struct {
	Package []cargoLockedPackage `toml:"package"`
}

type cargoLockedPackage struct {
	Name     string `toml:"name"`
	Version  string `toml:"version"`
	Source   string `toml:"source"`
	Checksum string `toml:"checksum"`
}

func exactCargoDependency(alias string, raw any, lockedPackages []cargoLockedPackage) (Evidence, bool) {
	packageName, remote := cargoDependencyIdentity(alias, raw)
	if !remote {
		return Evidence{}, false
	}
	var matches []cargoLockedPackage
	for _, locked := range lockedPackages {
		if locked.Name == packageName && locked.Version != "" && (locked.Source == "" || strings.HasPrefix(locked.Source, "registry+")) {
			matches = append(matches, locked)
		}
	}
	if len(matches) != 1 {
		return Evidence{}, false
	}
	return Evidence{Ecosystem: "cargo", ImportName: alias, Module: packageName, Version: matches[0].Version, Integrity: matches[0].Checksum}, true
}

func cargoDependencyIdentity(alias string, raw any) (string, bool) {
	table, ok := raw.(map[string]any)
	if !ok {
		return alias, true
	}
	if _, hasPath := table["path"]; hasPath {
		return "", false
	}
	if _, hasGit := table["git"]; hasGit {
		return "", false
	}
	if packageName, ok := table["package"].(string); ok {
		return packageName, true
	}
	return alias, true
}
