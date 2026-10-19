package render

import (
	"bytes"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/memory"
	"github.com/greppleai/grepple/internal/search"
)

func TestMemoryFooterBoundAndSanitization(t *testing.T) {
	found := memory.Discovery{Complete: true, Omitted: 5}
	for i := 0; i < 3; i++ {
		found.References = append(found.References, memory.Reference{Name: "memo", Description: strings.Repeat("é", 79) + "\u202espoof"})
	}
	footer := memoryFooter(found)
	if len(footer) > memoryFooterLimit || strings.Contains(footer, "\u202e") || !strings.Contains(footer, "more memories") {
		t.Fatalf("unsafe footer len=%d %q", len(footer), footer)
	}
	if got := memoryFooter(memory.Discovery{Complete: false}); got != "memory references incomplete\n" {
		t.Fatalf("incomplete marker %q", got)
	}
	if got := memoryFooter(memory.Discovery{Complete: true}); got != "" {
		t.Fatalf("empty footer %q", got)
	}
	if desc := safeMemoryDescription("a\u202eb", 120); desc != "ab" {
		t.Fatalf("unsafe description %q", desc)
	}
}
func TestMemoryFooterCannotTruncateExistingResult(t *testing.T) {
	results := []search.FileResult{{Path: "a.go", Matches: []search.ResultMatch{{Line: 2, Text: "needle"}}}}
	baseOptions := Options{JSON: "off", LineOnly: true}
	var base bytes.Buffer
	if _, err := Render(baseOptions, results, &base); err != nil {
		t.Fatal(err)
	}
	found := memory.Discovery{Complete: true, References: []memory.Reference{{Name: "first", Description: "Brief hint"}}}
	opts := baseOptions
	opts.Memories = &found
	observed := false
	for capBytes := base.Len() + 1; capBytes < base.Len()+300; capBytes++ {
		opts.MaxOutputBytes = capBytes
		var output, stderr bytes.Buffer
		if err := Search(SearchOptions{Options: opts, Output: &output, ErrorOutput: &stderr}, results); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(stderr.String(), "memory references omitted") {
			if output.String() != base.String() {
				t.Fatalf("changed result at cap %d: %q vs %q", capBytes, output.String(), base.String())
			}
			observed = true
			break
		}
	}
	if !observed {
		t.Fatal("no footer-only omission observed")
	}
	var unlimited bytes.Buffer
	opts.MaxOutputBytes = 0
	if err := Search(SearchOptions{Options: opts, Output: &unlimited}, results); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(unlimited.String(), base.String()) || !strings.Contains(unlimited.String(), "grepple memory recall first") {
		t.Fatalf("footer did not share output writer %q", unlimited.String())
	}
}
