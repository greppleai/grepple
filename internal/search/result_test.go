package search

import (
	"testing"

	"github.com/greppleai/grepple/internal/parser"
)

func TestResultSegmentsPreservesShortWhitespaceGap(t *testing.T) {
	segments := []parser.Segment{
		{Kind: "summary", Start: 1, End: 1, Text: "first"},
		{Kind: "lines", Start: 4, End: 4},
	}
	result := resultSegments("first\n\n  \nfourth", segments)
	if len(result) != 3 || result[1].Kind != "spacing" || result[1].Start != 2 || result[1].End != 3 || result[1].Text != "\n  " {
		t.Fatalf("short whitespace gap=%#v", result)
	}
}

func TestResultSegmentsKeepsMeaningfulAndLongGapsCollapsed(t *testing.T) {
	meaningful := resultSegments("first\nwork()\n\nfourth", []parser.Segment{
		{Kind: "summary", Start: 1, End: 1, Text: "first"},
		{Kind: "lines", Start: 4, End: 4},
	})
	if len(meaningful) != 2 {
		t.Fatalf("meaningful gap should remain collapsed: %#v", meaningful)
	}

	long := resultSegments("first\n\n\n\nfifth", []parser.Segment{
		{Kind: "summary", Start: 1, End: 1, Text: "first"},
		{Kind: "lines", Start: 5, End: 5},
	})
	if len(long) != 2 {
		t.Fatalf("long whitespace gap should remain collapsed: %#v", long)
	}
}
