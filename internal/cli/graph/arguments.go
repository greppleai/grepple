package graph

import (
	"fmt"

	cliruntime "github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/search"
)

type BuildArgs = graphArgs
type ResolveArgs = resolveArgs
type DiffArgs = graphDiffArgs
type QueryArgs = graphQueryArgs

type Args struct {
	Build        *BuildArgs   `arg:"subcommand:build"`
	Resolve      *ResolveArgs `arg:"subcommand:resolve"`
	Diff         *DiffArgs    `arg:"subcommand:diff"`
	Callers      *QueryArgs   `arg:"subcommand:callers"`
	Callees      *QueryArgs   `arg:"subcommand:callees"`
	Impact       *QueryArgs   `arg:"subcommand:impact"`
	Dependencies *QueryArgs   `arg:"subcommand:dependencies"`
	Dependents   *QueryArgs   `arg:"subcommand:dependents"`
}

func Execute(application cliruntime.Context, values *Args) error {
	if values == nil {
		return fmt.Errorf("graph arguments are required")
	}
	switch {
	case values.Resolve != nil:
		return executeResolve(application, values.Resolve)
	case values.Diff != nil:
		return executeDiff(application, values.Diff)
	case values.Callers != nil:
		return executeQuery(application, search.NavigationQueryCallers, values.Callers)
	case values.Callees != nil:
		return executeQuery(application, search.NavigationQueryCallees, values.Callees)
	case values.Impact != nil:
		return executeQuery(application, search.NavigationQueryImpact, values.Impact)
	case values.Dependencies != nil:
		return executeQuery(application, search.NavigationQueryDependencies, values.Dependencies)
	case values.Dependents != nil:
		return executeQuery(application, search.NavigationQueryDependents, values.Dependents)
	case values.Build != nil:
		return executeBuild(application, values.Build)
	default:
		return fmt.Errorf("graph requires build, resolve, diff, callers, callees, impact, dependencies, or dependents")
	}
}
