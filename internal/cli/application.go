package cli

import (
	"os"

	codeextract "github.com/greppleai/grepple/extract"

	"github.com/greppleai/grepple/internal/authstate"
	"github.com/greppleai/grepple/linerange"
	"github.com/greppleai/grepple/search"

	anchorscommand "github.com/greppleai/grepple/internal/cli/anchors"
	architecturecommand "github.com/greppleai/grepple/internal/cli/architecture"
	artifactscommand "github.com/greppleai/grepple/internal/cli/artifacts"
	askcommand "github.com/greppleai/grepple/internal/cli/ask"
	authcommand "github.com/greppleai/grepple/internal/cli/auth"
	boundariescommand "github.com/greppleai/grepple/internal/cli/boundaries"
	contextcommand "github.com/greppleai/grepple/internal/cli/context"
	examplescommand "github.com/greppleai/grepple/internal/cli/examples"
	extractcommand "github.com/greppleai/grepple/internal/cli/extract"
	getcommand "github.com/greppleai/grepple/internal/cli/get"
	graphcommand "github.com/greppleai/grepple/internal/cli/graph"
	gritcommand "github.com/greppleai/grepple/internal/cli/grit"
	languagescommand "github.com/greppleai/grepple/internal/cli/languages"
	refscommand "github.com/greppleai/grepple/internal/cli/refs"
	reposcommand "github.com/greppleai/grepple/internal/cli/repos"
	rulescommand "github.com/greppleai/grepple/internal/cli/rules"
	searchcommand "github.com/greppleai/grepple/internal/cli/search"
	sourcescommand "github.com/greppleai/grepple/internal/cli/sources"
	treecommand "github.com/greppleai/grepple/internal/cli/tree"
	versioncommand "github.com/greppleai/grepple/internal/cli/version"
	writecommand "github.com/greppleai/grepple/internal/cli/write"
	cliruntime "github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/gitcontext"
	rendercommand "github.com/greppleai/grepple/internal/render"
	"github.com/greppleai/grepple/internal/repositoryscope"
	"github.com/greppleai/grepple/internal/storagepaths"
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
	searchCommand := searchcommand.New(searchcommand.Dependencies{Execute: runSearch})
	graphServices := graphcommand.Services{ApplySourceConfig: applyRepositorySourceConfig, Remote: requestAnalysisRemote, ServerDefault: serverDefault, ActiveScopeFlags: appendActiveRepositoryScopeFlags, Metadata: graphResultMetadata, DiffMetadata: graphDiffResultMetadata}
	graphCommand := graphcommand.New(graphcommand.Dependencies{
		Build: func(args []string) error { return graphcommand.RunBuild(args, graphServices) },
		Diff:  func(args []string) error { return graphcommand.RunDiff(args, graphServices) },
		Query: func(args []string) error {
			return graphcommand.RunQuery(search.NavigationQueryDirection(args[0]), args[1:], graphServices)
		},
		Stdout: os.Stdout,
		LoadOutput: func(globs []string, maxFiles int) (graphcommand.Output, error) {
			paths, err := graphcommand.ResolveInputPaths(globs, applyRepositorySourceConfig)
			if err != nil {
				return graphcommand.Output{}, err
			}
			return graphcommand.BuildFromPaths(paths, maxFiles), nil
		},
		ActiveScopeFlags: appendActiveRepositoryScopeFlags,
		RequestExit:      setExit,
	})
	app := &application{commands: make(map[string]commandSpec), defaultCommand: searchCommand}
	app.register("search", searchCommand)
	app.register("version", versioncommand.New(versioncommand.Dependencies{Stdout: os.Stdout}))
	app.register("write", writecommand.New(writecommand.Dependencies{Stdin: os.Stdin, Stdout: os.Stdout, RequestExit: requestExit, RecordResponse: writecommand.NewContextRecorder(contextGuardEnabled(), activeInlineOutputThreshold)}))
	app.register("graph", graphCommand)
	app.register("anchors", anchorscommand.New(anchorscommand.Dependencies{}))
	app.register("boundaries", boundariescommand.New(boundariescommand.Dependencies{ResolvePaths: func(paths []string) ([]string, error) {
		return graphcommand.ResolveInputPaths(paths, applyRepositorySourceConfig)
	}, BuildGraph: func(paths []string, maxFiles int, options search.NavigationBuildOptions) boundariescommand.GraphOutput {
		return boundariescommand.GraphFromNavigation(graphcommand.BuildFromPathsWithOptions(paths, maxFiles, options))
	}, CacheDirectory: func() string { return storagepaths.Cache(mustGetwd()) }, Remote: requestAnalysisRemote, ServerDefault: serverDefault, ActiveScopeFlags: appendActiveRepositoryScopeFlags}))
	app.register("examples", examplescommand.New(examplescommand.Dependencies{Output: os.Stdout}))
	app.register("artifacts", artifactscommand.New(artifactscommand.Dependencies{Stdout: os.Stdout, ArtifactDirectory: storagepaths.OutputArtifacts, WorkingDirectory: mustGetwd}))
	app.register("context", contextcommand.New(contextcommand.Dependencies{Stdout: os.Stdout, Invalidate: rendercommand.InvalidateContext}))
	app.register("languages", languagescommand.New(languagescommand.Dependencies{Stdout: os.Stdout}))
	app.register("get", getcommand.New(getcommand.Dependencies{Stdout: os.Stdout, Stderr: os.Stderr, ServerDefault: serverDefault, NewRequest: authorizedRequest, RequestExit: setExit, RecordRangeOutcome: func(outcome linerange.Outcome) {
		rendercommand.RecordStandaloneLineRangeOutcome(outcome, contextGuardEnabled())
	}, ReportRangeError: reportLineRangeCommandError, FullMissError: func(err error) error { return remoteFullLineRangeMissError{err: err} }, RenderOutline: rendercommand.OutlineOrContent}))
	app.register("tree", treecommand.New(treecommand.Dependencies{Stdout: os.Stdout, ServerDefault: serverDefault, NewRequest: authorizedRequest, RequestExit: setExit, LocalTree: treecommand.NewLocal(applyRepositorySourceConfig, mustGetwd)}))
	app.register("repos", reposcommand.New(reposcommand.Dependencies{Stdout: os.Stdout, ServerDefault: serverDefault, NewRequest: authorizedRequest, RequestExit: setExit}))
	app.register("refs", refscommand.New(refscommand.Dependencies{Stdout: os.Stdout, ServerDefault: serverDefault, NewRequest: authorizedRequest, RequestExit: setExit}))
	app.register("ask", cliruntime.CommandFunc(func(args []string) error {
		return askcommand.Run(args, askcommand.Dependencies{Stdout: os.Stdout, Stderr: os.Stderr, RunSession: runAskSession})
	}))
	authentication := authcommand.Dependencies{ServerDefault: serverDefault, StoreLogin: authstate.StoreLogin, ClearToken: authstate.Clear, ConfigPath: authstate.Path}
	app.register("ai-provider", authcommand.NewAIProvider(authentication))
	app.register("login", authcommand.NewLogin(authentication))
	app.register("logout", authcommand.NewLogout(authentication))
	app.register("rules", rulescommand.New(rulescommand.Dependencies{ServerDefault: serverDefault, NewRequest: authorizedRequest, RequestExit: setExit}))
	app.register("grit", gritcommand.New(gritcommand.Dependencies{ApplySourceConfig: applyRepositorySourceConfig, CurrentRepository: gitcontext.Current, ServerDefault: serverDefault, RequestRemote: requestGritRemote, Metadata: gritResultMetadata, RequestExit: setExit}))
	app.register("extract", extractcommand.New(extractcommand.Dependencies{Stdout: os.Stdout, LoadSources: func(roots []string) ([]codeextract.Source, error) {
		options, err := repositoryScopeOptions()
		if err != nil {
			return nil, err
		}
		return repositoryscope.LoadSources(roots, options)
	}}))
	app.register("architecture", architecturecommand.New(architecturecommand.Dependencies{ApplySourceConfig: applyRepositorySourceConfig, Remote: requestAnalysisRemote, ServerDefault: serverDefault, RequestExit: requestExit}))
	app.register("sources", cliruntime.CommandFunc(func(args []string) error {
		return sourcescommand.Run(args, sourcescommand.Dependencies{Stdout: os.Stdout, Environment: sourceScopeEnvironment, WorkingDirectory: mustGetwd})
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
