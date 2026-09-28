package search

import (
	"github.com/greppleai/grepple/internal/navigation"
	"github.com/greppleai/grepple/internal/parser"
)

type NavigationResolutionCount = navigation.NavigationResolutionCount
type NavigationLanguageResolutionStats = navigation.NavigationLanguageResolutionStats
type NavigationResolutionStats = navigation.NavigationResolutionStats

func MeasureNavigationResolution(graph parser.NavigationGraph) NavigationResolutionStats {
	return navigation.NewGraphOperations().ResolutionStats(graph)
}
