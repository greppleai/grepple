package render

import (
	"github.com/greppleai/grepple/internal/wire"
	"testing"
)

func TestRelatedTypeContextLabelPreservesAllRoles(t *testing.T) {
	artifact := &wire.NavigationArtifactIdentity{Digest: "artifact-one"}
	segment := wire.ResultSegment{Kind: "lines", Start: 1, End: 1, Text: "type Client struct{}"}
	points := []wire.RelatedSymbol{
		{Name: "Client", Path: "client.go", Direction: "type", Role: "result", Start: 1, End: 1, Artifact: artifact, Segments: []wire.ResultSegment{segment}},
		{Name: "Client", Path: "client.go", Direction: "type", Role: "parameter", Start: 1, End: 1, Artifact: artifact, Segments: []wire.ResultSegment{segment}},
	}
	definitions := collectRelatedTypeDefinitions([]wire.FileResult{{Related: points}})
	if len(definitions) != 1 {
		t.Fatalf("got %d definitions", len(definitions))
	}
	if label := relatedTypeContextLabel(definitions[0]); label != "remote parameter/result type Client" {
		t.Fatalf("unexpected label %q", label)
	}
}
