// Package search implements explicit and default search command dispatch.
package search

import (
	cliruntime "github.com/greppleai/grepple/internal/cliruntime"
)

// command owns the complete search workflow.
type command struct{ context cliruntime.Context }

// New constructs the search command from the common command context.
func New(context cliruntime.Context) cliruntime.Command { return &command{context: context} }

// Run executes a search using explicit or default-command arguments.
func (command *command) Run(args []string) error { return runSearch(command.context, args) }
