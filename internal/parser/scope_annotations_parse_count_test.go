package parser

import (
	"fmt"
	"strings"
	"testing"
)

type countingScopeAdapter struct {
	languageAdapter
	parses *int
}

func (a countingScopeAdapter) Parse(content string) (*syntaxTree, error) {
	*a.parses++
	return a.languageAdapter.Parse(content)
}

func TestScopeAnnotationsParseOncePerFileNotPerHit(t *testing.T) {
	original := languageAdapters["go"]
	parses := 0
	languageAdapters["go"] = countingScopeAdapter{languageAdapter: original, parses: &parses}
	t.Cleanup(func() { languageAdapters["go"] = original })
	var source strings.Builder
	source.WriteString("package p\nfunc run() {\n")
	hits := make(map[int]bool, 1000)
	for i := 0; i < 1000; i++ {
		fmt.Fprintf(&source, " work(%d)\n", i)
		hits[i+3] = true
	}
	source.WriteString("}\n")
	for _, n := range []int{1, 100, 1000} {
		chosen := make(map[int]bool, n)
		for i := range n {
			chosen[i+3] = true
		}
		parses = 0
		annotations, old := LineScopeAnnotationsWithRanges(source.String(), "go", chosen, true, false)
		if parses != 1 || len(annotations) != n || len(old) == 0 || len(old) > n {
			t.Fatalf("hits=%d: parses=%d roles=%d ranges=%d", n, parses, len(annotations), len(old))
		}
	}
	parses = 0
	if got, _ := LineScopeAnnotationsWithRanges(source.String(), "go", nil, true, false); len(got) != 0 || parses != 0 {
		t.Fatalf("empty hit set parsed %d times", parses)
	}
}
