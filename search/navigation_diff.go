package search

import (
	"github.com/greppleai/grepple/navigation"
	"github.com/greppleai/grepple/parser"
)

const NavigationDiffSchema = navigation.NavigationDiffSchema

type NavigationDeclarationDelta = navigation.NavigationDeclarationDelta
type NavigationCallDelta = navigation.NavigationCallDelta
type NavigationGraphDiff = navigation.NavigationGraphDiff

func DiffNavigationGraphs(before, after parser.NavigationGraph) NavigationGraphDiff {
	return navigation.DiffNavigationGraphs(before, after)
}
