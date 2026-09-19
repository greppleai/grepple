package navigation

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/greppleai/grepple/api"
	toml "github.com/pelletier/go-toml/v2"
	"golang.org/x/mod/modfile"
)

// ArtifactModule identifies one publishable module found in a repository.
type ArtifactModule struct {
	Ecosystem string
	Module    string
	Version   string
	Root      string
}

type dependencyEvidence struct {
	ecosystem, importName, module, version, integrity string
}

type dependencyContext struct {
	language     string
	dependencies []dependencyEvidence
}

func dependencyManifestName(language string) string {
	switch language {
	case "go":
		return "go.mod"
	case "javascript", "typescript", "tsx":
		return "package.json"
	case "rust":
		return "Cargo.toml"
	case "java", "kotlin":
		return "pom.xml"
	default:
		return ""
	}
}

func readDependencyContext(language, path string) (dependencyContext, error) {
	context := dependencyContext{language: language}
	var dependencies []dependencyEvidence
	var err error
	switch language {
	case "go":
		var module goDependencyModule
		module, err = readGoDependencyModule(path)
		for _, dependency := range module.dependencies {
			if dependency.version != "" {
				dependencies = append(dependencies, dependencyEvidence{ecosystem: "go", importName: dependency.module, module: dependency.sourceModule, version: dependency.version, integrity: dependency.integrity})
			}
		}
	case "javascript", "typescript", "tsx":
		dependencies, err = readNPMDependencies(path)
	case "rust":
		dependencies, err = readCargoDependencies(path)
	case "java", "kotlin":
		dependencies, err = readMavenDependencies(path)
	}
	context.dependencies = dependencies
	return context, err
}

func qualifyExternalReference(reference *api.ExternalNavigationReference, context dependencyContext) {
	if reference.Language != context.language {
		return
	}
	matches := dependencyMatches(reference.ImportPath, context)
	if len(matches) == 1 && context.language != "java" && context.language != "kotlin" {
		match := matches[0]
		reference.Module, reference.Version, reference.Integrity = match.module, match.version, match.integrity
		reference.Package = reference.ImportPath
		if match.ecosystem == "go" {
			reference.Package = match.module + strings.TrimPrefix(reference.ImportPath, match.importName)
		}
		return
	}
	if len(matches) == 0 {
		return
	}
	reference.Candidates = make([]api.ExternalDependencyCandidate, 0, len(matches))
	for _, match := range matches {
		reference.Candidates = append(reference.Candidates, api.ExternalDependencyCandidate{Ecosystem: match.ecosystem, Module: match.module, Version: match.version, Integrity: match.integrity})
	}
}

func dependencyMatches(importPath string, context dependencyContext) []dependencyEvidence {
	var matches []dependencyEvidence
	switch context.language {
	case "go":
		matches = goDependencyMatches(importPath, context.dependencies)
	case "javascript", "typescript", "tsx":
		matches = namedDependencyMatches(npmPackageName(importPath), context.dependencies, false)
	case "rust":
		crate, _, _ := strings.Cut(importPath, "::")
		matches = namedDependencyMatches(crate, context.dependencies, true)
	case "java", "kotlin":
		matches = append(matches, context.dependencies...)
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].ecosystem != matches[j].ecosystem {
			return matches[i].ecosystem < matches[j].ecosystem
		}
		if matches[i].module != matches[j].module {
			return matches[i].module < matches[j].module
		}
		return matches[i].version < matches[j].version
	})
	return matches
}

func goDependencyMatches(importPath string, dependencies []dependencyEvidence) []dependencyEvidence {
	var matches []dependencyEvidence
	bestLength := 0
	for _, dependency := range dependencies {
		if importPath != dependency.importName && !strings.HasPrefix(importPath, dependency.importName+"/") {
			continue
		}
		if len(dependency.importName) > bestLength {
			matches, bestLength = matches[:0], len(dependency.importName)
		}
		if len(dependency.importName) == bestLength {
			matches = append(matches, dependency)
		}
	}
	return matches
}

func namedDependencyMatches(name string, dependencies []dependencyEvidence, normalizeCrate bool) []dependencyEvidence {
	var matches []dependencyEvidence
	if normalizeCrate {
		name = normalizedCrateName(name)
	}
	for _, dependency := range dependencies {
		candidate := dependency.importName
		if normalizeCrate {
			candidate = normalizedCrateName(candidate)
		}
		if candidate == name {
			matches = append(matches, dependency)
		}
	}
	return matches
}

func npmPackageName(importPath string) string {
	parts := strings.Split(importPath, "/")
	if strings.HasPrefix(importPath, "@") && len(parts) >= 2 {
		return parts[0] + "/" + parts[1]
	}
	return parts[0]
}

