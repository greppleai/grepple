package cli

import (
	graphcommand "github.com/greppleai/grepple/internal/cli/graph"
	"github.com/greppleai/grepple/search"
)

func graphDependencies() graphcommand.Dependencies {
	return graphcommand.Dependencies{Build: runGraphBuild, Resolve: runGraphResolve, Diff: runGraphDiff, Query: func(queryArgs []string) error {
		return runGraphQuery(search.NavigationQueryDirection(queryArgs[0]), queryArgs[1:])
	}}
}

func runGraph(args []string) error {
	return graphcommand.New(graphDependencies()).Run(args)
}
