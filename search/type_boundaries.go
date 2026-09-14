package search

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/greppleai/grepple/parser"
)

// BoundaryTypeOrigin classifies where a reported concrete type is owned.
type BoundaryTypeOrigin string

// Boundary type origin values separate dependency provenance from imported identity.
const (
	BoundaryTypeOriginLocal           BoundaryTypeOrigin = "local"
	BoundaryTypeOriginFirstParty      BoundaryTypeOrigin = "first-party"
	BoundaryTypeOriginStandardLibrary BoundaryTypeOrigin = "standard-library"
	BoundaryTypeOriginThirdParty      BoundaryTypeOrigin = "third-party"
	BoundaryTypeOriginUnresolved      BoundaryTypeOrigin = "unresolved"
)

// BoundaryTypeRoles counts the ways a type participates in callable declarations and bodies.
type BoundaryTypeRoles struct {
	Parameters int `json:"parameters"`
	Results    int `json:"results"`
	Receivers  int `json:"receivers"`
	Locals     int `json:"locals"`
	Unknown    int `json:"unknown"`
}

// BoundaryTypeUsage identifies one callable and role contributing to a type's spread.
type BoundaryTypeUsage struct {
	BoundaryConsumer
	Role   string `json:"role"`
	Public bool   `json:"public,omitempty"`
	Test   bool   `json:"test,omitempty"`
}

// BoundaryTypeSpread describes a concrete imported or project-owned type used across files.
type BoundaryTypeSpread struct {
	Type            string              `json:"type"`
	ImportPath      string              `json:"importPath,omitempty"`
	CanonicalType   string              `json:"canonicalType"`
	Language        string              `json:"language"`
	OwnerFile       string              `json:"ownerFile,omitempty"`
	Origin          BoundaryTypeOrigin  `json:"origin"`
	External        bool                `json:"external"` // Compatibility: true when ImportPath is non-empty.
	Usages          int                 `json:"usages"`
	Consumers       BoundaryBreadth     `json:"consumers"`
	Production      BoundaryBreadth     `json:"production"`
	Tests           BoundaryBreadth     `json:"tests"`
	Roles           BoundaryTypeRoles   `json:"roles"`
	PublicExposures []BoundaryTypeUsage `json:"publicExposures,omitempty"`
	UsageDetails    []BoundaryTypeUsage `json:"usageDetails"`
}

type boundaryTypeSpreadKey struct {
	language   string
	typeName   string
	importPath string
	ownerFile  string
	origin     BoundaryTypeOrigin
}

type boundaryDependencyContext struct {
	roots []string
}

func newBoundaryDependencyContext(declarations map[string]parser.NavigationDeclaration) boundaryDependencyContext {
	rootSet := make(map[string]bool)
	for _, declaration := range declarations {
		for _, root := range []string{declaration.ModuleID, declaration.PackageID} {
			root = normalizeBoundaryImportPath(root)
			if root != "" && root != "." {
				rootSet[root] = true
			}
		}
	}
	roots := make([]string, 0, len(rootSet))
	for root := range rootSet {
		roots = append(roots, root)
	}
	sort.Strings(roots)
	return boundaryDependencyContext{roots: roots}
}

func boundaryTypeOriginForImport(language, importPath string, context boundaryDependencyContext) BoundaryTypeOrigin {
	path := normalizeBoundaryImportPath(importPath)
	if path == "" {
		return BoundaryTypeOriginUnresolved
	}
	if boundaryFirstPartyImport(path, context.roots) {
		return BoundaryTypeOriginFirstParty
	}
	if boundaryStandardLibraryImport(language, path) {
		return BoundaryTypeOriginStandardLibrary
	}
	if language == "go" {
		return BoundaryTypeOriginThirdParty
	}
	return BoundaryTypeOriginUnresolved
}

func boundaryStandardLibraryImport(language, path string) bool {
	switch language {
	case "go":
		first := path
		if separator := strings.IndexByte(first, '/'); separator >= 0 {
			first = first[:separator]
		}
		return !strings.Contains(first, ".")
	case "javascript", "typescript", "tsx":
		return strings.HasPrefix(path, "node:")
	case "java":
		return path == "java" || strings.HasPrefix(path, "java.") || path == "javax" || strings.HasPrefix(path, "javax.")
	case "kotlin":
		return path == "kotlin" || strings.HasPrefix(path, "kotlin.")
	case "csharp":
		return path == "System" || strings.HasPrefix(path, "System.")
	default:
		return false
	}
}

func boundaryFirstPartyImport(path string, roots []string) bool {
	if strings.HasPrefix(path, "./") || strings.HasPrefix(path, "../") {
		return true
	}
	for _, root := range roots {
		if path == root || strings.HasPrefix(path, root+"/") || strings.HasPrefix(path, root+".") {
			return true
		}
	}
	return false
}

