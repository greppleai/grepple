package cli

import (
	"os"

	artifactscommand "github.com/greppleai/grepple/internal/cli/artifacts"
	askcommand "github.com/greppleai/grepple/internal/cli/ask"
	contextcommand "github.com/greppleai/grepple/internal/cli/context"
	"github.com/greppleai/grepple/internal/cli/examples"
	getcommand "github.com/greppleai/grepple/internal/cli/get"
	languagescommand "github.com/greppleai/grepple/internal/cli/languages"
	refscommand "github.com/greppleai/grepple/internal/cli/refs"
	reposcommand "github.com/greppleai/grepple/internal/cli/repos"
	cliruntime "github.com/greppleai/grepple/internal/cli/runtime"
	sourcescommand "github.com/greppleai/grepple/internal/cli/sources"
	treecommand "github.com/greppleai/grepple/internal/cli/tree"
	writecommand "github.com/greppleai/grepple/internal/cli/write"
)

type commandSpec struct {
	name    string
	command cliruntime.Command
}

type application struct {
	commands       map[string]commandSpec
	defaultCommand cliruntime.Command
}

func newApplication() *application {
	searchCommand := cliruntime.CommandFunc(runSearch)
	app := &application{commands: make(map[string]commandSpec), defaultCommand: searchCommand}
	app.register("search", searchCommand)
	app.register("write", cliruntime.CommandFunc(func(args []string) error {
		return writecommand.Run(args, writecommand.Dependencies{Stdin: os.Stdin, Stdout: os.Stdout, RequestExit: requestExit, RecordResponse: recordWriteResponseContext})
	}))
	app.register("graph", cliruntime.CommandFunc(runGraph))
	app.register("anchors", cliruntime.CommandFunc(runAnchors))
	app.register("boundaries", cliruntime.CommandFunc(runBoundaries))
	app.register("examples", cliruntime.CommandFunc(func(args []string) error { return examples.Run(args, os.Stdout) }))
	app.register("artifacts", cliruntime.CommandFunc(func(args []string) error {
		return artifactscommand.Run(args, artifactscommand.Dependencies{Stdout: os.Stdout, ArtifactDirectory: defaultOutputArtifactDirectory, WorkingDirectory: mustGetwd})
	}))
	app.register("context", cliruntime.CommandFunc(func(args []string) error {
		return contextcommand.Run(args, contextcommand.Dependencies{Stdout: os.Stdout, Invalidate: invalidateRenderedContext})
	}))
	app.register("languages", cliruntime.CommandFunc(func(args []string) error {
		return languagescommand.Run(args, languagescommand.Dependencies{Stdout: os.Stdout})
	}))
	app.register("get", cliruntime.CommandFunc(func(args []string) error {
		return getcommand.Run(args, getcommand.Dependencies{Stdout: os.Stdout, Stderr: os.Stderr, ServerDefault: serverDefault, NewRequest: authorizedRequest, RequestExit: setExit, RecordRangeOutcome: recordStandaloneLineRangeOutcome, ReportRangeError: reportLineRangeCommandError, FullMissError: func(err error) error { return remoteFullLineRangeMissError{err: err} }, RenderOutline: RenderOutlineOrContent})
	}))
	app.register("tree", cliruntime.CommandFunc(func(args []string) error {
		return treecommand.Run(args, treecommand.Dependencies{Stdout: os.Stdout, ServerDefault: serverDefault, NewRequest: authorizedRequest, RequestExit: setExit, LocalTree: localTree})
	}))
	app.register("repos", cliruntime.CommandFunc(func(args []string) error {
		return reposcommand.Run(args, reposcommand.Dependencies{Stdout: os.Stdout, ServerDefault: serverDefault, NewRequest: authorizedRequest, RequestExit: setExit})
	}))
	app.register("refs", cliruntime.CommandFunc(func(args []string) error {
		return refscommand.Run(args, refscommand.Dependencies{Stdout: os.Stdout, ServerDefault: serverDefault, NewRequest: authorizedRequest, RequestExit: setExit})
	}))
	app.register("ask", cliruntime.CommandFunc(func(args []string) error {
		return askcommand.Run(args, askcommand.Dependencies{Stdout: os.Stdout, Stderr: os.Stderr, RunSession: runAskSession})
	}))
	app.register("ai-provider", cliruntime.CommandFunc(runAIProvider))
	app.register("login", cliruntime.CommandFunc(runLogin))
	app.register("logout", cliruntime.CommandFunc(runLogout))
	app.register("rules", cliruntime.CommandFunc(runRules))
	app.register("grit", cliruntime.CommandFunc(runGrit))
	app.register("extract", cliruntime.CommandFunc(runExtract))
	app.register("architecture", cliruntime.CommandFunc(runArchitecture))
	app.register("sources", cliruntime.CommandFunc(func(args []string) error {
		return sourcescommand.Run(args, sourceCommandDependencies(os.Stdout))
	}))
	return app
}

func (app *application) register(name string, command cliruntime.Command) {
	if name == "" || command == nil {
		panic("CLI command registration requires a name and command")
	}
	if _, exists := app.commands[name]; exists {
		panic("duplicate CLI command registration: " + name)
	}
	app.commands[name] = commandSpec{name: name, command: command}
}

func (app *application) run(args []string) error {
	if len(args) > 0 {
		if err := validateCommandAvailability(args[0], args[1:]); err != nil {
			return err
		}
		if spec, ok := app.commands[args[0]]; ok {
			return spec.command.Run(args[1:])
		}
	}
	return app.defaultCommand.Run(args)
}
