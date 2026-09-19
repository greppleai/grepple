package navigation

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/greppleai/grepple/api"
	toml "github.com/pelletier/go-toml/v2"
	"golang.org/x/mod/modfile"
	"golang.org/x/mod/semver"
)

// ArtifactModule identifies one publishable module found in a repository.
type ArtifactModule struct {
	Ecosystem string
	Module    string
	Version   string
	Root      string
}

type dependencyEvidence struct {
	ecosystem, importName, module, version, integrity, source string
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
		reference.Module, reference.Version, reference.Integrity, reference.Source = match.module, match.version, match.integrity, match.source
		reference.Package = reference.ImportPath
		if match.ecosystem == "go" || match.ecosystem == "npm" {
			reference.Package = match.module + strings.TrimPrefix(reference.ImportPath, match.importName)
		}
		return
	}
	if len(matches) == 0 {
		return
	}
	reference.Candidates = make([]api.ExternalDependencyCandidate, 0, len(matches))
	for _, match := range matches {
		reference.Candidates = append(reference.Candidates, api.ExternalDependencyCandidate{Ecosystem: match.ecosystem, Module: match.module, Version: match.version, Integrity: match.integrity, Source: match.source})
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
		if matches[i].version != matches[j].version {
			return matches[i].version < matches[j].version
		}
		return matches[i].source < matches[j].source
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
	Name           string            `json:"name"`
	Version        string            `json:"version"`
	PackageManager string            `json:"packageManager"`
	Dependencies   map[string]string `json:"dependencies"`
}

func npmPackageManagerAllowsLockfile(specifier string) bool {
	if specifier == "" {
		return true
	}
	if strings.TrimSpace(specifier) != specifier || !strings.HasPrefix(specifier, "npm@") {
		return false
	}
	versionSpec := strings.TrimPrefix(specifier, "npm@")
	version, integrity, hasIntegrity := strings.Cut(versionSpec, "+")
	if !semver.IsValid("v" + version) {
		return false
	}
	coreVersion, _, _ := strings.Cut(version, "-")
	if len(strings.Split(coreVersion, ".")) != 3 {
		return false
	}
	if !hasIntegrity {
		return true
	}
	algorithm, digest, ok := strings.Cut(integrity, ".")
	if !ok {
		return false
	}
	expectedBytes := map[string]int{"sha224": 28, "sha256": 32, "sha384": 48, "sha512": 64}[algorithm]
	decoded, err := hex.DecodeString(digest)
	return err == nil && expectedBytes > 0 && len(decoded) == expectedBytes
}

type npmLockEntry struct {
	Name      string `json:"name"`
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
	if !npmPackageManagerAllowsLockfile(manifest.PackageManager) {
		return nil, nil
	}
	lock, authoritative, err := readAuthoritativeNPMLock(filepath.Dir(path))
	if err != nil || !authoritative {
		return nil, err
	}
	dependencies := make([]dependencyEvidence, 0, len(manifest.Dependencies))
	for name, constraint := range manifest.Dependencies {
		evidence, ok := npmLockedDependencyEvidence(lock, name, constraint)
		if ok {
			dependencies = append(dependencies, evidence)
		}
	}
	return dependencies, nil
}

func readAuthoritativeNPMLock(directory string) (npmLock, bool, error) {
	content, err := readNPMLockfile(directory)
	if err != nil || content == nil {
		return npmLock{}, false, err
	}
	// A lockfile is optional qualification evidence. Malformed or unknown schemas
	// must leave the original import unresolved rather than fail the search or guess.
	var lock npmLock
	if err := json.Unmarshal(content, &lock); err != nil {
		return npmLock{}, false, nil
	}
	if lock.LockfileVersion < 1 || lock.LockfileVersion > 3 {
		return npmLock{}, false, nil
	}
	return lock, true, nil
}

