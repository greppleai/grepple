package cli

import (
	"os"

	graphcommand "github.com/greppleai/grepple/internal/cli/graph"
	"github.com/greppleai/grepple/search"
)

const navigationResolveSchema = graphcommand.NavigationResolveSchema

type graphResolveOutput = graphcommand.ResolveOutput

func graphDependencies() graphcommand.Dependencies {
	return graphcommand.Dependencies{Build: runGraphBuild, Diff: runGraphDiff, Stdout: os.Stdout, LoadOutput: buildNavigationGraphOutput, ActiveScopeFlags: appendActiveRepositoryScopeFlags, RequestExit: setExit, Query: func(queryArgs []string) error {
		return runGraphQuery(search.NavigationQueryDirection(queryArgs[0]), queryArgs[1:])
	}}
}

func runGraph(args []string) error {
	return graphcommand.New(graphDependencies()).Run(args)
}
