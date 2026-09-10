package search

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
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
	params := Params{At: fmt.Sprintf("%s:5-6", path), MaxSegments: 20}
	match, err := At(params)
	if err != nil {
		t.Fatal(err)
	}
	if match.DisplayPath != filepath.ToSlash(path) && match.DisplayPath != path {
		t.Fatalf("unexpected path %q", match.DisplayPath)
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

	params := Params{At: fmt.Sprintf("%s:2", caller), MaxSegments: 20, Related: true, FollowRelated: 1}
	match, err := At(params)
	if err != nil {
		t.Fatal(err)
	}
	point := findRelatedPoint(t, match.Related, "helper", "callee")
	if point.Preview == nil {
		t.Fatalf("expected followed helper declaration, got %#v", point)
	}
}

func TestAtRejectsInvalidReferenceAndLine(t *testing.T) {
	if _, err := At(Params{At: "missing-line", MaxSegments: 20}); err == nil {
		t.Fatal("expected malformed reference error")
	}
	directory := t.TempDir()
	path := writeGoFixture(t, directory, "small.go", "package related\n")
	if _, err := At(Params{At: fmt.Sprintf("%s:20", path), MaxSegments: 20}); err == nil {
		t.Fatal("expected out-of-range line error")
	}
}