func npmLockedDependencyEvidence(lock npmLock, name, constraint string) (dependencyEvidence, bool) {
	if npmUnsupportedDependencySource(constraint) {
		return dependencyEvidence{}, false
	}
	entry, ok := npmLockedDirectDependency(lock, name)
	if !ok {
		return dependencyEvidence{}, false
	}
	module, version, ok := npmLockedDependencyIdentity(name, constraint, entry)
	if !ok || entry.Link {
		return dependencyEvidence{}, false
	}
	source, ok := NormalizeNPMRegistryEvidence(module, version, entry.Resolved, entry.Integrity)
	if !ok {
		return dependencyEvidence{}, false
	}
	return dependencyEvidence{ecosystem: "npm", importName: name, module: module, version: version, integrity: entry.Integrity, source: source}, true
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

// NPMRegistrySource is the only npm registry identity currently accepted as exact evidence.
const NPMRegistrySource = "https://registry.npmjs.org"

// NormalizeNPMRegistryEvidence validates canonical public-registry archive evidence.
// It validates the URL and SRI digest shape, not the archive bytes themselves.
func NormalizeNPMRegistryEvidence(module, version, resolved, integrity string) (string, bool) {
	archive, err := url.Parse(resolved)
	if err != nil || archive.Scheme != "https" || archive.Host != "registry.npmjs.org" || archive.User != nil || archive.RawQuery != "" || archive.Fragment != "" {
		return "", false
	}
	name := path.Base(module)
	expectedPath := "/" + module + "/-/" + name + "-" + version + ".tgz"
	if module == "" || version == "" || archive.Path != expectedPath || !ValidNPMIntegrity(integrity) {
		return "", false
	}
	return NPMRegistrySource, true
}

// ValidNPMIntegrity reports whether at least one SRI token has a supported
// algorithm, decodable base64 digest, and the algorithm's exact digest length.
func ValidNPMIntegrity(integrity string) bool {
	digestLengths := map[string]int{"sha256": 32, "sha384": 48, "sha512": 64}
	for _, token := range strings.Fields(integrity) {
		token, _, _ = strings.Cut(token, "?")
		algorithm, encoded, ok := strings.Cut(token, "-")
		expected := digestLengths[algorithm]
		if !ok || expected == 0 {
			continue
		}
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			decoded, err = base64.RawStdEncoding.DecodeString(encoded)
		}
		if err == nil && len(decoded) == expected {
			return true
		}
	}
	return false
}

func npmLockedDependencyIdentity(importName, constraint string, entry npmLockEntry) (string, string, bool) {
	aliasName, _, alias := npmAliasSpecifier(constraint)
	if alias {
		if entry.Name != "" {
			return aliasName, entry.Version, entry.Name == aliasName && entry.Version != "" && !strings.HasPrefix(entry.Version, "npm:")
		}
		lockedName, lockedVersion, lockedAlias := npmAliasSpecifier(entry.Version)
		return lockedName, lockedVersion, lockedAlias && lockedName == aliasName && lockedVersion != ""
	}
	if entry.Name != "" && entry.Name != importName {
		return "", "", false
	}
	if _, _, lockedAlias := npmAliasSpecifier(entry.Version); lockedAlias || entry.Version == "" {
		return "", "", false
	}
	return importName, entry.Version, true
}

func npmAliasSpecifier(specifier string) (string, string, bool) {
	if !strings.HasPrefix(specifier, "npm:") {
		return "", "", false
	}
	value := strings.TrimPrefix(specifier, "npm:")
	separator := strings.Index(value, "@")
	if strings.HasPrefix(value, "@") {
		slash := strings.Index(value, "/")
		if slash < 2 {
			return "", "", false
		}
		separator = strings.Index(value[slash+1:], "@")
		if separator >= 0 {
			separator += slash + 1
		}
	}
	if separator < 0 {
		return value, "", value != ""
	}
	name, version := value[:separator], value[separator+1:]
	return name, version, name != ""
}

func npmUnsupportedDependencySource(specifier string) bool {
	value := strings.ToLower(strings.TrimSpace(specifier))
	for _, prefix := range []string{
		"file:", "link:", "workspace:", "portal:", "patch:", "catalog:",
		"git:", "git+", "github:", "gitlab:", "bitbucket:",
		"http:", "https:",
	} {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	if strings.HasPrefix(value, "./") || strings.HasPrefix(value, "../") || strings.HasPrefix(value, "/") || strings.HasPrefix(value, "~") {
		return true
	}
	if strings.HasPrefix(value, "npm:") {
		return false
	}
	return strings.ContainsAny(value, `/\\`) || strings.HasSuffix(value, ".tgz") || strings.HasSuffix(value, ".tar.gz") || strings.HasSuffix(value, ".tar")
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
