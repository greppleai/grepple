package parser

import (
	"reflect"
	"testing"
)

func scopeSpan(kind string, start, end int) *ScopeLineRange {
	return &ScopeLineRange{Kind: kind, StartLine: start, EndLine: end}
}
func verifyContentScopes(t *testing.T, got map[int]LineScopeAnnotation, line int, nearest, scope *ScopeLineRange) {
	t.Helper()
	if !reflect.DeepEqual(got[line].Nearest, nearest) || !reflect.DeepEqual(got[line].Scope, scope) {
		t.Errorf("line %d: got %+v, want nearest=%+v scope=%+v", line, got[line], nearest, scope)
	}
}

func TestMarkdownLineScopeHeadingBoundaries(t *testing.T) {
	content := "preamble\n\n# Doc\nintro\n## Alpha\nfirst\n### Sub\nchild\n## Beta\nsecond\n\nSetext\n------\nthird\n# Next\nend\n"
	hits := map[int]bool{1: true, 2: true, 3: true, 4: true, 5: true, 6: true, 7: true, 8: true, 9: true, 10: true, 11: true, 12: true, 13: true, 14: true, 15: true, 16: true, 17: true}
	got := LineScopeAnnotations(content, "markdown", hits)
	for _, line := range []int{1, 2, 17} {
		verifyContentScopes(t, got, line, nil, nil)
	}
	for _, test := range []struct {
		lines      []int
		start, end int
	}{
		{[]int{3, 4}, 3, 14}, {[]int{5, 6}, 5, 8}, {[]int{7, 8}, 7, 8}, {[]int{9, 10, 11}, 9, 11},
		{[]int{12, 13, 14}, 12, 14}, {[]int{15, 16}, 15, 16},
	} {
		for _, line := range test.lines {
			span := scopeSpan("section", test.start, test.end)
			verifyContentScopes(t, got, line, span, span)
		}
	}
}

func TestStructuredLineScopesJSONDelimitersAndNestedContainers(t *testing.T) {
	content := "{\n  \"items\": [\n    {\"label\": \"} not a delimiter\"},\n    {\"id\": 2}\n  ],\n  \"meta\": {\n    \"ok\": true\n  }\n}\n"
	got := LineScopeAnnotations(content, "json", map[int]bool{1: true, 2: true, 3: true, 4: true, 5: true, 6: true, 7: true, 8: true, 9: true, 10: true})
	verifyContentScopes(t, got, 1, scopeSpan("object", 1, 9), scopeSpan("object", 1, 9))
	verifyContentScopes(t, got, 2, scopeSpan("array", 2, 5), scopeSpan("object", 1, 9))
	verifyContentScopes(t, got, 3, scopeSpan("object", 3, 3), scopeSpan("array", 2, 5))
	verifyContentScopes(t, got, 4, scopeSpan("object", 4, 4), scopeSpan("array", 2, 5))
	verifyContentScopes(t, got, 5, scopeSpan("array", 2, 5), scopeSpan("object", 1, 9))
	verifyContentScopes(t, got, 6, scopeSpan("object", 6, 8), scopeSpan("object", 1, 9))
	verifyContentScopes(t, got, 7, scopeSpan("object", 6, 8), scopeSpan("object", 1, 9))
	verifyContentScopes(t, got, 8, scopeSpan("object", 6, 8), scopeSpan("object", 1, 9))
	verifyContentScopes(t, got, 9, scopeSpan("object", 1, 9), scopeSpan("object", 1, 9))
	verifyContentScopes(t, got, 10, nil, nil)
}

func TestStructuredLineScopesYAMLNestedAndMultiDocument(t *testing.T) {
	content := "kind: Service\nmetadata:\n  name: a\n  labels:\n    tier: api\n---\nkind: ConfigMap\ndata:\n  - name: one\n    value: two\n  - name: three\n    value: four\n"
	hits := map[int]bool{1: true, 2: true, 3: true, 4: true, 5: true, 6: true, 7: true, 8: true, 9: true, 10: true, 11: true, 12: true}
	got := LineScopeAnnotations(content, "yaml", hits)
	verifyContentScopes(t, got, 1, scopeSpan("mapping", 1, 5), scopeSpan("document", 1, 5))
	verifyContentScopes(t, got, 3, scopeSpan("mapping", 2, 5), scopeSpan("mapping", 1, 5))
	verifyContentScopes(t, got, 5, scopeSpan("mapping", 4, 5), scopeSpan("mapping", 2, 5))
	verifyContentScopes(t, got, 6, nil, nil)
	verifyContentScopes(t, got, 7, scopeSpan("mapping", 7, 12), scopeSpan("document", 7, 12))
	verifyContentScopes(t, got, 8, scopeSpan("sequence", 8, 12), scopeSpan("mapping", 7, 12))
	verifyContentScopes(t, got, 10, scopeSpan("mapping", 9, 10), scopeSpan("sequence", 8, 12))
	verifyContentScopes(t, got, 12, scopeSpan("mapping", 11, 12), scopeSpan("sequence", 8, 12))
}

func TestContentScopesFallbacksAndRegistry(t *testing.T) {
	for _, test := range []struct{ content, lang string }{
		{"", "json"}, {"abc", "text"}, {"# hello\n", "text"}, {"abc", "unknown"},
		{"foo\x00bar", "go"}, {"{\"a\": 1,}", "json"}, {"{\"a\": 1", "json"},
		{"a: x\n---\na: [invalid\n", "yaml"}, {"plain before\n", "markdown"},
	} {
		if got := LineScopeAnnotations(test.content, test.lang, map[int]bool{1: true, 2: true, 3: true}); len(got) > 0 {
			t.Errorf("%q %q: unexpected %+v", test.lang, test.content, got)
		}
	}
	// The general dispatcher must retain the code projection rather than making
	// a content-only alternate API.
	got := LineScopeAnnotations("package p\nfunc run() {\n  work()\n}\n", "go", map[int]bool{3: true})
	verifyContentScopes(t, got, 3, scopeSpan("func", 2, 4), scopeSpan("func", 2, 4))
	ids := map[string]bool{}
	for _, language := range NewParser().SupportedContentLanguages() {
		ids[language.ID] = true
	}
	for _, id := range []string{"markdown", "json", "yaml", "text"} {
		if !ids[id] {
			t.Errorf("unsupported fixture ID %s", id)
		}
	}
	if len(ids) != len(languageAdapters)+4 {
		t.Errorf("content matrix %d languages, want %d", len(ids), len(languageAdapters)+4)
	}
}
