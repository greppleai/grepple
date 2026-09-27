// Package graph implements navigation graph command dispatch.
package graph

import (
	"fmt"

	cliruntime "github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/search"
)

// command owns navigation graph command operations.
type command struct{ context cliruntime.Context }

// New constructs the graph command from the common command context.
func New(context cliruntime.Context) cliruntime.Command { return &command{context: context} }

// Run dispatches graph subcommands without depending on the parent CLI package.
func (command *command) Run(args []string) error {
	if command.context == nil {
		return fmt.Errorf("graph command context is unavailable")
	}
	if len(args) > 0 {
		switch args[0] {
		case "resolve":
			return command.runResolve(args[1:])
		case "callers", "callees":
			return runQuery(command.context, search.NavigationQueryDirection(args[0]), args[1:])
		case "build", "diff", "impact", "dependencies", "dependents":
			return fmt.Errorf("graph %s has been removed; use graph callers or graph callees", args[0])
		}
	}
	return fmt.Errorf("graph requires resolve, callers, or callees")
}
