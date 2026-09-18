package parser

import "strings"

// navigationTypeKinds is the language-neutral vocabulary emitted by adapters'
// outlines for source-declared types. Grammar node kinds remain adapter-owned.
var navigationTypeKinds = newStringSet("class", "enum", "interface", "object", "record", "struct", "trait", "type", "typedef", "union")

func navigationTypeDeclarations(symbols []Symbol, language, path, packageName string) []NavigationTypeDeclaration {
	var declarations []NavigationTypeDeclaration
	var collect func([]Symbol, string)
	collect = func(current []Symbol, container string) {
		for _, symbol := range current {
			nextContainer := container
			if navigationTypeKinds.contains(symbol.Kind) && symbol.Name != "" {
				name := symbol.Name
				if container != "" && !strings.Contains(name, ".") {
					name = container + "." + name
				}
				declarations = append(declarations, NavigationTypeDeclaration{Name: name, Kind: symbol.Kind, Language: language, Path: path, Container: container, Package: packageName, Start: symbol.Start, End: symbol.End})
				nextContainer = name
			}
			collect(symbol.Children, nextContainer)
		}
	}
	collect(symbols, "")
	return declarations
}
