package search

import (
	"os"
	"path/filepath"
	"testing"
)

func writeGoFile(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// TestSegmentsAreBuiltOnceInSearch verifies that only returned files receive
// parser output and result construction reuses that attached output.
func TestSegmentsAreBuiltOnceInSearch(t *testing.T) {
	dir := t.TempDir()
	writeGoFile(t, dir, "a.go", "package a\nfunc Target() {}\n// Target\n// Target\n")
	writeGoFile(t, dir, "b.go", "package b\nfunc Target() {}\n// Target\n")
	writeGoFile(t, dir, "c.go", "package c\nfunc Target() {}\n")
	t.Chdir(dir)

	p := Params{Query: "Target", Regex: true, MaxSegments: DefaultMaxSegments, Limit: 1, Root: dir}
	matches, err := Files(p, nil)
	if err != nil {
		t.Fatalf("Files: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 windowed match (Limit=1), got %d", len(matches))
	}
	if !matches[0].SegmentsReady || len(matches[0].Segments) == 0 {
		t.Fatal("expected parser segments to be attached during search")
	}

	// A sentinel proves BuildResults consumes the attached segments rather than
	// asking parser to reconstruct them.
	matches[0].Segments[0].Kind = "summary"
	matches[0].Segments[0].Text = "precomputed sentinel"
	results := BuildResults(matches, 0, 0, DefaultMaxSegments, true)
	if len(results) != 1 || len(results[0].Segments) == 0 {
		t.Fatalf("expected 1 result with segments, got %d results", len(results))
	}
	if results[0].Segments[0].Text != "precomputed sentinel" {
		t.Fatalf("BuildResults did not reuse precomputed segments: %#v", results[0].Segments)
	}
}

// TestSkipSegmentsAvoidsParserOutput verifies that segment-free output modes
// leave parser work disabled while retaining files and matching lines.
func TestSkipSegmentsAvoidsParserOutput(t *testing.T) {
	dir := t.TempDir()
	writeGoFile(t, dir, "a.go", "package a\nfunc Target() {}\n// Target\n")
	writeGoFile(t, dir, "b.go", "package b\nfunc Target() {}\n")
	t.Chdir(dir)

	p := Params{Query: "Target", Regex: true, MaxSegments: DefaultMaxSegments, Root: dir, SkipSegments: true}
	matches, err := Files(p, nil)
	if err != nil {
		t.Fatalf("Files: %v", err)
	}
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(matches))
	}
	for _, m := range matches {
		if len(m.MatchLines) == 0 {
			t.Fatalf("match %s has no match lines", m.DisplayPath)
		}
		if m.SegmentsReady || len(m.Segments) != 0 {
			t.Fatalf("did not expect parser segments for %s", m.DisplayPath)
		}
	}

	p.SkipSegments = false
	parsed, err := Files(p, nil)
	if err != nil {
		t.Fatalf("Files: %v", err)
	}
	for _, m := range parsed {
		if !m.SegmentsReady {
			t.Fatalf("expected parser segments for %s", m.DisplayPath)
		}
	}
}

func TestSkipAndLimitWindow(t *testing.T) {
	dir := t.TempDir()
	writeGoFile(t, dir, "a.go", "package a\n// Target Target Target Target\n// Target\n")
	writeGoFile(t, dir, "b.go", "package b\n// Target Target Target\n")
	writeGoFile(t, dir, "c.go", "package c\n// Target Target\n")
	writeGoFile(t, dir, "d.go", "package d\n// Target\n")
	t.Chdir(dir)

	base := Params{Query: "Target", Regex: true, MaxSegments: DefaultMaxSegments, Root: dir}
	all, err := Files(base, nil)
	if err != nil {
		t.Fatalf("Files: %v", err)
	}
	if len(all) != 4 {
		t.Fatalf("expected 4 matches, got %d", len(all))
	}
	paged := base
	paged.Skip = 1
	paged.Limit = 2
	page, err := Files(paged, nil)
	if err != nil {
		t.Fatalf("Files paged: %v", err)
	}
	if len(page) != 2 {
		t.Fatalf("expected 2 results for skip=1 limit=2, got %d", len(page))
	}
	if page[0].DisplayPath != all[1].DisplayPath || page[1].DisplayPath != all[2].DisplayPath {
		t.Fatalf("window mismatch: got [%s %s], want [%s %s]", page[0].DisplayPath, page[1].DisplayPath, all[1].DisplayPath, all[2].DisplayPath)
	}
}
