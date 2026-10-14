package parser

import (
	"reflect"
	"testing"
)

func TestDeclarationArtifactOmitsLargeNonDeclarationGraph(t *testing.T) {
	graph := NavigationGraph{Declarations: []NavigationDeclaration{{Name: "Example", Path: "example.go", Start: 1, End: 2}}, TypeDeclarations: []NavigationTypeDeclaration{{Name: "ExampleType", Path: "example.go", Start: 1, End: 2}}, Calls: make([]NavigationCall, 10000)}
	content, err := MarshalNavigationFactArtifact(NavigationFactArtifact{Digest: "test", Graph: graph})
	if err != nil {
		t.Fatal(err)
	}
	projected, err := UnmarshalNavigationDeclarationArtifact(content)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(projected.Graph.Declarations, graph.Declarations) || !reflect.DeepEqual(projected.Graph.TypeDeclarations, graph.TypeDeclarations) || len(projected.Graph.Calls) != 0 {
		t.Fatal("declaration projection changed evidence or decoded calls")
	}
	content[len(content)/2] ^= 1
	if _, err := UnmarshalNavigationDeclarationArtifact(content); err == nil {
		t.Fatal("tampered artifact accepted")
	}
}