func normalizeBoundaryImportPath(path string) string {
	return strings.Trim(strings.TrimSpace(path), "\"'<> ")
}

// AnalyzeTypeBoundaries finds imported concrete types and project-owned types whose
// explicit uses span at least two files. Imported types are identified by resolved
// import path; local types require an unambiguous declaration owner.
func AnalyzeTypeBoundaries(graph parser.NavigationGraph, minOccurrences int) ([]BoundaryTypeSpread, error) {
	if minOccurrences < 1 {
		return nil, fmt.Errorf("minimum occurrences must be positive")
	}
	declarations := indexBoundaryDeclarations(graph.Declarations)
	context := newBoundaryDependencyContext(declarations)
	groups := indexBoundaryTypeSpreads(graph.TypeUsages, declarations, boundaryTypeOwners(declarations), context)
	spreads := buildBoundaryTypeSpreads(groups, minOccurrences)
	sortBoundaryTypeSpreads(spreads)
	return spreads, nil
}

func indexBoundaryTypeSpreads(usages []parser.NavigationTypeUsage, declarations map[string]parser.NavigationDeclaration, localOwners map[boundaryTypeCandidate]boundaryOwnerKey, context boundaryDependencyContext) map[boundaryTypeSpreadKey][]BoundaryTypeUsage {
	groups := make(map[boundaryTypeSpreadKey][]BoundaryTypeUsage)
	seen := make(map[string]bool)
	for _, usage := range usages {
		declaration, ok := declarations[usage.CallerID]
		if !ok || usage.Type == "" || usage.Path == "" {
			continue
		}
		key, ok := boundaryTypeSpreadKeyForUsage(usage, localOwners, context)
		if !ok {
			continue
		}
		detail := boundaryTypeUsageDetail(usage, declaration)
		dedup := strings.Join([]string{key.language, key.typeName, key.importPath, key.ownerFile, detail.ID, detail.Role, fmt.Sprint(detail.Line)}, "\x00")
		if !seen[dedup] {
			seen[dedup] = true
			groups[key] = append(groups[key], detail)
		}
	}
	return groups
}

func boundaryTypeSpreadKeyForUsage(usage parser.NavigationTypeUsage, localOwners map[boundaryTypeCandidate]boundaryOwnerKey, context boundaryDependencyContext) (boundaryTypeSpreadKey, bool) {
	key := boundaryTypeSpreadKey{language: usage.Language, typeName: boundaryTerminalTypeName(usage.Type), importPath: usage.ImportPath}
	if key.typeName == "" {
		return boundaryTypeSpreadKey{}, false
	}
	if key.importPath != "" {
		key.origin = boundaryTypeOriginForImport(key.language, key.importPath, context)
		return key, true
	}
	owner, owned := localOwners[boundaryTypeCandidate{typeName: key.typeName, language: key.language}]
	if !owned || owner.path == usage.Path {
		return boundaryTypeSpreadKey{}, false
	}
	key.ownerFile = owner.path
	key.origin = BoundaryTypeOriginLocal
	return key, true
}

func boundaryTypeUsageDetail(usage parser.NavigationTypeUsage, declaration parser.NavigationDeclaration) BoundaryTypeUsage {
	role := normalizedBoundaryTypeRole(usage.Role)
	detail := BoundaryTypeUsage{BoundaryConsumer: boundaryConsumer(declaration), Role: role, Public: boundaryTypeUsageIsPublic(declaration, role), Test: boundaryTestPath(usage.Path)}
	if usage.Line > 0 {
		detail.Line = usage.Line
	}
	return detail
}

func buildBoundaryTypeSpreads(groups map[boundaryTypeSpreadKey][]BoundaryTypeUsage, minOccurrences int) []BoundaryTypeSpread {
	spreads := make([]BoundaryTypeSpread, 0, len(groups))
	for key, details := range groups {
		sortBoundaryTypeUsages(details)
		breadth := boundaryTypeUsageBreadth(details, nil)
		if breadth.Functions < minOccurrences || breadth.Files < 2 {
			continue
		}
		spread := BoundaryTypeSpread{
			Type: key.typeName, ImportPath: key.importPath, CanonicalType: boundaryCanonicalType(key.importPath, key.typeName), Language: key.language, OwnerFile: key.ownerFile, Origin: key.origin, External: key.importPath != "",
			Usages: len(details), Consumers: breadth, Production: boundaryTypeUsageBreadth(details, func(detail BoundaryTypeUsage) bool { return !detail.Test }),
			Tests: boundaryTypeUsageBreadth(details, func(detail BoundaryTypeUsage) bool { return detail.Test }), Roles: boundaryTypeRoles(details), UsageDetails: details,
		}
		for _, detail := range details {
			if detail.Public {
				spread.PublicExposures = append(spread.PublicExposures, detail)
			}
		}
		spreads = append(spreads, spread)
	}
	return spreads
}

