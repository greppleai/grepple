package search

import (
	"github.com/greppleai/grepple/internal/navigation"
	"github.com/greppleai/grepple/internal/parser"
)

type NavigationGraphFilter = navigation.NavigationGraphFilter

func FilterNavigationGraph(graph parser.NavigationGraph, filter NavigationGraphFilter) (parser.NavigationGraph, error) {
	return navigation.NewGraphOperations().Filter(graph, filter)
}

func NormalizeNavigationGraphFilter(filter NavigationGraphFilter) (NavigationGraphFilter, error) {
	return navigation.NewGraphOperations().NormalizeFilter(filter)
}
