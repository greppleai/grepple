package search

import (
	"github.com/greppleai/grepple/navigation"
	"github.com/greppleai/grepple/parser"
)

type NavigationResolutionCount = navigation.NavigationResolutionCount
type NavigationLanguageResolutionStats = navigation.NavigationLanguageResolutionStats
type NavigationResolutionStats = navigation.NavigationResolutionStats

func MeasureNavigationResolution(graph parser.NavigationGraph) NavigationResolutionStats {
	return navigation.MeasureNavigationResolution(graph)
}
