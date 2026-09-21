// Package search implements explicit and default search command dispatch.
package search

import (
	"fmt"

	cliruntime "github.com/greppleai/grepple/internal/cli/runtime"
)

// Dependencies supplies the search execution engine.
type Dependencies struct {
	Execute func([]string) error
}

type command struct{ dependencies Dependencies }

// New constructs the search command.
func New(dependencies Dependencies) cliruntime.Command { return &command{dependencies: dependencies} }

// Run executes a search using explicit or default-command arguments.
func (command *command) Run(args []string) error {
	if command.dependencies.Execute == nil {
		return fmt.Errorf("search execution is unavailable")
	}
	return command.dependencies.Execute(args)
}
