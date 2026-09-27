package search

import (
	"github.com/greppleai/grepple/internal/navigation"
	"github.com/greppleai/grepple/internal/parser"
)

type NavigationQueryDirection = navigation.NavigationQueryDirection

const (
	NavigationQueryCallers      = navigation.NavigationQueryCallers
	NavigationQueryCallees      = navigation.NavigationQueryCallees
	NavigationQueryDependencies = navigation.NavigationQueryDependencies
	NavigationQueryDependents   = navigation.NavigationQueryDependents
	NavigationQueryImpact       = navigation.NavigationQueryImpact
)

func QueryNavigationGraph(graph parser.NavigationGraph, rootIDs []string, direction NavigationQueryDirection, depth int) (parser.NavigationGraph, error) {
	return navigation.QueryNavigationGraph(graph, rootIDs, direction, depth)
}
