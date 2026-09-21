// Package anchors implements anchor-provider command dispatch.
package anchors

import (
	"fmt"

	cliruntime "github.com/greppleai/grepple/internal/cli/runtime"
)

// Dependencies supplies settings/provider operations owned by the parent application.
type Dependencies struct {
	Help   func() error
	Doctor func([]string) error
	Setup  func([]string) error
}

type command struct{ dependencies Dependencies }

// New constructs the anchors command.
func New(dependencies Dependencies) cliruntime.Command { return &command{dependencies: dependencies} }

// Run dispatches anchor-provider operations.
func (command *command) Run(args []string) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		if command.dependencies.Help == nil {
			return fmt.Errorf("anchors help is unavailable")
		}
		return command.dependencies.Help()
	}
	switch args[0] {
	case "doctor":
		if command.dependencies.Doctor == nil {
			return fmt.Errorf("anchors doctor is unavailable")
		}
		return command.dependencies.Doctor(args[1:])
	case "setup":
		if command.dependencies.Setup == nil {
			return fmt.Errorf("anchors setup is unavailable")
		}
		return command.dependencies.Setup(args[1:])
	default:
		return fmt.Errorf("unknown anchors command %q; expected doctor or setup", args[0])
	}
}
