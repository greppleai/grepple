package parser

import (
	"reflect"
	"testing"
)

func TestParserProjectsOneOwnedDocumentWithoutReparsing(t *testing.T) {
	t.Setenv(NavigationCacheDirectoryEnv, "")
	service := NewParser()
	before := parseInvocations.Load()
	document, err := service.Parse("go", "package sample\nfunc target() {}\n")
	if err != nil {
		t.Fatal(err)
	}
	defer document.Close()
	afterParse := parseInvocations.Load()
	if afterParse != before+1 {
		t.Fatalf("parse invocations before=%d after=%d", before, afterParse)
	}
	graph := service.NavigationGraph(document, "sample.go")
	again := service.NavigationGraph(document, "sample.go")
	outline := service.Outline(document, "sample.go")
	segments, status := service.Segments(document, map[int]bool{2: true})
	if len(graph.Declarations) == 0 || !reflect.DeepEqual(graph, again) {
		t.Fatalf("graph=%+v repeat=%+v", graph, again)
	}
	if len(outline.Symbols) == 0 || len(segments) == 0 || status != SegmentBuildStructured {
		t.Fatalf("outline=%+v segments=%+v status=%s", outline, segments, status)
	}
	if got := parseInvocations.Load(); got != afterParse {
		t.Fatalf("projections reparsed source: after parse=%d after projections=%d", afterParse, got)
	}
}

func TestParserHandlesNilDocument(t *testing.T) {
	service := NewParser()
	if graph := service.NavigationGraph(nil, "sample.go"); len(graph.Declarations) != 0 {
		t.Fatalf("nil document graph=%+v", graph)
	}
	if outline := service.Outline(nil, "sample.go"); len(outline.Symbols) != 0 {
		t.Fatalf("nil document outline=%+v", outline)
	}
	if _, status := service.Segments(nil, map[int]bool{1: true}); status != SegmentBuildFailed {
		t.Fatalf("nil document segments status=%s", status)
	}
}
