package dependency

import (
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/mod/modfile"
)

type goModDependencyResolver struct{}

var _ Resolver = goModDependencyResolver{}

// NewGoModResolver returns the resolver for go.mod, go.sum, and go.work evidence.
func NewGoModResolver() Resolver { return goModDependencyResolver{} }

func (goModDependencyResolver) ID() string              { return "go-mod" }
func (goModDependencyResolver) Languages() []string     { return []string{"go"} }
func (goModDependencyResolver) ManifestNames() []string { return []string{"go.mod"} }

func (goModDependencyResolver) Resolve(manifestPath string) (Resolution, error) {
	content, err := os.ReadFile(manifestPath)
	if err != nil {
		return Resolution{}, err
	}
	parsed, err := modfile.Parse(manifestPath, content, nil)
	if err != nil {
		return Resolution{}, err
	}
	integrities, err := readGoDependencySums(filepath.Join(filepath.Dir(manifestPath), "go.sum"))
	if err != nil {
		return Resolution{}, err
	}
	dependencies := make([]goDependency, 0, len(parsed.Require))
	for _, required := range parsed.Require {
		dependency := goDependency{module: required.Mod.Path, sourceModule: required.Mod.Path, version: required.Mod.Version, integrity: integrities[required.Mod.Path+"@"+required.Mod.Version]}
		applyGoDependencyReplacements(&dependency, parsed.Replace)
		dependencies = append(dependencies, dependency)
	}
	if workPath, ok := nearestFile(filepath.Dir(manifestPath), "go.work"); ok {
		workContent, readErr := os.ReadFile(workPath)
		if readErr != nil {
			return Resolution{}, readErr
		}
		work, parseErr := modfile.ParseWork(workPath, workContent, nil)
		if parseErr != nil {
			return Resolution{}, parseErr
		}
		for index := range dependencies {
			applyGoDependencyReplacements(&dependencies[index], work.Replace)
		}
	}
	evidence := make([]Evidence, 0, len(dependencies))
	for _, dependency := range dependencies {
		if dependency.version != "" {
			evidence = append(evidence, Evidence{Ecosystem: "go", ImportName: dependency.module, Module: dependency.sourceModule, Version: dependency.version, Integrity: dependency.integrity})
		}
	}
	return Resolution{Applicable: true, Dependencies: evidence}, nil
}

func (goModDependencyResolver) Match(importPath string, dependencies []Evidence) Match {
	var matches []Evidence
	bestLength := 0
	for _, dependency := range dependencies {
		if importPath != dependency.ImportName && !strings.HasPrefix(importPath, dependency.ImportName+"/") {
			continue
		}
		if len(dependency.ImportName) > bestLength {
			matches, bestLength = matches[:0], len(dependency.ImportName)
		}
		if len(dependency.ImportName) == bestLength {
			matches = append(matches, dependency)
		}
	}
	sortEvidence(matches)
	return exactMatch(importPath, matches, func(match Evidence) string {
		return match.Module + strings.TrimPrefix(importPath, match.ImportName)
	})
}

func (goModDependencyResolver) DiscoverModule(manifestPath, relativeRoot string) (*ArtifactModule, error) {
	content, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, err
	}
	parsed, err := modfile.Parse(manifestPath, content, nil)
	if err != nil {
		return nil, err
	}
	if parsed.Module == nil {
		return nil, nil
	}
	return &ArtifactModule{Ecosystem: "go", Module: parsed.Module.Mod.Path, Root: relativeRoot}, nil
}

type goDependency struct {
	module, sourceModule, version, integrity string
}

func readGoDependencySums(path string) (map[string]string, error) {
	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	integrities := make(map[string]string)
	for _, line := range strings.Split(string(content), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 3 && !strings.HasSuffix(fields[1], "/go.mod") {
			integrities[fields[0]+"@"+fields[1]] = fields[2]
		}
	}
	return integrities, nil
}

func applyGoDependencyReplacements(dependency *goDependency, replacements []*modfile.Replace) {
	for _, replacement := range replacements {
		if replacement.Old.Path != dependency.module || replacement.Old.Version != "" && replacement.Old.Version != dependency.version {
			continue
		}
		dependency.integrity = ""
		if replacement.New.Version == "" {
			dependency.version = ""
			continue
		}
		dependency.sourceModule = replacement.New.Path
		dependency.version = replacement.New.Version
	}
}
