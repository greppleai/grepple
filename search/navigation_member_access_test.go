package search

import (
	"testing"

	"github.com/greppleai/grepple/parser"
)

func TestNavigationGraphProjectionsPreserveMemberAccessesForIncludedCallers(t *testing.T) {
	graph := parser.NavigationGraph{
		Declarations: []parser.NavigationDeclaration{
			{ID: "root", Name: "Root", Language: "go", Visibility: parser.NavigationVisibilityPublic},
			{ID: "hidden", Name: "Hidden", Language: "python", Visibility: parser.NavigationVisibilityNonPublic},
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
	if len(filtered.TypeUsages) != 1 || filtered.TypeUsages[0].CallerID != "root" {
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
