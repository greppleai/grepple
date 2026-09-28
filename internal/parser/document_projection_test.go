package parser

import (
	"reflect"
	"testing"
)

func TestDocumentStructuralProjectionsDoNotReparse(t *testing.T) {
	content := "package sample\n\n// run delegates.\nfunc run() { helper() }\nfunc helper() {}\n"
	before := parseInvocations.Load()
	document, err := NewParser().Parse("go", content)
	if err != nil {
		t.Fatal(err)
	}
	defer document.Close()
	afterParse := parseInvocations.Load()
	if afterParse != before+1 {
		t.Fatalf("parse invocations before=%d after=%d", before, afterParse)
	}

	start, end, ok := DeclarationRangeAtFromDocument(document, "source.go", 4)
	if !ok || start != 3 || end != 4 {
		t.Fatalf("declaration range=%d-%d ok=%v", start, end, ok)
	}
	segments, status := NewParser().Segments(document, map[int]bool{4: true})
	if status != SegmentBuildStructured {
		t.Fatalf("segment status=%q", status)
	}
	cold, coldStatus := BuildSegmentsWithStatus(content, "go", map[int]bool{4: true})
	if status != coldStatus || !reflect.DeepEqual(segments, cold) {
		t.Fatalf("document/cold segments differ:\n%#v\n%#v", segments, cold)
	}
	if got := parseInvocations.Load(); got != afterParse+1 {
		t.Fatalf("document projections reparsed source: after document=%d final=%d", afterParse, got)
	}
}
