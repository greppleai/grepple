package render

import (
	"github.com/greppleai/grepple/api"
	"testing"
)

func TestRelatedTypeContextLabelPreservesAllRoles(t *testing.T) {
	artifact := &api.NavigationArtifactIdentity{Digest: "artifact-one"}
	segment := api.ResultSegment{Kind: "lines", Start: 1, End: 1, Text: "type Client struct{}"}
	points := []api.RelatedSymbol{
		{Name: "Client", Path: "client.go", Direction: "type", Role: "result", Start: 1, End: 1, Artifact: artifact, Segments: []api.ResultSegment{segment}},
		{Name: "Client", Path: "client.go", Direction: "type", Role: "parameter", Start: 1, End: 1, Artifact: artifact, Segments: []api.ResultSegment{segment}},
	}
	definitions := collectRelatedTypeDefinitions([]api.FileResult{{Related: points}})
	if len(definitions) != 1 {
		t.Fatalf("got %d definitions", len(definitions))
	}
	if label := relatedTypeContextLabel(definitions[0]); label != "remote parameter/result type Client" {
		t.Fatalf("unexpected label %q", label)
	}
}
