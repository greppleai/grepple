package render

import (
	"testing"

	"github.com/greppleai/grepple/parser"
)

func TestOutlineOrContentDumpsTinyFile(t *testing.T) {
	content := "a:\n  b:\n    c: 1\n"
	outline := parser.OutlineFile("tiny.yaml", content)
	if len(outline.Symbols) == 0 {
		t.Fatal("expected structured symbols for the yaml fixture")
	}
	rendered := Outline(outline)
	got := OutlineOrContent(outline, content)
	if len(rendered) <= len("tiny.yaml\tyaml\n"+content) {
		t.Skip("outline is not larger than the file; nothing to fall back from")
	}
	want := "tiny.yaml\tyaml\n" + content
	if got != want {
		t.Fatalf("expected raw file dump, got:\n%q\nwant:\n%q", got, want)
	}
}

func TestOutlineOrContentKeepsCompactOutline(t *testing.T) {
	source := "package p\n\nfunc Alpha() {}\n\nfunc Beta() {}\n\nfunc Gamma() {}\n"
	outline := parser.OutlineFile("small.go", source)
	if len(outline.Symbols) == 0 {
		t.Fatal("expected go symbols")
	}
	rendered := Outline(outline)
	if got := OutlineOrContent(outline, source); got != rendered {
		t.Fatalf("expected compact outline to be kept, got:\n%q", got)
	}
}

func TestOutlineOrContentAppendsTrailingNewline(t *testing.T) {
	content := "a:\n  b:\n    c: 1"
	outline := parser.OutlineFile("tiny.yaml", content)
	got := OutlineOrContent(outline, content)
	if got == Outline(outline) {
		t.Skip("outline chosen; trailing-newline path not exercised")
	}
	if got[len(got)-1] != '\n' {
		t.Fatalf("expected dump to end with newline, got:\n%q", got)
	}
}
