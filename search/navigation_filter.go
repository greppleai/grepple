package search

import (
	"github.com/greppleai/grepple/navigation"
	"github.com/greppleai/grepple/parser"
)

type NavigationGraphFilter = navigation.NavigationGraphFilter

func FilterNavigationGraph(graph parser.NavigationGraph, filter NavigationGraphFilter) (parser.NavigationGraph, error) {
	return navigation.FilterNavigationGraph(graph, filter)
}

func NormalizeNavigationGraphFilter(filter NavigationGraphFilter) (NavigationGraphFilter, error) {
	return navigation.NormalizeNavigationGraphFilter(filter)
}
