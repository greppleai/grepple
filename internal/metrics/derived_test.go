package metrics

import (
	"encoding/json"
	"testing"
	"time"
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

func TestPathlessNavigationDoesNotConvertUnrelatedWork(t *testing.T) {
	run := Run{}
	calls := []*analyzedCall{
		{kind: callKind{navigation: true, grepple: true}, success: true},
		{kind: callKind{read: true}, paths: []string{"parser/parser.go"}},
		{kind: callKind{mutation: true}, paths: []string{"api/types.go"}},
	}
	deriveConversions(&run, calls)
	if run.SearchToRead != 0 || run.SearchToEdit != 0 || run.FirstEvidence != nil {
		t.Fatalf("pathless conversion = %#v", run)
	}
}

func TestRedundantReadsRequireSameRangeWithoutInterveningMutation(t *testing.T) {
	run := Run{GreppleModes: make(map[string]int)}
	analysis := newSegmentAnalysis(&run)
	calls := []contentBlock{
		{ID: "r1", Name: "functions.read", Arguments: json.RawMessage(`{"path":"main.go","offset":1,"limit":10}`)},
		{ID: "r2", Name: "functions.read", Arguments: json.RawMessage(`{"path":"main.go","offset":11,"limit":10}`)},
		{ID: "r3", Name: "functions.read", Arguments: json.RawMessage(`{"path":"main.go","offset":11,"limit":10}`)},
		{ID: "e1", Name: "functions.edit", Arguments: json.RawMessage(`{"path":"main.go","changes":[{"hash_range_inclusive":["abc","abc"],"content_lines":["changed"]}]}`)},
		{ID: "r4", Name: "functions.read", Arguments: json.RawMessage(`{"path":"main.go","offset":11,"limit":10}`)},
	}
	for _, call := range calls {
		analysis.recordCall(call, time.Time{})
	}
	if run.RedundantReads != 1 {
		t.Fatalf("redundant reads = %d, want 1", run.RedundantReads)
	}
}
