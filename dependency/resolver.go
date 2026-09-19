package dependency

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Evidence is exact dependency identity recovered from a manager-owned manifest
// and resolved lock state.
type Evidence struct {
	Ecosystem  string
	ImportName string
	Module     string
	Version    string
	Integrity  string
	Source     string
}

// Match describes how a resolver maps one source import to dependency evidence.
// Exact is nil when package ownership remains ambiguous even with one candidate.
type Match struct {
	Exact      *Evidence
	Package    string
	Candidates []Evidence
}

// Resolution is the result of interpreting one resolver's manifest. Applicable
// distinguishes an authoritative but unresolved project from another manager's project.
type Resolution struct {
	Applicable   bool
	Dependencies []Evidence
}

// ArtifactModule identifies one publishable module found in a repository.
type ArtifactModule struct {
	Ecosystem string
	Module    string
	Version   string
	Root      string
}

// Resolver owns one dependency manager's project selection, lock semantics,
// import matching, source identity, and publishable-module discovery.
type Resolver interface {
	ID() string
	Languages() []string
	ManifestNames() []string
	Resolve(manifestPath string) (Resolution, error)
	Match(importPath string, dependencies []Evidence) Match
	DiscoverModule(manifestPath, relativeRoot string) (*ArtifactModule, error)
}

// Project binds a resolver to the nearest manifest it can interpret.
type Project struct {
	Resolver     Resolver
	ManifestPath string
}

// Key is stable for the resolver and manifest selected for a source file.
func (p Project) Key() string { return p.Resolver.ID() + "\x00" + p.ManifestPath }

// Resolve interprets the project's exact manager state.
func (p Project) Resolve() (Resolution, error) { return p.Resolver.Resolve(p.ManifestPath) }

// Registry selects resolvers without embedding manager-specific branches in callers.
type Registry struct {
	resolvers []Resolver
}

// NewRegistry builds an ordered resolver registry. Order only breaks ties between
// resolvers that explicitly report the same project as applicable.
func NewRegistry(resolvers ...Resolver) Registry {
	return Registry{resolvers: append([]Resolver(nil), resolvers...)}
}

// DefaultRegistry returns all dependency managers currently implemented.
func DefaultRegistry() Registry {
	return NewRegistry(NewGoModResolver(), NewNPMResolver(), NewCargoResolver(), NewMavenResolver())
}

// Projects returns resolver/manifest candidates nearest to directory.
func (r Registry) Projects(language, directory string) []Project {
	var projects []Project
	for _, resolver := range r.resolvers {
		if !containsString(resolver.Languages(), language) {
			continue
		}
		for _, name := range resolver.ManifestNames() {
			if manifestPath, ok := nearestFile(directory, name); ok {
				projects = append(projects, Project{Resolver: resolver, ManifestPath: manifestPath})
				break
			}
		}
	}
	return projects
}

// DiscoverArtifactModules returns publishable identities declared by repository manifests.
func (r Registry) DiscoverArtifactModules(root string) ([]ArtifactModule, error) {
	var modules []ArtifactModule
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		return r.discoverArtifactAt(root, path, entry, walkErr, &modules)
	})
	if err != nil {
		return nil, fmt.Errorf("discover dependency modules: %w", err)
	}
	sort.Slice(modules, func(i, j int) bool { return artifactModuleLess(modules[i], modules[j]) })
	return compactModules(modules), nil
}

func (r Registry) discoverArtifactAt(root, path string, entry fs.DirEntry, walkErr error, modules *[]ArtifactModule) error {
	if walkErr != nil {
		return walkErr
	}
	if entry.IsDir() {
		if path != root && artifactDirectoryIgnored(entry.Name()) {
			return filepath.SkipDir
		}
		return nil
	}
	relativeRoot, _ := filepath.Rel(root, filepath.Dir(path))
	for _, resolver := range r.resolvers {
		if !containsString(resolver.ManifestNames(), entry.Name()) {
			continue
		}
		module, err := resolver.DiscoverModule(path, relativeRoot)
		if err != nil {
			return err
		}
		if module != nil {
			*modules = append(*modules, *module)
		}
	}
	return nil
}

func artifactModuleLess(left, right ArtifactModule) bool {
	if left.Ecosystem != right.Ecosystem {
		return left.Ecosystem < right.Ecosystem
	}
	if left.Module != right.Module {
		return left.Module < right.Module
	}
	return left.Root < right.Root
}

func nearestFile(directory, name string) (string, bool) {
	for {
		candidate := filepath.Join(directory, name)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, true
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return "", false
		}
		directory = parent
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func namedDependencyMatches(name string, dependencies []Evidence, normalize func(string) string) []Evidence {
	if normalize != nil {
		name = normalize(name)
	}
	var matches []Evidence
	for _, candidate := range dependencies {
		candidateName := candidate.ImportName
		if normalize != nil {
			candidateName = normalize(candidateName)
		}
		if candidateName == name {
			matches = append(matches, candidate)
		}
	}
	sortEvidence(matches)
	return matches
}

func sortEvidence(evidence []Evidence) {
	sort.Slice(evidence, func(i, j int) bool {
		left, right := evidence[i], evidence[j]
		if left.Ecosystem != right.Ecosystem {
			return left.Ecosystem < right.Ecosystem
		}
		if left.Module != right.Module {
			return left.Module < right.Module
		}
		if left.Version != right.Version {
			return left.Version < right.Version
		}
		return left.Source < right.Source
	})
}

func exactMatch(importPath string, matches []Evidence, packagePath func(Evidence) string) Match {
	if len(matches) != 1 {
		return Match{Candidates: matches}
	}
	match := matches[0]
	packageName := importPath
	if packagePath != nil {
		packageName = packagePath(match)
	}
	return Match{Exact: &match, Package: packageName}
}

func compactModules(modules []ArtifactModule) []ArtifactModule {
	if len(modules) < 2 {
		return modules
	}
	result := modules[:1]
	for _, module := range modules[1:] {
		previous := result[len(result)-1]
		if module != previous {
			result = append(result, module)
		}
	}
	return result
}

func artifactDirectoryIgnored(name string) bool {
	switch name {
	case ".git", ".grepple", "node_modules", "target", "vendor":
		return true
	default:
		return false
	}
}

func npmPackageName(importPath string) string {
	parts := strings.Split(importPath, "/")
	if strings.HasPrefix(importPath, "@") && len(parts) >= 2 {
		return parts[0] + "/" + parts[1]
	}
	return parts[0]
}
