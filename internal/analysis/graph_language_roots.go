package analysis

import "github.com/greppleai/grepple/internal/parser"

// File/package identity is shared across root rows, not repeated per callable.
// Only selector fields are stored; query output still hydrates native details.
type languageRootSource struct{ Path, Language, Package, PackageID, ModuleID string }
type languageRootRow struct {
	ID, Name   string
	Source     uint32
	Start, End int
}

func languageManifestDeclarations(manifest languageManifest) []parser.NavigationDeclaration {
	declarations := make([]parser.NavigationDeclaration, 0, len(manifest.Roots))
	for _, root := range manifest.Roots {
		source := manifest.RootSources[root.Source]
		declarations = append(declarations, parser.NavigationDeclaration{ID: root.ID, Name: root.Name, Start: root.Start, End: root.End, Path: source.Path, Language: source.Language, Package: source.Package, PackageID: source.PackageID, ModuleID: source.ModuleID})
	}
	return declarations
}
