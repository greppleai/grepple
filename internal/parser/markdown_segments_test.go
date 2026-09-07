package parser

import "testing"

func segmentKinds(segs []Segment) map[int]string {
	m := map[int]string{}
	for _, s := range segs {
		m[s.Start] = s.Kind
	}
	return m
}

func TestLanguageForMarkdown(t *testing.T) {
	for _, ext := range []string{"a.md", "b.markdown", "c.mkd"} {
		if got := LanguageFor(ext); got != "markdown" {
			t.Fatalf("LanguageFor(%q) = %q, want markdown", ext, got)
		}
	}
}

// TestMarkdownSegmentsBreadcrumb verifies a match deep in a document is preceded
// by its enclosing heading chain (rendered as summary segments), mirroring how the
// AST path shows a match's enclosing function/class.
func TestMarkdownSegmentsBreadcrumb(t *testing.T) {
	content := "# Doc\n\nintro\n\n## Section\n\n### Sub\n\nthe needle here\n"
	// lines: 1 "# Doc", 5 "## Section", 7 "### Sub", 9 "the needle here"
	segs := buildMarkdownSegments(content, map[int]bool{9: true}, 20)
	kinds := segmentKinds(segs)
	for _, ln := range []int{1, 5, 7} {
		if kinds[ln] != "summary" {
			t.Fatalf("line %d should be a heading summary; segs=%#v", ln, segs)
		}
	}
	if kinds[9] != "lines" {
		t.Fatalf("match line 9 should be a real line; segs=%#v", segs)
	}
	// The h2 summary should carry the actual heading text.
	for _, s := range segs {
		if s.Start == 5 && s.Text != "## Section" {
			t.Fatalf("h2 summary text = %q, want '## Section'", s.Text)
		}
	}
}

// A sibling section's heading must not be shown for a match in another section.
func TestMarkdownSegmentsExcludesSiblingSection(t *testing.T) {
	content := "# Doc\n\n## Alpha\n\naaa\n\n## Beta\n\nneedle\n"
	// lines: 1 Doc, 3 ## Alpha, 5 aaa, 7 ## Beta, 9 needle
	segs := buildMarkdownSegments(content, map[int]bool{9: true}, 20)
	kinds := segmentKinds(segs)
	if _, ok := kinds[3]; ok {
		t.Fatalf("sibling '## Alpha' (line 3) must not appear; segs=%#v", segs)
	}
	if kinds[1] != "summary" || kinds[7] != "summary" || kinds[9] != "lines" {
		t.Fatalf("expected Doc+Beta breadcrumb then the line; segs=%#v", segs)
	}
}

// Ancestor headings shared by multiple matches appear once, not per match.
func TestMarkdownSegmentsDedupesSharedHeadings(t *testing.T) {
	content := "# Doc\n\n## Section\n\nfirst needle\n\nsecond needle\n"
	// lines: 1 Doc, 3 ## Section, 5 first needle, 7 second needle
	segs := buildMarkdownSegments(content, map[int]bool{5: true, 7: true}, 20)
	headingCount := 0
	for _, s := range segs {
		if s.Start == 3 {
			headingCount++
		}
	}
	if headingCount != 1 {
		t.Fatalf("shared '## Section' heading should appear once, got %d; segs=%#v", headingCount, segs)
	}
}

// A match in the preamble (before any heading) is shown with no breadcrumb.
func TestMarkdownSegmentsPreamble(t *testing.T) {
	content := "intro needle\n\n# Later\n"
	segs := buildMarkdownSegments(content, map[int]bool{1: true}, 20)
	if len(segs) != 1 || segs[0].Kind != "lines" || segs[0].Start != 1 {
		t.Fatalf("preamble match should be a single line segment; segs=%#v", segs)
	}
}
