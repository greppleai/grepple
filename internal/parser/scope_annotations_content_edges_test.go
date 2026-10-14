package parser

import "testing"

func TestStructuredScopesUncertainYAMLBoundsOmittedIndependently(t *testing.T) {
	content := "# file comment\nfirst:\n  ok: yes\n# between\nflow: [1,\n  2]\nlast:\n  id: 3\n"
	got := LineScopeAnnotations(content, "yaml", map[int]bool{1: true, 2: true, 3: true, 4: true, 5: true, 6: true, 7: true, 8: true})
	verifyContentScopes(t, got, 1, nil, nil)
	verifyContentScopes(t, got, 2, scopeSpan("mapping", 2, 3), scopeSpan("mapping", 2, 3))
	verifyContentScopes(t, got, 3, scopeSpan("mapping", 2, 3), scopeSpan("mapping", 2, 3))
	verifyContentScopes(t, got, 4, nil, nil)
	verifyContentScopes(t, got, 5, nil, nil)
	verifyContentScopes(t, got, 6, nil, nil)
	verifyContentScopes(t, got, 7, scopeSpan("mapping", 7, 8), scopeSpan("mapping", 7, 8))
	verifyContentScopes(t, got, 8, scopeSpan("mapping", 7, 8), scopeSpan("mapping", 7, 8))
	block := "first:\n  ok: yes\nmessage: |\n  many\n  lines\nlast:\n  id: 3\n"
	got = LineScopeAnnotations(block, "yaml", map[int]bool{2: true, 4: true, 5: true, 7: true})
	verifyContentScopes(t, got, 2, scopeSpan("mapping", 1, 2), scopeSpan("mapping", 1, 2))
	verifyContentScopes(t, got, 4, nil, nil)
	verifyContentScopes(t, got, 5, nil, nil)
	verifyContentScopes(t, got, 7, scopeSpan("mapping", 6, 7), scopeSpan("mapping", 6, 7))
}

func TestStructuredScopesYAMLDocumentsAndJSONWhitespace(t *testing.T) {
	stream := "---\nhello\n---\n- one\n- two\n"
	got := LineScopeAnnotations(stream, "yaml", map[int]bool{1: true, 2: true, 3: true, 4: true, 5: true})
	verifyContentScopes(t, got, 1, nil, nil)
	verifyContentScopes(t, got, 2, scopeSpan("document", 2, 2), scopeSpan("document", 2, 2))
	verifyContentScopes(t, got, 3, nil, nil)
	verifyContentScopes(t, got, 4, scopeSpan("sequence", 4, 5), scopeSpan("document", 4, 5))
	verifyContentScopes(t, got, 5, scopeSpan("sequence", 4, 5), scopeSpan("document", 4, 5))
	jsonContent := "{\n\n  \"a\": {\n    \"b\": [\n      1\n    ]\n  }\n}\n"
	got = LineScopeAnnotations(jsonContent, "json", map[int]bool{1: true, 2: true, 4: true, 6: true, 8: true})
	verifyContentScopes(t, got, 2, nil, nil)
	verifyContentScopes(t, got, 4, scopeSpan("array", 4, 6), scopeSpan("object", 3, 7))
	verifyContentScopes(t, got, 6, scopeSpan("array", 4, 6), scopeSpan("object", 3, 7))
	verifyContentScopes(t, got, 8, scopeSpan("object", 1, 8), scopeSpan("object", 1, 8))
}

func TestStructuredScopesSameLineSiblingAmbiguity(t *testing.T) {
	content := `{"a": {}, "b": {}}`
	got := LineScopeAnnotations(content, "json", map[int]bool{1: true})
	verifyContentScopes(t, got, 1, nil, nil)
	// A nested one-line container is not a sibling ambiguity: depth proves
	// ownership even when both spans share the same line number.
	got = LineScopeAnnotations(`{"a": {"b": 1}}`, "json", map[int]bool{1: true})
	verifyContentScopes(t, got, 1, scopeSpan("object", 1, 1), scopeSpan("object", 1, 1))
}

func TestMarkdownLineScopeIgnoresFencedFakeHeadings(t *testing.T) {
	content := "# Real\n```md\n# Fake\n```\nafter\n\n- # Nested\n"
	got := LineScopeAnnotations(content, "markdown", map[int]bool{3: true, 5: true, 7: true})
	for _, line := range []int{3, 5, 7} {
		verifyContentScopes(t, got, line, scopeSpan("section", 1, 7), scopeSpan("section", 1, 7))
	}
}
