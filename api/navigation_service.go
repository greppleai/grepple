package api

import (
	"github.com/greppleai/grepple/internal/navigation"
	"github.com/greppleai/grepple/internal/parser"
)

// NavigationArtifactSchema identifies the portable, versioned graph format.
const NavigationArtifactSchema = parser.NavigationFactArtifactSchema

// NavigationCacheDirectoryEnv controls optional source-fact caching on a shard.
const NavigationCacheDirectoryEnv = parser.NavigationCacheDirectoryEnv

// NPMRegistrySource identifies verified public-registry npm artifacts.
const NPMRegistrySource = navigation.NPMRegistrySource

// NavigationVisibilityPublic is the source-language public declaration class.
const NavigationVisibilityPublic = "public"

// NavigationSourceStats records how many selected sources were fully parsed.
type NavigationSourceStats struct {
	Attempted, Parsed, Skipped, Failed, Recovered int
}

// IndexedNavigation exposes only artifact encoding and projected declarations.
// The underlying parser graph and its representation remain private.
type IndexedNavigation interface {
	Encode(digest string, recovered bool) ([]byte, error)
	VisitDeclarations(func(ArtifactDeclaration))
	VisitTypeDeclarations(func(ArtifactTypeDeclaration))
}

type indexedNavigation struct{ graph parser.NavigationGraph }

// DecodeNavigationArtifact validates and loads one portable navigation artifact.
func DecodeNavigationArtifact(content []byte) (IndexedNavigation, error) {
	artifact, err := parser.UnmarshalNavigationFactArtifact(content)
	if err != nil {
		return nil, err
	}
	return indexedNavigation{graph: artifact.Graph}, nil
}

// BuildNavigationArtifact parses and resolves a complete set of source paths.
func BuildNavigationArtifact(paths []string) (IndexedNavigation, NavigationSourceStats) {
	graph, stats := navigation.BuildGraphWithStats(paths)
	return indexedNavigation{graph: graph}, NavigationSourceStats{
		Attempted: stats.Attempted, Parsed: stats.Parsed, Skipped: stats.Skipped,
		Failed: stats.Failed, Recovered: stats.Recovered,
	}
}

// Encode serializes the graph and its declared identity into the native format.
func (artifact indexedNavigation) Encode(digest string, recovered bool) ([]byte, error) {
	return parser.MarshalNavigationFactArtifact(parser.NavigationFactArtifact{Digest: digest, Recovered: recovered, Graph: artifact.graph})
}

// ArtifactDeclaration is the source-backed callable data needed to resolve an
// indexed external reference; it deliberately omits parser-only graph details.
type ArtifactDeclaration struct {
	Name, Kind, Path, Package, PackageID, Receiver, Container, Visibility string
	Start, End                                                            int
}

// VisitDeclarations projects callable facts one at a time without allocating
// a second full graph for each external reference resolved against an artifact.
func (artifact indexedNavigation) VisitDeclarations(visit func(ArtifactDeclaration)) {
	for _, declaration := range artifact.graph.Declarations {
		visit(ArtifactDeclaration{
			Name: declaration.Name, Kind: declaration.Kind, Path: declaration.Path,
			Package: declaration.Package, PackageID: declaration.PackageID,
			Receiver: declaration.Receiver, Container: declaration.Container,
			Visibility: string(declaration.Visibility), Start: declaration.Start, End: declaration.End,
		})
	}
}

// ArtifactTypeDeclaration is the source-backed type data needed by a shard.
type ArtifactTypeDeclaration struct {
	Name, Kind, Path, Package, PackageID string
	Start, End                           int
}

// VisitTypeDeclarations projects source-declared types without copying the
// entire artifact's declarations for each reference lookup.
func (artifact indexedNavigation) VisitTypeDeclarations(visit func(ArtifactTypeDeclaration)) {
	for _, declaration := range artifact.graph.TypeDeclarations {
		visit(ArtifactTypeDeclaration{
			Name: declaration.Name, Kind: declaration.Kind, Path: declaration.Path,
			Package: declaration.Package, PackageID: declaration.PackageID,
			Start: declaration.Start, End: declaration.End,
		})
	}
}

// ArtifactModule is one publishable module discovered in an indexed repository.
type ArtifactModule struct {
	Ecosystem, Module, Version, Root string
}

// DiscoverArtifactModules reads supported manifests and lockfiles for a repository.
func DiscoverArtifactModules(root string) ([]ArtifactModule, error) {
	modules, err := navigation.DiscoverArtifactModules(root)
	if err != nil {
		return nil, err
	}
	out := make([]ArtifactModule, len(modules))
	for i, module := range modules {
		out[i] = ArtifactModule{Ecosystem: module.Ecosystem, Module: module.Module, Version: module.Version, Root: module.Root}
	}
	return out, nil
}

// ValidNPMIntegrity checks the supported structural SRI forms.
func ValidNPMIntegrity(integrity string) bool { return navigation.ValidNPMIntegrity(integrity) }

// QualifyRepositoryDependencies adds manifest evidence to repository results.
func QualifyRepositoryDependencies(results []FileResult, root string) error {
	return navigation.QualifyRepositoryExternalDependencies(results, root)
}

// QualifyExternalDependencies adds manifest evidence to local search results.
func QualifyExternalDependencies(results []FileResult, root string) error {
	return navigation.QualifyExternalDependencies(results, root)
}

// ExternalDependencyReferences lists unresolved references in search results.
func ExternalDependencyReferences(results []FileResult) []ExternalNavigationReference {
	return navigation.ExternalDependencyReferences(results)
}
