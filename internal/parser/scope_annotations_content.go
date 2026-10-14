package parser

import "strings"

// LineScopeAnnotationsWithRanges shares one code parse with the legacy
// construct-range collector when both are requested. Specialized content has
// no legacy code ranges; it retains its one projection per file.
func LineScopeAnnotationsWithRanges(content, language string, hits map[int]bool, includeRanges, enclosing bool) (map[int]LineScopeAnnotation, map[int]StructuralLineRange) {
	if content == "" || strings.IndexByte(content, 0) >= 0 || len(hits) == 0 {
		return nil, nil
	}
	if languageAdapters[language] != nil {
		return codeLineScopeAndRanges(content, language, hits, includeRanges, enclosing)
	}
	return LineScopeAnnotations(content, language, hits), nil
}

// LineScopeAnnotations dispatches one source projection per selected file. A
// missing annotation never changes which lines matched. Callers supply the
// canonical content-language ID, not an inferred grammar node name.
func LineScopeAnnotations(content, language string, hits map[int]bool) map[int]LineScopeAnnotation {
	if content == "" || strings.IndexByte(content, 0) >= 0 || len(hits) == 0 {
		return nil
	}
	switch language {
	case "markdown":
		return markdownLineScopeAnnotations(content, hits)
	case "json", "yaml":
		return structuredLineScopeAnnotations(content, language, hits)
	case "text":
		return nil
	default:
		return CodeLineScopeAnnotations(content, language, hits)
	}
}
