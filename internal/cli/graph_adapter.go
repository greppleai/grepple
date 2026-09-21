package cli

import (
	graphcommand "github.com/greppleai/grepple/internal/cli/graph"
	"github.com/greppleai/grepple/search"
)

func runGraph(args []string) error {
	return graphcommand.Run(args, graphcommand.Dependencies{Build: runGraphBuild, Resolve: runGraphResolve, Diff: runGraphDiff, Query: func(queryArgs []string) error {
		return runGraphQuery(search.NavigationQueryDirection(queryArgs[0]), queryArgs[1:])
	}})
}
