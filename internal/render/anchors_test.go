package render

import (
	"strings"
	"testing"

	"github.com/greppleai/grepple/api"
)

func TestSegmentRendererEmitsHashLineContentRows(t *testing.T) {
	var output strings.Builder
	result := api.FileResult{Path: "sample.go", Segments: []api.ResultSegment{{Kind: "lines", Start: 1, End: 2, Text: "first\nsecond"}}}
	_, err := Render(Options{JSON: "off", Anchors: AnchorLookup{"sample.go": {1: "abc", 2: "def"}}}, []api.FileResult{result}, &output, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rendered := output.String(); rendered != "sample.go\n\nabc│1│first\ndef│2│second\n" {
		t.Fatalf("segment output = %q", rendered)
	}
}
