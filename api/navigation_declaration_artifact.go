package api

import "github.com/greppleai/grepple/internal/parser"

// DecodeNavigationDeclarationArtifact verifies the artifact envelope and
// projects declaration/type evidence without allocating calls/imports/usages.
// It is not a substitute for complete graph decoding for graph operations.
func DecodeNavigationDeclarationArtifact(content []byte) (IndexedNavigation, error) {
	artifact, err := parser.UnmarshalNavigationDeclarationArtifact(content)
	if err != nil {
		return nil, err
	}
	return indexedNavigation{graph: artifact.Graph}, nil
}
