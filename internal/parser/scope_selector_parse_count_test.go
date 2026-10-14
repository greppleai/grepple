package parser

import (
	"fmt"
	"strings"
	"testing"
)

func TestScopeSelectorParsesOnceAndReusesCallerDocument(t *testing.T) {
	original := languageAdapters["go"]
	parses := 0
	languageAdapters["go"] = countingScopeAdapter{languageAdapter: original, parses: &parses}
	t.Cleanup(func() { languageAdapters["go"] = original })
	var source strings.Builder
	source.WriteString("package p\nfunc run() {\n")
	for i := 0; i < 1000; i++ {
		fmt.Fprintf(&source, " work(%d)\n", i)
	}
	source.WriteString("}\n")
	content := source.String()
	selector := ScopeSelector{Kind: "func", StartLine: 2, EndLine: 1003, Exact: true}
	if span, status := ResolveScopeSelector(content, "go", selector, nil); status != ScopeVerified || span.EndLine != 1003 || parses != 1 {
		t.Fatalf("cold span=%+v status=%s parses=%d", span, status, parses)
	}
	document, err := parseDocument("go", content)
	if err != nil {
		t.Fatal(err)
	}
	defer document.Close()
	if parses != 2 {
		t.Fatalf("document parses=%d", parses)
	}
	selector = ScopeSelector{Kind: "func", StartLine: 503, EndLine: 503}
	if span, status := ResolveScopeSelector(content, "go", selector, document); status != ScopeVerified || span.StartLine != 2 || span.EndLine != 1003 || parses != 2 {
		t.Fatalf("document span=%+v status=%s parses=%d", span, status, parses)
	}
}
