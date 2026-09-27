package search

import (
	"testing"

	"github.com/greppleai/grepple/internal/parser"
)

func TestNavigationGraphProjectionsPreserveMemberAccessesForIncludedCallers(t *testing.T) {
	graph := parser.NavigationGraph{
		Declarations: []parser.NavigationDeclaration{
			{ID: "root", Name: "Root", Language: "go", Visibility: parser.NavigationVisibilityPublic},
			{ID: "hidden", Name: "Hidden", Language: "python", Visibility: parser.NavigationVisibilityNonPublic},
		},
		Fields: []parser.NavigationField{
			{OwnerType: "Root", Name: "state", Type: "State", Language: "go"},
			{OwnerType: "Hidden", Name: "state", Type: "State", Language: "python"},
		},
		Exports: []parser.NavigationExport{
			{Name: "Root", Language: "go"},
			{Name: "Hidden", Language: "python"},
		},
		TypeUsages: []parser.NavigationTypeUsage{
			{CallerID: "root", Type: "State", Language: "go"},
			{CallerID: "hidden", Type: "State", Language: "python"},
		},
		MemberAccesses: []parser.NavigationMemberAccess{
			{ID: "root-access", CallerID: "root", Member: "State", Language: "go"},
			{ID: "hidden-access", CallerID: "hidden", Member: "State", Language: "python"},
		},
	}
	filtered, err := FilterNavigationGraph(graph, NavigationGraphFilter{Languages: []string{"go"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered.MemberAccesses) != 1 || filtered.MemberAccesses[0].ID != "root-access" {
		t.Fatalf("filtered accesses=%#v", filtered.MemberAccesses)
	}
	if len(filtered.Fields) != 1 || filtered.Fields[0].OwnerType != "Root" {
		t.Fatalf("filtered fields=%#v", filtered.Fields)
	}
	if len(filtered.Exports) != 1 || filtered.Exports[0].Name != "Root" {
		t.Fatalf("filtered exports=%#v", filtered.Exports)
	}
	if len(filtered.Fields) != 1 || filtered.Fields[0].OwnerType != "Root" {
		t.Fatalf("filtered type usages=%#v", filtered.TypeUsages)
	}
	queried, err := QueryNavigationGraph(filtered, []string{"root"}, NavigationQueryCallees, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(queried.MemberAccesses) != 1 || queried.MemberAccesses[0].ID != "root-access" {
		t.Fatalf("queried accesses=%#v", queried.MemberAccesses)
	}
	if len(queried.TypeUsages) != 1 || queried.TypeUsages[0].CallerID != "root" {
		t.Fatalf("queried type usages=%#v", queried.TypeUsages)
	}
}
