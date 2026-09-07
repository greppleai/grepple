package search

import "testing"

func TestSearchContentMatch(t *testing.T) {
	p := Params{Query: "hello", Regex: true, MaxSegments: DefaultMaxSegments}
	fm, err := Content(p, StdinPath, []byte("first\nhello world\nlast hello\n"))
	if err != nil {
		t.Fatal(err)
	}
	if fm == nil {
		t.Fatal("expected a match")
	}
	if fm.DisplayPath != StdinPath || fm.File != StdinPath {
		t.Fatalf("expected virtual path %q, got %q/%q", StdinPath, fm.DisplayPath, fm.File)
	}
	if !fm.MatchLines[2] || !fm.MatchLines[3] || len(fm.MatchLines) != 2 {
		t.Fatalf("expected matches on lines 2 and 3, got %v", fm.MatchLines)
	}
	// The virtual file has no extension: plain-text fallback language, but
	// segments are still built (unless SkipSegments is set).
	if fm.Language != "text" {
		t.Fatalf("expected language text, got %q", fm.Language)
	}
	if !fm.SegmentsReady {
		t.Fatal("expected segments to be built")
	}
}

func TestSearchContentNoMatch(t *testing.T) {
	p := Params{Query: "zzz", Regex: true, MaxSegments: DefaultMaxSegments}
	fm, err := Content(p, StdinPath, []byte("alpha\nbeta\n"))
	if err != nil || fm != nil {
		t.Fatalf("expected no match and no error, got %v, %v", fm, err)
	}
}

func TestSearchContentBinarySkipped(t *testing.T) {
	p := Params{Query: "hello", Regex: true, MaxSegments: DefaultMaxSegments}
	fm, err := Content(p, StdinPath, []byte{'h', 'e', 'l', 'l', 'o', 0, 'x'})
	if err != nil || fm != nil {
		t.Fatalf("expected binary content to be skipped, got %v, %v", fm, err)
	}
}

func TestSearchContentIgnoreCaseFixed(t *testing.T) {
	p := Params{Query: "HELLO", IgnoreCase: true, MaxSegments: DefaultMaxSegments}
	fm, err := Content(p, StdinPath, []byte("say hello\n"))
	if err != nil {
		t.Fatal(err)
	}
	if fm == nil || !fm.MatchLines[1] {
		t.Fatalf("expected case-insensitive fixed-string match on line 1, got %v", fm)
	}
}

func TestSearchContentInvalidRegex(t *testing.T) {
	p := Params{Query: "([", Regex: true, MaxSegments: DefaultMaxSegments}
	if _, err := Content(p, StdinPath, []byte("x\n")); err == nil {
		t.Fatal("expected an error for an invalid regex")
	}
}

func TestSearchContentSkipSegments(t *testing.T) {
	p := Params{Query: "hello", Regex: true, SkipSegments: true, MaxSegments: DefaultMaxSegments}
	fm, err := Content(p, StdinPath, []byte("hello\n"))
	if err != nil {
		t.Fatal(err)
	}
	if fm == nil {
		t.Fatal("expected a match")
	}
	if fm.SegmentsReady {
		t.Fatal("expected segments to be skipped when SkipSegments is set")
	}
}
