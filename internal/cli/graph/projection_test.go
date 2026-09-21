package graph

import (
	"reflect"
	"testing"

	"github.com/greppleai/grepple/parser"
	"github.com/greppleai/grepple/search"
)

func TestSourcePathsKeepsNavigationLanguages(t *testing.T) {
	paths := SourcePaths([]string{"service.go", "notes.txt", "worker.py"})
	if want := []string{"service.go", "worker.py"}; !reflect.DeepEqual(paths, want) {
		t.Fatalf("SourcePaths = %v, want %v", paths, want)
	}
}

func TestOutputFromPartsNormalizesRequiredCollections(t *testing.T) {
	truncation := &Truncation{Reason: "max_files", Limit: 1, Skipped: 2}
	output := OutputFromParts([]string{"service.go"}, 4, 1, truncation, parser.NavigationGraph{}, search.NavigationSourceStats{Attempted: 1, Parsed: 1})
	if output.Schema != navigationGraphSchema || output.Files != 1 {
		t.Fatalf("output identity = %#v", output)
	}
	if output.Declarations == nil || output.Calls == nil {
		t.Fatalf("required collections must be non-nil: %#v", output)
	}
	if output.Sources.Discovered != 4 || output.Sources.Selected != 1 || output.Sources.Skipped != 1 {
		t.Fatalf("sources = %#v", output.Sources)
	}
	if output.Truncation != truncation {
		t.Fatalf("truncation = %#v", output.Truncation)
	}
}
