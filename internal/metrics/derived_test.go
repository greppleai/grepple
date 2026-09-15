package metrics

import (
	"encoding/json"
	"testing"
)

func TestEditTransitionDetectsRevertShape(t *testing.T) {
	oldHash, newHash := editTransition("edit", json.RawMessage(`{"oldText":"before","newText":"after"}`))
	revertOld, revertNew := editTransition("edit", json.RawMessage(`{"oldText":"after","newText":"before"}`))
	if oldHash == "" || newHash == "" || oldHash != revertNew || newHash != revertOld {
		t.Fatalf("transition hashes do not identify reversal")
	}
}

func TestPathsRelated(t *testing.T) {
	if !pathsRelated([]string{"parser"}, []string{"parser/parser.go"}) {
		t.Fatal("directory evidence should relate to child read")
	}
	if pathsRelated([]string{"api"}, []string{"parser/parser.go"}) {
		t.Fatal("unrelated paths matched")
	}
}
