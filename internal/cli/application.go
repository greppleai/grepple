package cli

import (
	"os"

	artifactscommand "github.com/greppleai/grepple/internal/cli/artifacts"
	askcommand "github.com/greppleai/grepple/internal/cli/ask"
	contextcommand "github.com/greppleai/grepple/internal/cli/context"
	examplescommand "github.com/greppleai/grepple/internal/cli/examples"
	extractcommand "github.com/greppleai/grepple/internal/cli/extract"
	getcommand "github.com/greppleai/grepple/internal/cli/get"
	gritcommand "github.com/greppleai/grepple/internal/cli/grit"
	languagescommand "github.com/greppleai/grepple/internal/cli/languages"
	refscommand "github.com/greppleai/grepple/internal/cli/refs"
	reposcommand "github.com/greppleai/grepple/internal/cli/repos"
	rulescommand "github.com/greppleai/grepple/internal/cli/rules"
	cliruntime "github.com/greppleai/grepple/internal/cli/runtime"
	sourcescommand "github.com/greppleai/grepple/internal/cli/sources"
	treecommand "github.com/greppleai/grepple/internal/cli/tree"
	versioncommand "github.com/greppleai/grepple/internal/cli/version"
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
	app.register("version", versioncommand.New(versioncommand.Dependencies{Stdout: os.Stdout}))
	app.register("write", writecommand.New(writecommand.Dependencies{Stdin: os.Stdin, Stdout: os.Stdout, RequestExit: requestExit, RecordResponse: recordWriteResponseContext}))
	app.register("graph", cliruntime.CommandFunc(runGraph))
	app.register("anchors", cliruntime.CommandFunc(runAnchors))
	app.register("boundaries", cliruntime.CommandFunc(runBoundaries))
	app.register("examples", examplescommand.New(examplescommand.Dependencies{Output: os.Stdout}))
	app.register("artifacts", artifactscommand.New(artifactscommand.Dependencies{Stdout: os.Stdout, ArtifactDirectory: defaultOutputArtifactDirectory, WorkingDirectory: mustGetwd}))
	app.register("context", contextcommand.New(contextcommand.Dependencies{Stdout: os.Stdout, Invalidate: invalidateRenderedContext}))
	app.register("languages", languagescommand.New(languagescommand.Dependencies{Stdout: os.Stdout}))
	app.register("get", getcommand.New(getcommand.Dependencies{Stdout: os.Stdout, Stderr: os.Stderr, ServerDefault: serverDefault, NewRequest: authorizedRequest, RequestExit: setExit, RecordRangeOutcome: recordStandaloneLineRangeOutcome, ReportRangeError: reportLineRangeCommandError, FullMissError: func(err error) error { return remoteFullLineRangeMissError{err: err} }, RenderOutline: RenderOutlineOrContent}))
	app.register("tree", treecommand.New(treecommand.Dependencies{Stdout: os.Stdout, ServerDefault: serverDefault, NewRequest: authorizedRequest, RequestExit: setExit, LocalTree: localTree}))
	app.register("repos", reposcommand.New(reposcommand.Dependencies{Stdout: os.Stdout, ServerDefault: serverDefault, NewRequest: authorizedRequest, RequestExit: setExit}))
	app.register("refs", refscommand.New(refscommand.Dependencies{Stdout: os.Stdout, ServerDefault: serverDefault, NewRequest: authorizedRequest, RequestExit: setExit}))
	app.register("ask", cliruntime.CommandFunc(func(args []string) error {
		return askcommand.Run(args, askcommand.Dependencies{Stdout: os.Stdout, Stderr: os.Stderr, RunSession: runAskSession})
	}))
	app.register("ai-provider", cliruntime.CommandFunc(runAIProvider))
	app.register("login", cliruntime.CommandFunc(runLogin))
	app.register("logout", cliruntime.CommandFunc(runLogout))
	app.register("rules", rulescommand.New(rulescommand.Dependencies{ServerDefault: serverDefault, NewRequest: authorizedRequest, RequestExit: setExit}))
	app.register("grit", gritcommand.New(gritDependencies()))
	app.register("extract", extractcommand.New(extractDependencies()))
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
