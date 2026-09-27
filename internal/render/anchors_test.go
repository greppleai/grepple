package render

import (
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/wire"
)

func TestSegmentRendererEmitsHashLineContentRows(t *testing.T) {
	var output strings.Builder
	result := wire.FileResult{Path: "sample.go", Segments: []wire.ResultSegment{{Kind: "lines", Start: 1, End: 2, Text: "first\nsecond"}}}
	_, err := Render(Options{JSON: "off", Anchors: AnchorLookup{"sample.go": {1: "abc", 2: "def"}}}, []wire.FileResult{result}, &output, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rendered := output.String(); rendered != "sample.go\n\nabc│1│first\ndef│2│second\n" {
		t.Fatalf("segment output = %q", rendered)
	}
}
