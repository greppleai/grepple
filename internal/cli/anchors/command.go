// Package anchors implements anchor-provider command dispatch.
package anchors

import (
	"fmt"

	cliruntime "github.com/greppleai/grepple/internal/cliruntime"
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
		help := command.dependencies.Help
		if help == nil {
			help = WriteHelp
		}
		return help()
	}
	switch args[0] {
	case "doctor":
		doctor := command.dependencies.Doctor
		if doctor == nil {
			doctor = RunDoctor
		}
		return doctor(args[1:])
	case "setup":
		setup := command.dependencies.Setup
		if setup == nil {
			setup = RunSetup
		}
		return setup(args[1:])
	default:
		return fmt.Errorf("unknown anchors command %q; expected doctor or setup", args[0])
	}
}
