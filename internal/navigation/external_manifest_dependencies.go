package navigation

import "github.com/greppleai/grepple/internal/dependency"

// ArtifactModule identifies one publishable module found in a repository.
// It remains an alias for compatibility; dependency resolvers own discovery.
type ArtifactModule = dependency.ArtifactModule

// NPMRegistrySource is the canonical public npm registry identity.
const NPMRegistrySource = dependency.NPMRegistrySource

// NormalizeNPMRegistryEvidence validates canonical public-registry archive evidence.
func NormalizeNPMRegistryEvidence(module, version, resolved, integrity string) (string, bool) {
	return dependency.NormalizeNPMRegistryEvidence(module, version, resolved, integrity)
}

// ValidNPMIntegrity validates the supported structural SRI forms.
func ValidNPMIntegrity(integrity string) bool {
	return dependency.ValidNPMIntegrity(integrity)
}

// DiscoverArtifactModules delegates manifest interpretation to the dependency resolver registry.
func DiscoverArtifactModules(root string) ([]ArtifactModule, error) {
	return externalDependencyResolvers.DiscoverArtifactModules(root)
}
