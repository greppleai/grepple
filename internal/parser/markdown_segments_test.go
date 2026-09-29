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
		if got := NewParser().LanguageFor(ext); got != "markdown" {
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
	segs := BuildSegments(content, "markdown", map[int]bool{9: true})
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
	segs := BuildSegments(content, "markdown", map[int]bool{9: true})
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
	segs := BuildSegments(content, "markdown", map[int]bool{5: true, 7: true})
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
	segs := BuildSegments(content, "markdown", map[int]bool{1: true})
	if len(segs) != 1 || segs[0].Kind != "lines" || segs[0].Start != 1 {
		t.Fatalf("preamble match should be a single line segment; segs=%#v", segs)
	}
}

func markdownHeadingsFromDocument(t *testing.T, content string) []mdHeading {
	t.Helper()
	doc, err := parseDocument("markdown", content)
	if err != nil {
		t.Fatalf("parseDocument: %v", err)
	}
	defer doc.Close()
	var heads []mdHeading
	doc.mu.RLock()
	defer doc.mu.RUnlock()
	collectMarkdownHeadings(doc.tree.RootNode(), &heads)
	return heads
}

func TestMarkdownHeadingsTreeSitter(t *testing.T) {
	content := "# Title #\n\nintro\n\n```\n# not heading\n```\n\n    # indented code\n\nSetext\n======\n\n## Sub  heading\n\nAlt\n---\n\n#hashtag\n"
	heads := markdownHeadingsFromDocument(t, content)
	want := []mdHeading{{1, 1, "Title"}, {1, 11, "Setext"}, {2, 14, "Sub heading"}, {2, 16, "Alt"}}
	if len(heads) != len(want) {
		t.Fatalf("headings=%#v want %#v", heads, want)
	}
	for i := range want {
		if heads[i] != want[i] {
			t.Fatalf("heading %d=%#v want %#v", i, heads[i], want[i])
		}
	}
}

func TestMarkdownHeadingsExcludeListQuoteAndCode(t *testing.T) {
	content := "# Top\n\n- # list heading\n\n> # quote heading\n>\n> Quoted\n> ------\n\n- item\n\n  Nested\n  ======\n\n## Real\n"
	outline := OutlineFile("doc.md", content)
	if outline.Language != "markdown" || len(outline.Symbols) != 1 {
		t.Fatalf("outline=%#v", outline)
	}
	top := outline.Symbols[0]
	if top.Name != "Top" || len(top.Children) != 1 || top.Children[0].Name != "Real" {
		t.Fatalf("scoped headings wrong: %#v", top)
	}
}

func TestMarkdownOutlineNestingAndSegments(t *testing.T) {
	content := "# A\n\n## B\n\ntext\n\nB2\n--\n\n# C\n"
	syms := OutlineFile("a.md", content).Symbols
	if len(syms) != 2 || len(syms[0].Children) != 2 || syms[0].Children[1].Name != "B2" || syms[0].End != 9 {
		t.Fatalf("outline=%#v", syms)
	}
	segs, status := BuildSegmentsWithStatus(content, "markdown", map[int]bool{5: true})
	kinds := segmentKinds(segs)
	if status != SegmentBuildStructured || kinds[1] != "summary" || kinds[3] != "summary" || kinds[5] != "lines" {
		t.Fatalf("segments=%#v status=%s", kinds, status)
	}
}

func TestMarkdownDocumentIntegration(t *testing.T) {
	content := "# A\n\n## B\n\nneedle\n"
	doc, err := parseDocument("markdown", content)
	if err != nil {
		t.Fatalf("parseDocument: %v", err)
	}
	defer doc.Close()
	if doc.Language() != "markdown" || doc.Root().Kind() != "document" {
		t.Fatalf("language=%q root=%q", doc.Language(), doc.Root().Kind())
	}
	if out := outlineFromDocument("a.md", doc); len(out.Symbols) != 1 || out.Symbols[0].Children[0].Name != "B" {
		t.Fatalf("outline=%#v", out)
	}
	segs, status := buildSegmentsFromDocument(doc, map[int]bool{5: true})
	kinds := segmentKinds(segs)
	if status != SegmentBuildStructured || kinds[1] != "summary" || kinds[3] != "summary" || kinds[5] != "lines" {
		t.Fatalf("segments=%#v status=%s", kinds, status)
	}
}

func TestMarkdownRegistrationKeepsSwift(t *testing.T) {
	for _, id := range []string{"markdown", "swift"} {
		if adapterForLanguage(id) == nil {
			t.Fatalf("adapter %q not registered", id)
		}
	}
	if navigationAdapterForLanguage("markdown") != nil {
		t.Fatal("markdown must not have navigation")
	}
	if got := NewParser().LanguageFor("a.swift"); got != "swift" {
		t.Fatalf("swift language=%q", got)
	}
}

func TestMdATXTitle(t *testing.T) {
	for in, want := range map[string]string{
		"Title ##": "Title",
		"Title#":   "Title#",
		"##":       "",
		"C# ##  ":  "C#",
		"a   b":    "a b",
	} {
		if got := mdATXTitle(in); got != want {
			t.Errorf("mdATXTitle(%q)=%q want %q", in, got, want)
		}
	}
	if got := mdSetextTitle("Multi\n  line  title\n"); got != "Multi line title" {
		t.Errorf("mdSetextTitle=%q", got)
	}
}
