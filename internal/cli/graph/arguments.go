package graph

import (
	"fmt"

	cliruntime "github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/search"
)

type BuildArgs = graphArgs
type DiffArgs = graphDiffArgs
type ResolveArgs = resolveArgs
type QueryArgs = graphQueryArgs

type Args struct {
	Resolve *ResolveArgs `arg:"subcommand:resolve"`
	Callers *QueryArgs   `arg:"subcommand:callers"`
	Callees *QueryArgs   `arg:"subcommand:callees"`
}

func Execute(application cliruntime.Context, values *Args) error {
	return ExecuteWithDaemon(application, values, false)
}

// ExecuteWithDaemon opts focused local graph commands into the shared report cache.
func ExecuteWithDaemon(application cliruntime.Context, values *Args, daemon bool) error {
	if values == nil {
		return fmt.Errorf("graph arguments are required")
	}
	switch {
	case values.Resolve != nil:
		return executeResolveWithDaemon(application, values.Resolve, daemon)
	case values.Callers != nil:
		return executeQueryWithDaemon(application, search.NavigationQueryCallers, values.Callers, daemon)
	case values.Callees != nil:
		return executeQueryWithDaemon(application, search.NavigationQueryCallees, values.Callees, daemon)
	default:
		return fmt.Errorf("graph requires resolve, callers, or callees")
	}
}
