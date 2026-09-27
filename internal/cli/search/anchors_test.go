package search

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/wire"
)

func TestNoAnchorableResultsSkipProvider(t *testing.T) {
	options := &Options{Anchors: true}
	lookup, err := prepareAnchors(options, nil)
	if err != nil {
		t.Fatal(err)
	}
	if lookup == nil {
		t.Fatal("expected initialized empty anchor lookup")
	}
}

func TestAnchorSelectionIncludesRelatedTypeAppendixLines(t *testing.T) {
	directory := t.TempDir()
	t.Chdir(directory)
	content := "package sample\ntype Request struct {\n\tName string\n}\n"
	if err := os.WriteFile("request.go", []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	results := []wire.FileResult{{Path: "caller.go", Related: []wire.RelatedSymbol{{
		Name: "Request", Path: "request.go", Direction: "type", Start: 2, End: 4,
		Segments: []wire.ResultSegment{{Kind: "lines", Start: 2, End: 4, Text: "type Request struct {\n\tName string\n}"}},
	}}}}
	selections := collectAnchorSelections(&Options{}, results)
	if len(selections) != 1 || selections[0].displayPath != "request.go" {
		t.Fatalf("selections=%#v", selections)
	}
	if lines := sortedAnchorLines(selections[0].lines); !reflect.DeepEqual(lines, []int{2, 3, 4}) {
		t.Fatalf("lines=%v", lines)
	}
	file, err := anchorFile(selections[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(file.Path, "request.go") || file.DisplayPath != "request.go" {
		t.Fatalf("file=%#v", file)
	}
}
