package dependency

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type composerDependencyResolver struct{}

var _ Resolver = composerDependencyResolver{}

// NewComposerResolver resolves locked Composer packages by their declared autoload namespaces.
func NewComposerResolver() Resolver { return composerDependencyResolver{} }

func (composerDependencyResolver) ID() string              { return "composer" }
func (composerDependencyResolver) Languages() []string     { return []string{"php"} }
func (composerDependencyResolver) ManifestNames() []string { return []string{"composer.json"} }

type composerManifest struct {
	Name        string            `json:"name"`
	Version     string            `json:"version"`
	Require     map[string]string `json:"require"`
	RequireDev  map[string]string `json:"require-dev"`
	Autoload    composerAutoload  `json:"autoload"`
	AutoloadDev composerAutoload  `json:"autoload-dev"`
}

type composerAutoload struct {
	PSR4 map[string]json.RawMessage `json:"psr-4"`
	PSR0 map[string]json.RawMessage `json:"psr-0"`
}

type composerLock struct {
	Packages    []composerLockedPackage `json:"packages"`
	PackagesDev []composerLockedPackage `json:"packages-dev"`
}

type composerLockedPackage struct {
	Name     string           `json:"name"`
	Version  string           `json:"version"`
	Autoload composerAutoload `json:"autoload"`
	Dist     struct {
		Type string `json:"type"`
	} `json:"dist"`
}

// Composer records namespace ownership in package autoload rules, not in the
// package name. ImportName carries a namespace prefix (including its trailing
// separator); Match uses the longest prefix rather than treating a vendor name
// as a PHP namespace.
func (composerDependencyResolver) Resolve(manifestPath string) (Resolution, error) {
	content, err := os.ReadFile(manifestPath)
	if err != nil {
		return Resolution{}, err
	}
	var manifest composerManifest
	if err := json.Unmarshal(content, &manifest); err != nil {
		return Resolution{}, err
	}
	content, err = os.ReadFile(filepath.Join(filepath.Dir(manifestPath), "composer.lock"))
	if os.IsNotExist(err) {
		return Resolution{Applicable: true}, nil
	}
	if err != nil {
		return Resolution{}, err
	}
	var lock composerLock
	if err := json.Unmarshal(content, &lock); err != nil {
		return Resolution{}, err
	}
	var evidence []Evidence
	for _, group := range []struct {
		packages []composerLockedPackage
		required map[string]string
	}{{lock.Packages, manifest.Require}, {lock.PackagesDev, manifest.RequireDev}} {
		for _, pkg := range group.packages {
			if _, declared := group.required[pkg.Name]; !declared || pkg.Version == "" || pkg.Dist.Type == "path" || strings.HasPrefix(pkg.Version, "dev-") || strings.HasSuffix(pkg.Version, "-dev") {
				continue
			}
			for _, rules := range []map[string]json.RawMessage{pkg.Autoload.PSR4, pkg.Autoload.PSR0} {
				for prefix := range rules {
					if prefix == "" { // catch-all rules cannot establish package ownership
						continue
					}
					evidence = append(evidence, Evidence{Ecosystem: "composer", ImportName: strings.TrimPrefix(prefix, "\\"), Module: pkg.Name, Version: pkg.Version})
				}
			}
		}
	}
	sortEvidence(evidence)
	return Resolution{Applicable: true, Dependencies: evidence}, nil
}

func (composerDependencyResolver) Match(importPath string, dependencies []Evidence) Match {
	importPath = strings.TrimPrefix(importPath, "\\")
	longest := 0
	var matches []Evidence
	for _, candidate := range dependencies {
		prefix := candidate.ImportName
		if !strings.HasSuffix(prefix, "\\") {
			prefix += "\\"
		}
		if !strings.HasPrefix(importPath, prefix) {
			continue
		}
		if len(prefix) > longest {
			longest, matches = len(prefix), nil
		}
		if len(prefix) == longest {
			matches = append(matches, candidate)
		}
	}
	sortEvidence(matches)
	// Multiple namespaces of one package may be declared in both PSR maps;
	// identical identity is not an ambiguity.
	if len(matches) > 1 {
		unique := matches[:1]
		for _, candidate := range matches[1:] {
			if candidate.Module != unique[len(unique)-1].Module || candidate.Version != unique[len(unique)-1].Version {
				unique = append(unique, candidate)
			}
		}
		matches = unique
	}
	return exactMatch(importPath, matches, nil)
}

func (composerDependencyResolver) DiscoverModule(manifestPath, relativeRoot string) (*ArtifactModule, error) {
	content, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, err
	}
	var manifest composerManifest
	if err := json.Unmarshal(content, &manifest); err != nil {
		return nil, err
	}
	if manifest.Name == "" || manifest.Version == "" {
		return nil, nil
	}
	return &ArtifactModule{Ecosystem: "composer", Module: manifest.Name, Version: manifest.Version, Root: relativeRoot}, nil
}

// ComposerAutoloadRules returns only PSR-4 paths; other autoload mechanisms
// require runtime Composer behavior and are intentionally not guessed.
func ComposerAutoloadRules(manifestPath string) (map[string][]string, error) {
	content, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, err
	}
	var manifest composerManifest
	if err := json.Unmarshal(content, &manifest); err != nil {
		return nil, err
	}
	rules := make(map[string][]string)
	for _, group := range []map[string]json.RawMessage{manifest.Autoload.PSR4, manifest.AutoloadDev.PSR4} {
		for prefix, raw := range group {
			var paths []string
			var path string
			if json.Unmarshal(raw, &path) == nil {
				paths = []string{path}
			} else if json.Unmarshal(raw, &paths) != nil {
				continue
			}
			rules[prefix] = append(rules[prefix], paths...)
		}
	}
	return rules, nil
}
