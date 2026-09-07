package parser

// analyzeStructure parses the AST once and returns the AST-derived segments. ok
// is false when the content could not be parsed, so callers fall back to
// plain-text segments. grepple does not rank results, so this is used purely to
// build structural output segments for supported languages.
func analyzeStructure(config *languageConfig, content string, hits map[int]bool, maxSegments int) ([]Segment, bool) {
	tree, err := parseTree(config, content)
	if err != nil {
		return nil, false
	}
	defer tree.Close()
	root := tree.RootNode()
	return buildASTSegments(root, content, newMatchLines(hits), config, maxSegments), true
}