func normalizedCrateName(name string) string { return strings.ReplaceAll(name, "-", "_") }

type npmManifest struct {
	Name         string            `json:"name"`
	Version      string            `json:"version"`
	Dependencies map[string]string `json:"dependencies"`
}

type npmLockEntry struct {
	Version   string `json:"version"`
	Integrity string `json:"integrity"`
	Resolved  string `json:"resolved"`
	Link      bool   `json:"link"`
}

type npmLock struct {
	LockfileVersion int                     `json:"lockfileVersion"`
	Packages        map[string]npmLockEntry `json:"packages"`
	Dependencies    map[string]npmLockEntry `json:"dependencies"`
}

func readNPMDependencies(path string) ([]dependencyEvidence, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var manifest npmManifest
	if err := json.Unmarshal(content, &manifest); err != nil {
		return nil, err
	}
	lockContent, err := readNPMLockfile(filepath.Dir(path))
	if err != nil || lockContent == nil {
		return nil, err
	}
	// A lockfile is optional qualification evidence. Malformed or unknown schemas
	// must leave the original import unresolved rather than fail the search or guess.
	var lock npmLock
	if err := json.Unmarshal(lockContent, &lock); err != nil {
		return nil, nil
	}
	if lock.LockfileVersion < 1 || lock.LockfileVersion > 3 {
		return nil, nil
	}
	dependencies := make([]dependencyEvidence, 0, len(manifest.Dependencies))
	for name := range manifest.Dependencies {
		entry, ok := npmLockedDirectDependency(lock, name)
		if !ok {
			continue
		}
		version, integrity, resolved, linked := entry.Version, entry.Integrity, entry.Resolved, entry.Link
		if version == "" || linked || strings.HasPrefix(resolved, "file:") || strings.HasPrefix(resolved, "link:") {
			continue
		}
		dependencies = append(dependencies, dependencyEvidence{ecosystem: "npm", importName: name, module: name, version: version, integrity: integrity})
	}
	return dependencies, nil
}

func readNPMLockfile(directory string) ([]byte, error) {
	// npm-shrinkwrap.json is publishable and takes precedence over package-lock.json.
	// Once present, even an invalid shrinkwrap remains the selected lockfile: falling
	// back would qualify a dependency graph that npm itself does not select.
	for _, name := range []string{"npm-shrinkwrap.json", "package-lock.json"} {
		content, err := os.ReadFile(filepath.Join(directory, name))
		if err == nil {
			return content, nil
		}
		if !os.IsNotExist(err) {
			return nil, err
		}
	}
	return nil, nil
}

