// Package anchors implements anchor-provider command dispatch.
package anchors

import (
	"fmt"

	cliruntime "github.com/greppleai/grepple/internal/cliruntime"
)

// Args contains anchor-provider subcommands.
type Args struct {
	Doctor *DoctorArgs `arg:"subcommand:doctor"`
	Setup  *SetupArgs  `arg:"subcommand:setup"`
}

type dependencies struct {
	Help   func() error
	Doctor func([]string) error
	Setup  func([]string) error
}

type command struct {
	context      cliruntime.Context
	dependencies dependencies
}

// New constructs the anchors command from the common command context.
func New(context cliruntime.Context) cliruntime.Command { return &command{context: context} }

func newWithDependencies(dependencies dependencies) cliruntime.Command {
	return &command{dependencies: dependencies}
}

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

// Execute applies application-parsed anchor-provider arguments.
func Execute(_ cliruntime.Context, values *Args) error {
	if values == nil {
		return fmt.Errorf("anchors requires doctor or setup")
	}
	switch {
	case values.Doctor != nil:
		return executeDoctor(values.Doctor)
	case values.Setup != nil:
		return executeSetup(values.Setup)
	default:
		return fmt.Errorf("anchors requires doctor or setup")
	}
}
