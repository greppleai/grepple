package dependency

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"golang.org/x/mod/semver"
)

// NPMRegistrySource is the only npm registry identity currently accepted as exact evidence.
const NPMRegistrySource = "https://registry.npmjs.org"

type npmDependencyResolver struct{}

var _ Resolver = npmDependencyResolver{}

// NewNPMResolver returns the resolver for npm package manifests and lockfiles.
func NewNPMResolver() Resolver { return npmDependencyResolver{} }

func (npmDependencyResolver) ID() string { return "npm" }
func (npmDependencyResolver) Languages() []string {
	return []string{"javascript", "typescript", "tsx"}
}
func (npmDependencyResolver) ManifestNames() []string { return []string{"package.json"} }

func (npmDependencyResolver) Resolve(manifestPath string) (Resolution, error) {
	content, err := os.ReadFile(manifestPath)
	if err != nil {
		return Resolution{}, err
	}
	var manifest npmManifest
	if err := json.Unmarshal(content, &manifest); err != nil {
		return Resolution{}, err
	}
	if !npmPackageManagerSelected(manifest.PackageManager) {
		return Resolution{Applicable: false}, nil
	}
	if !npmPackageManagerAllowsLockfile(manifest.PackageManager) {
		return Resolution{Applicable: true}, nil
	}
	lock, authoritative, err := readAuthoritativeNPMLock(filepath.Dir(manifestPath))
	if err != nil || !authoritative {
		return Resolution{Applicable: true}, err
	}
	dependencies := make([]Evidence, 0, len(manifest.Dependencies))
	for name, constraint := range manifest.Dependencies {
		if evidence, ok := npmLockedDependencyEvidence(lock, name, constraint); ok {
			dependencies = append(dependencies, evidence)
		}
	}
	return Resolution{Applicable: true, Dependencies: dependencies}, nil
}

func (npmDependencyResolver) Match(importPath string, dependencies []Evidence) Match {
	matches := namedDependencyMatches(npmPackageName(importPath), dependencies, nil)
	return exactMatch(importPath, matches, func(match Evidence) string {
		return match.Module + strings.TrimPrefix(importPath, match.ImportName)
	})
}

func (npmDependencyResolver) DiscoverModule(manifestPath, relativeRoot string) (*ArtifactModule, error) {
	content, err := os.ReadFile(manifestPath)
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
	return &ArtifactModule{Ecosystem: "npm", Module: manifest.Name, Version: manifest.Version, Root: relativeRoot}, nil
}

type npmManifest struct {
	Name           string            `json:"name"`
	Version        string            `json:"version"`
	PackageManager string            `json:"packageManager"`
	Dependencies   map[string]string `json:"dependencies"`
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

func npmPackageManagerSelected(specifier string) bool {
	if specifier == "" {
		return true
	}
	value := strings.TrimSpace(specifier)
	return value == "npm" || strings.HasPrefix(value, "npm@")
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

func readAuthoritativeNPMLock(directory string) (npmLock, bool, error) {
	content, err := readNPMLockfile(directory)
	if err != nil || content == nil {
		return npmLock{}, false, err
	}
	var lock npmLock
	if err := json.Unmarshal(content, &lock); err != nil {
		return npmLock{}, false, nil
	}
	if lock.LockfileVersion < 1 || lock.LockfileVersion > 3 {
		return npmLock{}, false, nil
	}
	return lock, true, nil
}

func npmLockedDependencyEvidence(lock npmLock, name, constraint string) (Evidence, bool) {
	if npmUnsupportedDependencySource(constraint) {
		return Evidence{}, false
	}
	entry, ok := npmLockedDirectDependency(lock, name)
	if !ok {
		return Evidence{}, false
	}
	module, version, ok := npmLockedDependencyIdentity(name, constraint, entry)
	if !ok || entry.Link {
		return Evidence{}, false
	}
	source, ok := NormalizeNPMRegistryEvidence(module, version, entry.Resolved, entry.Integrity)
	if !ok {
		return Evidence{}, false
	}
	return Evidence{Ecosystem: "npm", ImportName: name, Module: module, Version: version, Integrity: entry.Integrity, Source: source}, true
}

func readNPMLockfile(directory string) ([]byte, error) {
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