func npmLockedDirectDependency(lock npmLock, name string) (npmLockEntry, bool) {
	if lock.LockfileVersion == 1 {
		entry, ok := lock.Dependencies[name]
		return entry, ok
	}
	entry, ok := lock.Packages["node_modules/"+name]
	if ok || lock.LockfileVersion == 3 {
		return entry, ok
	}
	entry, ok = lock.Dependencies[name]
	return entry, ok
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

func readCargoDependencies(path string) ([]dependencyEvidence, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var manifest cargoManifest
	if err := toml.Unmarshal(content, &manifest); err != nil {
		return nil, err
	}
	lockContent, err := os.ReadFile(filepath.Join(filepath.Dir(path), "Cargo.lock"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var lock cargoLock
	if err := toml.Unmarshal(lockContent, &lock); err != nil {
		return nil, err
	}
	var dependencies []dependencyEvidence
	for alias, raw := range manifest.Dependencies {
		if dependency, ok := exactCargoDependency(alias, raw, lock.Package); ok {
			dependencies = append(dependencies, dependency)
		}
	}
	return dependencies, nil
}

func exactCargoDependency(alias string, raw any, lockedPackages []cargoLockedPackage) (dependencyEvidence, bool) {
	packageName, remote := cargoDependencyIdentity(alias, raw)
	if !remote {
		return dependencyEvidence{}, false
	}
	var matches []cargoLockedPackage
	for _, locked := range lockedPackages {
		if locked.Name == packageName && locked.Version != "" && (locked.Source == "" || strings.HasPrefix(locked.Source, "registry+")) {
			matches = append(matches, locked)
		}
	}
	if len(matches) != 1 {
		return dependencyEvidence{}, false
	}
	return dependencyEvidence{ecosystem: "cargo", importName: alias, module: packageName, version: matches[0].Version, integrity: matches[0].Checksum}, true
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

type mavenProject struct {
	XMLName    xml.Name `xml:"project"`
	GroupID    string   `xml:"groupId"`
	ArtifactID string   `xml:"artifactId"`
	Version    string   `xml:"version"`
	Parent     struct {
		GroupID string `xml:"groupId"`
		Version string `xml:"version"`
	} `xml:"parent"`
	Properties   []xmlProperty `xml:"properties>*"`
	Dependencies []struct {
		GroupID    string `xml:"groupId"`
		ArtifactID string `xml:"artifactId"`
		Version    string `xml:"version"`
		Scope      string `xml:"scope"`
	} `xml:"dependencies>dependency"`
}

type xmlProperty struct {
	XMLName xml.Name
	Value   string `xml:",chardata"`
}

func parseMavenProject(path string) (mavenProject, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return mavenProject{}, err
	}
	var project mavenProject
	if err := xml.Unmarshal(content, &project); err != nil {
		return mavenProject{}, err
	}
	return project, nil
}

func readMavenDependencies(path string) ([]dependencyEvidence, error) {
	project, err := parseMavenProject(path)
	if err != nil {
		return nil, err
	}
	properties := map[string]string{"project.version": project.Version, "pom.version": project.Version}
	for _, property := range project.Properties {
		properties[property.XMLName.Local] = strings.TrimSpace(property.Value)
	}
	var dependencies []dependencyEvidence
	for _, dependency := range project.Dependencies {
		if dependency.Scope == "test" || dependency.Scope == "provided" {
			continue
		}
		version := resolveMavenValue(strings.TrimSpace(dependency.Version), properties)
		if version == "" || strings.Contains(version, "${") {
			continue
		}
		module := strings.TrimSpace(dependency.GroupID) + ":" + strings.TrimSpace(dependency.ArtifactID)
		dependencies = append(dependencies, dependencyEvidence{ecosystem: "maven", importName: module, module: module, version: version})
	}
	return dependencies, nil
}

func resolveMavenValue(value string, properties map[string]string) string {
	if strings.HasPrefix(value, "${") && strings.HasSuffix(value, "}") {
		return properties[strings.TrimSuffix(strings.TrimPrefix(value, "${"), "}")]
	}
	return value
}

// DiscoverArtifactModules returns publishable module identities declared by repository manifests.
func DiscoverArtifactModules(root string) ([]ArtifactModule, error) {
	var modules []ArtifactModule
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != root && artifactDirectoryIgnored(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		module, err := discoverArtifactModule(root, path, entry.Name())
		if err != nil {
			return err
		}
		if module != nil {
			modules = append(modules, *module)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("discover dependency modules: %w", err)
	}
	sort.Slice(modules, func(i, j int) bool { return artifactModuleLess(modules[i], modules[j]) })
	return modules, nil
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

func artifactDirectoryIgnored(name string) bool {
	switch name {
	case ".git", ".grepple", "node_modules", "target", "vendor":
		return true
	default:
		return false
	}
}

func discoverArtifactModule(root, path, name string) (*ArtifactModule, error) {
	relativeRoot, _ := filepath.Rel(root, filepath.Dir(path))
	switch name {
	case "go.mod":
		return discoverGoArtifactModule(path, relativeRoot)
	case "package.json":
		return discoverNPMArtifactModule(path, relativeRoot)
	case "Cargo.toml":
		return discoverCargoArtifactModule(path, relativeRoot)
	case "pom.xml":
		return discoverMavenArtifactModule(path, relativeRoot)
	default:
		return nil, nil
	}
}

func discoverGoArtifactModule(path, root string) (*ArtifactModule, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	parsed, err := modfile.Parse(path, content, nil)
	if err != nil {
		return nil, err
	}
	if parsed.Module == nil {
		return nil, nil
	}
	return &ArtifactModule{Ecosystem: "go", Module: parsed.Module.Mod.Path, Root: root}, nil
}

func discoverNPMArtifactModule(path, root string) (*ArtifactModule, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var manifest npmManifest
	if err := json.Unmarshal(content, &manifest); err != nil {
		return nil, err
	}
	if manifest.Name == "" || manifest.Version == "" {
		return nil, nil
	}
	return &ArtifactModule{Ecosystem: "npm", Module: manifest.Name, Version: manifest.Version, Root: root}, nil
}

func discoverCargoArtifactModule(path, root string) (*ArtifactModule, error) {
	content, err := os.ReadFile(path)
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
	return &ArtifactModule{Ecosystem: "cargo", Module: manifest.Package.Name, Version: manifest.Package.Version, Root: root}, nil
}

func discoverMavenArtifactModule(path, root string) (*ArtifactModule, error) {
	project, err := parseMavenProject(path)
	if err != nil {
		return nil, err
	}
	group, version := project.GroupID, project.Version
	if group == "" {
		group = project.Parent.GroupID
	}
	if version == "" {
		version = project.Parent.Version
	}
	if group == "" || project.ArtifactID == "" || version == "" {
		return nil, nil
	}
	return &ArtifactModule{Ecosystem: "maven", Module: group + ":" + project.ArtifactID, Version: version, Root: root}, nil
}