func sortBoundaryTypeSpreads(spreads []BoundaryTypeSpread) {
	sort.Slice(spreads, func(i, j int) bool {
		leftPublic, rightPublic := boundaryExternalPublicExposures(spreads[i]), boundaryExternalPublicExposures(spreads[j])
		if leftPublic != rightPublic {
			return leftPublic > rightPublic
		}
		if spreads[i].Production.Files != spreads[j].Production.Files {
			return spreads[i].Production.Files > spreads[j].Production.Files
		}
		if spreads[i].Consumers.Files != spreads[j].Consumers.Files {
			return spreads[i].Consumers.Files > spreads[j].Consumers.Files
		}
		if spreads[i].External != spreads[j].External {
			return spreads[i].External
		}
		if spreads[i].Usages != spreads[j].Usages {
			return spreads[i].Usages > spreads[j].Usages
		}
		if spreads[i].CanonicalType != spreads[j].CanonicalType {
			return spreads[i].CanonicalType < spreads[j].CanonicalType
		}
		return spreads[i].Language < spreads[j].Language
	})
}

func boundaryExternalPublicExposures(spread BoundaryTypeSpread) int {
	if spread.External {
		return len(spread.PublicExposures)
	}
	return 0
}

func boundaryCanonicalType(importPath, typeName string) string {
	if importPath == "" {
		return typeName
	}
	return strings.TrimSuffix(importPath, "/") + "." + typeName
}

func normalizedBoundaryTypeRole(role string) string {
	switch role {
	case "parameter", "result", "receiver", "local":
		return role
	default:
		return "unknown"
	}
}

func boundaryTypeUsageIsPublic(declaration parser.NavigationDeclaration, role string) bool {
	if role != "parameter" && role != "result" && role != "receiver" {
		return false
	}
	if declaration.Visibility != parser.NavigationVisibilityPublic || boundaryTestPath(declaration.Path) {
		return false
	}
	if declaration.Language == "go" {
		container := boundaryTerminalTypeName(declaration.Container)
		if container == "" {
			if separator := strings.IndexByte(declaration.Name, '.'); separator > 0 {
				container = boundaryTerminalTypeName(declaration.Name[:separator])
			}
		}
		if container != "" && container[0] >= 'a' && container[0] <= 'z' {
			return false
		}
	}
	return true
}

func boundaryTestPath(path string) bool {
	path = "/" + strings.ToLower(filepath.ToSlash(path))
	base := filepath.Base(path)
	return strings.Contains(path, "/test/") || strings.Contains(path, "/tests/") || strings.Contains(path, "/__tests__/") ||
		strings.HasSuffix(base, "_test.go") || strings.Contains(base, ".test.") || strings.Contains(base, ".spec.")
}

func boundaryTypeUsageBreadth(details []BoundaryTypeUsage, include func(BoundaryTypeUsage) bool) BoundaryBreadth {
	functions := make(map[string]bool)
	files := make(map[string]bool)
	packages := make(map[string]bool)
	for _, detail := range details {
		if include != nil && !include(detail) {
			continue
		}
		functions[detail.ID] = true
		files[detail.Path] = true
		packages[detail.Package] = true
	}
	return BoundaryBreadth{Functions: len(functions), Files: len(files), Packages: len(packages)}
}

func boundaryTypeRoles(details []BoundaryTypeUsage) BoundaryTypeRoles {
	var roles BoundaryTypeRoles
	for _, detail := range details {
		switch detail.Role {
		case "parameter":
			roles.Parameters++
		case "result":
			roles.Results++
		case "receiver":
			roles.Receivers++
		case "local":
			roles.Locals++
		default:
			roles.Unknown++
		}
	}
	return roles
}

func sortBoundaryTypeUsages(details []BoundaryTypeUsage) {
	sort.Slice(details, func(i, j int) bool {
		if details[i].Public != details[j].Public {
			return details[i].Public
		}
		if details[i].Test != details[j].Test {
			return !details[i].Test
		}
		if details[i].Path != details[j].Path {
			return details[i].Path < details[j].Path
		}
		if details[i].Line != details[j].Line {
			return details[i].Line < details[j].Line
		}
		if details[i].Name != details[j].Name {
			return details[i].Name < details[j].Name
		}
		return details[i].Role < details[j].Role
	})
}
