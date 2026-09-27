package render

import (
	"fmt"
	"strings"

	"github.com/greppleai/grepple/internal/parser"
)

// OutlineKinds is the set of requested declaration categories. Nil means no filter.
type OutlineKinds map[string]bool

// ParseOutlineKinds accepts repeatable or comma-separated category names.
func ParseOutlineKinds(values []string, outline bool) (OutlineKinds, error) {
	if len(values) == 0 {
		return nil, nil
	}
	if !outline {
		return nil, fmt.Errorf("--kind requires --outline")
	}
	kinds := make(OutlineKinds)
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			kind := strings.TrimSpace(part)
			switch kind {
			case "types", "functions", "variables":
				kinds[kind] = true
			default:
				return nil, fmt.Errorf("invalid --kind %q: expected types, functions, or variables", kind)
			}
		}
	}
	return kinds, nil
}

// FilterOutline retains selected declarations, lifting matching descendants out
// of unselected containers so nested methods remain visible in functions-only mode.
func FilterOutline(outline parser.FileOutline, kinds OutlineKinds) parser.FileOutline {
	if len(kinds) == 0 {
		return outline
	}
	// Config shape nodes (notably JSON/YAML "object") are not code types.
	switch outline.Language {
	case "json", "yaml", "markdown":
		outline.Symbols = []parser.Symbol{}
		return outline
	}
	outline.Symbols = filterSymbols(outline.Symbols, kinds)
	return outline
}

func filterSymbols(symbols []parser.Symbol, kinds OutlineKinds) []parser.Symbol {
	result := make([]parser.Symbol, 0)
	for _, symbol := range symbols {
		children := filterSymbols(symbol.Children, kinds)
		if kinds[symbolCategory(symbol.Kind)] {
			symbol.Children = children
			result = append(result, symbol)
		} else {
			result = append(result, children...)
		}
	}
	return result
}

func symbolCategory(kind string) string {
	switch kind {
	case "type", "typedef", "class", "struct", "interface", "enum", "union", "trait", "record", "concept", "object":
		return "types"
	case "func", "function", "fun", "method", "constructor":
		return "functions"
	case "var", "val", "const", "static", "field", "property":
		return "variables"
	}
	return ""
}
