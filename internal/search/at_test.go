package search

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/linerange"
	"github.com/greppleai/grepple/internal/parser"
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
	params := Params{At: fmt.Sprintf("%s:5", path)}
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
	params := Params{At: fmt.Sprintf("%s:2", caller), Related: true, FollowRelated: 2}
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

func TestAtExplicitRangeReturnsTheWholeRangeWithoutDeclarationExpansion(t *testing.T) {
	directory := t.TempDir()
	path := writeGoFixture(t, directory, "sample.go", "package sample\n\nfunc first() {}\nfunc second() {}\n")
	match, err := At(Params{At: path + ":1-4"})
	if err != nil {
		t.Fatal(err)
	}
	if match.CallableDeclaration {
		t.Fatal("explicit range was incorrectly treated as one callable declaration")
	}
	if len(match.Segments) != 1 || match.Segments[0].Kind != "lines" || match.Segments[0].Start != 1 || match.Segments[0].End != 4 {
		t.Fatalf("explicit range segments=%#v", match.Segments)
	}
	if len(match.MatchLines) != 4 {
		t.Fatalf("explicit range match lines=%v", match.MatchLines)
	}
	result := BuildResults([]FileMatch{*match}, 0, 0, true)[0]
	if len(result.Segments) != 1 || result.Segments[0].Text != "package sample\n\nfunc first() {}\nfunc second() {}" {
		t.Fatalf("explicit range result=%#v", result.Segments)
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

func TestAtClampsPartialEOFRangesAndRejectsFullMisses(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "sample.go")
	lines := []string{"package sample"}
	for line := 2; line <= 28; line++ {
		lines = append(lines, fmt.Sprintf("// line %d", line))
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		start, end, wantStart, wantEnd int
	}{
		{1, 30, 1, 28},
		{20, 40, 20, 28},
		{28, 40, 28, 28},
	} {
		assertPartialAtRange(t, path, test.start, test.end, test.wantStart, test.wantEnd)
	}
	for _, requested := range []string{"29-40", "50-60"} {
		assertFullAtRange(t, path, requested)
	}
}
func assertPartialAtRange(t *testing.T, path string, start, end, wantStart, wantEnd int) {
	t.Helper()
	match, err := At(Params{At: fmt.Sprintf("%s:%d-%d", path, start, end), LineRanges: true})
	if err != nil {
		t.Fatal(err)
	}
	if match.LineRange == nil || match.LineRange.Outcome != linerange.OutcomePartialMiss || match.LineRange.Warning == "" {
		t.Fatalf("range %d-%d metadata=%#v", start, end, match.LineRange)
	}
	if len(match.MatchLines) != wantEnd-wantStart+1 || !match.MatchLines[wantStart] || !match.MatchLines[wantEnd] {
		t.Fatalf("range %d-%d lines=%v", start, end, match.MatchLines)
	}
	result := BuildResults([]FileMatch{*match}, 0, 0, true)[0]
	if result.LineRange == nil || result.LineRange.ReturnedEnd != 28 {
		t.Fatalf("range metadata lost in result: %#v", result.LineRange)
	}
}

func assertFullAtRange(t *testing.T, path, requested string) {
	t.Helper()
	_, err := At(Params{At: path + ":" + requested, LineRanges: true})
	var outside *linerange.OutsideError
	if !errors.As(err, &outside) || outside.Result.Outcome != linerange.OutcomeFullMiss {
		t.Fatalf("range %s error=%v", requested, err)
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
	callerDocument, err := parser.NewParser().Parse("go", "package related\nfunc run() string { return helper() }\n")
	if err != nil {
		t.Fatal(err)
	}
	defer callerDocument.Close()
	helperDocument, err := parser.NewParser().Parse("go", "package related\nfunc helper() string { return \"value\" }\n")
	if err != nil {
		t.Fatal(err)
	}
	defer helperDocument.Close()
	analysis, _ := BuildNavigationAnalysisFromDocuments([]NavigationDocumentSource{{Path: caller, Document: callerDocument}, {Path: helper, Document: helperDocument}}, NavigationBuildOptions{})
	params := Params{At: "caller.go:2-20", Root: directory, Related: true, FollowRelated: 2}
	cold, err := At(params)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := AtFromDocument(params, callerDocument, analysis)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.LineRange == nil || prepared.LineRange.Outcome != linerange.OutcomePartialMiss || prepared.LineRange.ReturnedEnd != 2 {
		t.Fatalf("prepared range metadata=%#v", prepared.LineRange)
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
