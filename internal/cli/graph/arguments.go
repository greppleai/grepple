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
	return ExecuteWithDaemon(application, values, false)
}

// ExecuteWithDaemon opts focused local graph commands into the shared report cache.
func ExecuteWithDaemon(application cliruntime.Context, values *Args, daemon bool) error {
	if values == nil {
		return fmt.Errorf("graph arguments are required")
	}
	if daemon && (values.Build != nil || values.Diff != nil) {
		return fmt.Errorf("--daemon supports graph resolve, callers, callees, impact, dependencies, and dependents; not build or diff")
	}
	switch {
	case values.Resolve != nil:
		return executeResolveWithDaemon(application, values.Resolve, daemon)
	case values.Diff != nil:
		return executeDiff(application, values.Diff)
	case values.Callers != nil:
		return executeQueryWithDaemon(application, search.NavigationQueryCallers, values.Callers, daemon)
	case values.Callees != nil:
		return executeQueryWithDaemon(application, search.NavigationQueryCallees, values.Callees, daemon)
	case values.Impact != nil:
		return executeQueryWithDaemon(application, search.NavigationQueryImpact, values.Impact, daemon)
	case values.Dependencies != nil:
		return executeQueryWithDaemon(application, search.NavigationQueryDependencies, values.Dependencies, daemon)
	case values.Dependents != nil:
		return executeQueryWithDaemon(application, search.NavigationQueryDependents, values.Dependents, daemon)
	case values.Build != nil:
		return executeBuild(application, values.Build)
	default:
		return fmt.Errorf("graph requires build, resolve, diff, callers, callees, impact, dependencies, or dependents")
	}
}
