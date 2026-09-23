package navigation

import (
	"testing"

	"github.com/greppleai/grepple/parser"
)

func TestDiffNavigationGraphsClassifiesSemanticChangesAndIgnoresLineShifts(t *testing.T) {
	before := parser.NavigationGraph{
		Declarations: []parser.NavigationDeclaration{
			{ID: "run-old", Name: "Run", Kind: "func", Language: "go", Path: "old/run.go", PackageID: "example/app", Visibility: parser.NavigationVisibilityPublic, Start: 2, End: 4},
			{ID: "helper-old", Name: "helper", Kind: "func", Language: "go", Path: "helper.go", PackageID: "example/app", Visibility: parser.NavigationVisibilityNonPublic, Start: 6, End: 6},
			{ID: "removed", Name: "removed", Kind: "func", Language: "go", Path: "old.go", PackageID: "example/app", Visibility: parser.NavigationVisibilityNonPublic, Start: 8, End: 8},
		},
		Calls: []parser.NavigationCall{{ID: "call-old", CallerID: "run-old", TargetID: "helper-old", Name: "helper", Display: "helper", Confidence: "exact", Language: "go", Line: 3}},
	}
	after := parser.NavigationGraph{
		Declarations: []parser.NavigationDeclaration{
			{ID: "run-new", Name: "Run", Kind: "func", Language: "go", Path: "new/run.go", PackageID: "example/app", Visibility: parser.NavigationVisibilityPublic, ResultType: "error", Start: 20, End: 23},
			{ID: "helper-new", Name: "helper", Kind: "func", Language: "go", Path: "helper.go", PackageID: "example/app", Visibility: parser.NavigationVisibilityNonPublic, Start: 30, End: 30},
			{ID: "added", Name: "added", Kind: "func", Language: "go", Path: "new.go", PackageID: "example/app", Visibility: parser.NavigationVisibilityNonPublic, Start: 40, End: 40},
		},
		Calls: []parser.NavigationCall{{ID: "call-new", CallerID: "run-new", TargetID: "added", Name: "helper", Display: "helper", Confidence: "context-resolved", Language: "go", Line: 21}},
	}
	diff := DiffNavigationGraphs(before, after)
	if diff.Schema != NavigationDiffSchema || len(diff.AddedDeclarations) != 1 || len(diff.RemovedDeclarations) != 1 {
		t.Fatalf("declaration additions/removals=%#v", diff)
	}
	if len(diff.MovedDeclarations) != 1 || diff.MovedDeclarations[0].Before.Name != "Run" {
		t.Fatalf("moves=%#v", diff.MovedDeclarations)
	}
	if len(diff.ChangedDeclarations) != 1 || diff.ChangedDeclarations[0].After.ResultType != "error" {
		t.Fatalf("changes=%#v", diff.ChangedDeclarations)
	}
	if len(diff.ChangedCalls) != 1 || len(diff.AddedCalls) != 0 || len(diff.RemovedCalls) != 0 {
		t.Fatalf("call changes=%#v added=%#v removed=%#v", diff.ChangedCalls, diff.AddedCalls, diff.RemovedCalls)
	}
}

func TestDiffNavigationGraphsIgnoresPositionOnlyChanges(t *testing.T) {
	before := parser.NavigationGraph{Declarations: []parser.NavigationDeclaration{{ID: "old", Name: "Run", Kind: "func", Language: "go", Path: "run.go", Start: 1, End: 2}}}
	after := parser.NavigationGraph{Declarations: []parser.NavigationDeclaration{{ID: "new", Name: "Run", Kind: "func", Language: "go", Path: "run.go", Start: 10, End: 11}}}
	diff := DiffNavigationGraphs(before, after)
	if len(diff.AddedDeclarations)+len(diff.RemovedDeclarations)+len(diff.MovedDeclarations)+len(diff.ChangedDeclarations) != 0 {
		t.Fatalf("position-only diff=%#v", diff)
	}
}

func TestDiffNavigationGraphsTreatsRustModuleScopeAsSemanticIdentity(t *testing.T) {
	before := parser.NavigationGraph{Declarations: []parser.NavigationDeclaration{{ID: "old", Name: "run", Kind: "function", Language: "rust", Path: "src/lib.rs", Scope: "first", Start: 1, End: 1}}}
	after := parser.NavigationGraph{Declarations: []parser.NavigationDeclaration{{ID: "new", Name: "run", Kind: "function", Language: "rust", Path: "src/lib.rs", Scope: "second", Start: 1, End: 1}}}
	diff := DiffNavigationGraphs(before, after)
	if len(diff.AddedDeclarations) != 1 || len(diff.RemovedDeclarations) != 1 || len(diff.MovedDeclarations) != 0 {
		t.Fatalf("scoped Rust diff=%#v", diff)
	}
}

func TestDiffNavigationGraphsReportsRestrictedRustVisibilityChanges(t *testing.T) {
	before := parser.NavigationGraph{Declarations: []parser.NavigationDeclaration{{ID: "old", Name: "run", Kind: "function", Language: "rust", Path: "src/lib.rs", Scope: "model", Visibility: parser.NavigationVisibilityNonPublic, VisibilityDetail: "pub(super)", Start: 1, End: 1}}}
	after := parser.NavigationGraph{Declarations: []parser.NavigationDeclaration{{ID: "new", Name: "run", Kind: "function", Language: "rust", Path: "src/lib.rs", Scope: "model", Visibility: parser.NavigationVisibilityNonPublic, VisibilityDetail: "pub(in crate::model)", Start: 1, End: 1}}}
	diff := DiffNavigationGraphs(before, after)
	if len(diff.ChangedDeclarations) != 1 || diff.ChangedDeclarations[0].After.VisibilityDetail != "pub(in crate::model)" {
		t.Fatalf("restricted visibility diff=%#v", diff)
	}
}
