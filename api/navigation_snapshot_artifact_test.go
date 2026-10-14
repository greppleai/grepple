package api

import (
	"github.com/greppleai/grepple/internal/parser"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestActiveSnapshotMigrationPreservesCandidateShapes(t *testing.T) {
	graph := parser.NavigationGraph{Declarations: []parser.NavigationDeclaration{{ID: "root", Name: "Root", Language: "go", Path: "root.go", Start: 1, End: 3}}, Calls: []parser.NavigationCall{{ID: "call", CallerID: "root", Name: "unknown", Language: "go", Path: "root.go", Line: 2, CandidateTargetIDs: []string{}, Confidence: "candidate"}}}
	store := NewNavigationSnapshotStore()
	path := filepath.Join(t.TempDir(), "active.gnav")
	key := strings.Repeat("c", 64)
	if err := store.Save(path, key, indexedNavigation{graph: graph, complete: true}, nil); err != nil {
		t.Fatal(err)
	}
	artifact, _, err := store.OpenArtifact(path, key)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(artifact.(indexedNavigation).graph, graph) {
		t.Fatal("active generation migration changed empty/native shapes")
	}
}
