package render

import (
	"bytes"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/search"
	"github.com/greppleai/grepple/internal/wire"
)

func TestAnnotatedHumanRejectsEmbeddedRemoteNewlineBeforeRendering(t *testing.T) {
	results := []wire.FileResult{{Path: "ok.go", Matches: []wire.ResultMatch{{Line: 1, Text: "good"}}}, {Path: "remote.go", Matches: []wire.ResultMatch{{Line: 4, Text: "bad\ninjection", Nearest: testScopeSpan("func", 2, 5)}}}}
	for _, anchors := range []AnchorLookup{nil, {"ok.go": {1: "AAA"}, "remote.go": {4: "BBB"}}} {
		var output bytes.Buffer
		_, err := Render(Options{JSON: "off", LineOnly: true, Anchors: anchors, Params: search.Params{AnnotateScopes: true}}, results, &output)
		if err == nil || !strings.Contains(err.Error(), "embedded line break") || output.Len() != 0 {
			t.Fatalf("ambiguous remote row rendered: %q, %v", output.String(), err)
		}
	}
}
