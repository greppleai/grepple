package search

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/parser"
)

func TestAtVerifiedScopeIsExactAndCurrent(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "sample.go")
	original := "package sample\nfunc outer() {\n if true {\n  println(1)\n }\n}\n"
	if err := os.WriteFile(file, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, selector := range []parser.ScopeSelector{{Kind: "if", StartLine: 3, EndLine: 5, Exact: true}, {Kind: "if", StartLine: 4, EndLine: 4}, {Kind: "func", StartLine: 2, EndLine: 6, Exact: true}} {
		match, err := At(Params{At: file, Root: root, AtScope: &selector, Related: true})
		if err != nil {
			t.Fatalf("selector=%+v: %v", selector, err)
		}
		start, end := selector.StartLine, selector.EndLine
		if !selector.Exact {
			start, end = 3, 5
		}
		if match.CallableDeclaration || match.LineRange != nil || len(match.MatchLines) != end-start+1 || len(match.Segments) != 1 || match.Segments[0].Start != start || match.Segments[0].End != end {
			t.Fatalf("selector=%+v match=%+v", selector, match)
		}
	}
	stale := parser.ScopeSelector{Kind: "if", StartLine: 3, EndLine: 5, Exact: true}
	if err := os.WriteFile(file, []byte("package sample\nfunc outer() {\n println(1)\n}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := At(Params{At: file, Root: root, AtScope: &stale}); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("stale scope error=%v", err)
	}
	if _, err := At(Params{At: filepath.Join(root, "outside.go"), Root: filepath.Join(root, "child"), AtScope: &stale}); err == nil || !strings.Contains(err.Error(), "outside") {
		t.Fatalf("root error=%v", err)
	}
}

func TestAtVerifiedScopeDocumentUsesCallerOwnedSource(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "sample.go")
	original := "package sample\nfunc outer() {\n if true {\n  println(1)\n }\n}\n"
	if err := os.WriteFile(file, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	doc, err := parser.NewParser().Parse("go", original)
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()
	selector := parser.ScopeSelector{Kind: "if", StartLine: 3, EndLine: 5, Exact: true}
	if err := os.WriteFile(file, []byte("package sample\nfunc outer() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	match, err := AtFromDocument(Params{At: file, Root: root, AtScope: &selector}, doc, nil)
	if err != nil || len(match.MatchLines) != 3 || match.Content != original {
		t.Fatalf("document selection=%+v error=%v", match, err)
	}
	if _, err := At(Params{At: file, Root: root, AtScope: &selector}); err == nil {
		t.Fatal("disk selection used stale document bounds")
	}
}

func TestAtVerifiedScopeSingleLineAndWireGuard(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "single.go")
	if err := os.WriteFile(file, []byte("package sample\nfunc f() { println(1) }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	selector := parser.ScopeSelector{Kind: "func", StartLine: 2, EndLine: 2, Exact: true}
	match, err := At(Params{At: file, Root: root, AtScope: &selector, Related: true})
	if err != nil || match.CallableDeclaration || len(match.Segments) != 1 || match.Segments[0].Start != 2 || match.Segments[0].End != 2 {
		t.Fatalf("single line=%+v error=%v", match, err)
	}
	request := RequestFromParams(Params{At: file, AtScope: &selector})
	if request.At != "" || request.Query != nil {
		t.Fatalf("selector leaked to wire: %+v", request)
	}
	if _, err := ResolveRequest(request); err == nil {
		t.Fatal("empty remote request unexpectedly accepted")
	}
	if _, err := At(Params{At: file, Root: root, AtScope: &parser.ScopeSelector{Kind: "func", StartLine: 0, EndLine: 2, Exact: true}}); err == nil {
		t.Fatal("invalid typed selector accepted")
	}
}

func TestAtVerifiedScopeRejectsUnsupportedBinaryAndMalformed(t *testing.T) {
	root := t.TempDir()
	selector := parser.ScopeSelector{Kind: "func", StartLine: 1, EndLine: 1, Exact: true}
	for _, tc := range []struct{ name, content, diagnostic string }{
		{"notes.txt", "func run() {} @func:1-1\n", "unsupported"},
		{"binary.go", "\x00func run() {}", "binary"},
		{"broken.go", "func run( {\n", "no verified"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file := filepath.Join(root, tc.name)
			if err := os.WriteFile(file, []byte(tc.content), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := At(Params{At: file, Root: root, AtScope: &selector}); err == nil || !strings.Contains(err.Error(), tc.diagnostic) {
				t.Fatalf("unsupported source error=%v, want %s", err, tc.diagnostic)
			}
		})
	}
}
