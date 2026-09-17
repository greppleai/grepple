package search

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/greppleai/grepple/parser"
)

func TestAtReturnsExactDocumentedGoCallable(t *testing.T) {
	directory := t.TempDir()
	path := writeGoFixture(t, directory, "helper.go", `package related

// helper returns a stable value.
func helper() string {
	return "value"
}

func other() {}
`)
	params := Params{At: fmt.Sprintf("%s:5-6", path)}
	match, err := At(params)
	if err != nil {
		t.Fatal(err)
	}
	if match.DisplayPath != filepath.ToSlash(path) && match.DisplayPath != path {
		t.Fatalf("unexpected path %q", match.DisplayPath)
	}
	if !match.CallableDeclaration {
		t.Fatal("expected callable declaration preflight")
	}
	if len(match.Segments) != 1 || match.Segments[0].Start != 3 || match.Segments[0].End != 6 {
		t.Fatalf("expected exact documented declaration, got %#v", match.Segments)
	}
}
func TestAtSupportsRelatedAndFollowedDeclarations(t *testing.T) {
	directory := t.TempDir()
	caller := writeGoFixture(t, directory, "caller.go", `package related
func run() string { return helper() }
`)
	writeGoFixture(t, directory, "helper.go", `package related
func helper() string { return "value" }
`)
	oldDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(directory); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldDirectory) })

	relatedBuildInvocations.Store(0)
	params := Params{At: fmt.Sprintf("%s:2", caller), Related: true, FollowRelated: 1}
	match, err := At(params)
	if err != nil {
		t.Fatal(err)
	}
	if builds := relatedBuildInvocations.Load(); builds != 1 {
		t.Fatalf("related graph builds=%d, want 1", builds)
	}
	point := findRelatedPoint(t, match.Related, "helper", "callee")
	if point.Preview == nil {
		t.Fatalf("expected followed helper declaration, got %#v", point)
	}
}

func TestAtLineRangesReturnExactEditableLines(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "sample.go")
	content := "package sample\n\nfunc first() {}\nfunc second() {}\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	match, err := At(Params{At: path + ":2-4", LineRanges: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range []int{2, 3, 4} {
		if !match.MatchLines[line] {
			t.Fatalf("missing exact line %d: %+v", line, match.MatchLines)
		}
	}
	if len(match.MatchLines) != 3 || len(match.Segments) != 0 {
		t.Fatalf("match=%+v", match)
	}
	if _, _, _, err := parseAtRange(path + ":4-2"); err == nil {
		t.Fatal("descending --at range succeeded")
	}
}

func TestAtSkipsRelatedGraphOutsideCallable(t *testing.T) {
	directory := t.TempDir()
	path := writeGoFixture(t, directory, "source.go", `package related

import "fmt"

func run() { fmt.Println("ok") }
`)
	for _, line := range []int{1, 3, 4} {
		relatedBuildInvocations.Store(0)
		match, err := At(Params{At: fmt.Sprintf("%s:%d", path, line), Root: directory, Related: true, FollowRelated: 2})
		if err != nil {
			t.Fatal(err)
		}
		if match.CallableDeclaration {
			t.Fatalf("line %d was classified as a callable declaration", line)
		}
		if len(match.Related) != 0 {
			t.Fatalf("line %d returned unexpected related points: %#v", line, match.Related)
		}
		if builds := relatedBuildInvocations.Load(); builds != 0 {
			t.Fatalf("line %d related graph builds=%d, want 0", line, builds)
		}
		if len(match.Segments) > 4 {
			t.Fatalf("line %d segments=%d, want at most 4", line, len(match.Segments))
		}
	}
}

func TestAtFromDocumentMatchesColdRelatedOutput(t *testing.T) {
	directory := t.TempDir()
	caller := writeGoFixture(t, directory, "caller.go", "package related\nfunc run() string { return helper() }\n")
	helper := writeGoFixture(t, directory, "helper.go", "package related\nfunc helper() string { return \"value\" }\n")
	t.Chdir(directory)
	callerDocument, err := parser.ParseDocument("go", "package related\nfunc run() string { return helper() }\n")
	if err != nil {
		t.Fatal(err)
	}
	defer callerDocument.Close()
	helperDocument, err := parser.ParseDocument("go", "package related\nfunc helper() string { return \"value\" }\n")
	if err != nil {
		t.Fatal(err)
	}
	defer helperDocument.Close()
	analysis, _ := BuildNavigationAnalysisFromDocuments([]NavigationDocumentSource{{Path: caller, Document: callerDocument}, {Path: helper, Document: helperDocument}}, NavigationBuildOptions{})
	params := Params{At: "caller.go:2", Root: directory, Related: true, FollowRelated: 1}
	cold, err := At(params)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := AtFromDocument(params, callerDocument, analysis)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cold, prepared) {
		t.Fatalf("cold/prepared navigation differs:\n%#v\n%#v", cold, prepared)
	}
}

func TestAtRejectsInvalidReferenceAndLine(t *testing.T) {
	if _, err := At(Params{At: "missing-line"}); err == nil {
		t.Fatal("expected malformed reference error")
	}
	directory := t.TempDir()
	path := writeGoFixture(t, directory, "small.go", "package related\n")
	if _, err := At(Params{At: fmt.Sprintf("%s:20", path)}); err == nil {
		t.Fatal("expected out-of-range line error")
	}
}
