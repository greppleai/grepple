package cli

import (
	"fmt"
	anchorscommand "github.com/greppleai/grepple/internal/cli/anchors"
	architecturecommand "github.com/greppleai/grepple/internal/cli/architecture"
	artifactscommand "github.com/greppleai/grepple/internal/cli/artifacts"
	askcommand "github.com/greppleai/grepple/internal/cli/ask"
	authcommand "github.com/greppleai/grepple/internal/cli/auth"
	contextcommand "github.com/greppleai/grepple/internal/cli/context"
	examplescommand "github.com/greppleai/grepple/internal/cli/examples"
	getcommand "github.com/greppleai/grepple/internal/cli/get"
	graphcommand "github.com/greppleai/grepple/internal/cli/graph"
	gritcommand "github.com/greppleai/grepple/internal/cli/grit"
	hookcommand "github.com/greppleai/grepple/internal/cli/hook"
	initcommand "github.com/greppleai/grepple/internal/cli/init"
	languagescommand "github.com/greppleai/grepple/internal/cli/languages"
	metricscommand "github.com/greppleai/grepple/internal/cli/metrics"
	refscommand "github.com/greppleai/grepple/internal/cli/refs"
	reposcommand "github.com/greppleai/grepple/internal/cli/repos"
	rulescommand "github.com/greppleai/grepple/internal/cli/rules"
	searchcommand "github.com/greppleai/grepple/internal/cli/search"
	sourcescommand "github.com/greppleai/grepple/internal/cli/sources"
	treecommand "github.com/greppleai/grepple/internal/cli/tree"
	verifycommand "github.com/greppleai/grepple/internal/cli/verify"
	versioncommand "github.com/greppleai/grepple/internal/cli/version"
	writecommand "github.com/greppleai/grepple/internal/cli/write"
	cliruntime "github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/outputspill"
)

// Arguments is the complete application command tree. go-arg populates one
// command pointer and all process-level controls in a single parse.
type Arguments struct {
	NoSpill            bool   `arg:"--no-spill" help:"keep complete output on stdout regardless of size"`
	SpillThreshold     int    `arg:"--spill-threshold-bytes" default:"-1" placeholder:"N" help:"spill output above N bytes"`
	ArtifactDirectory  string `arg:"--artifact-dir" placeholder:"PATH" help:"store spilled artifacts here"`
	NoRepositoryConfig bool   `arg:"--no-repo-config" help:"ignore repository-owned grepple.json behavior"`
	NoConfigIgnore     bool   `arg:"--no-config-ignore" help:"load grepple.json but ignore ignore.paths"`
	ProductionOnly     bool   `arg:"--production-only" help:"recursively select production-classified sources"`
	VersionFlag        bool   `arg:"--version" help:"print build and source version information"`
	Daemon             bool   `arg:"--daemon" help:"use a running greppled for local architecture or focused graph commands (fallback to direct)"`

	Search       *searchcommand.Args         `arg:"subcommand:search" help:"search local or explicitly selected remote code"`
	Write        *writecommand.Args          `arg:"subcommand:write" help:"apply local transactional anchored writes"`
	Grit         *gritcommand.Args           `arg:"subcommand:grit" help:"run native structural queries"`
	Hook         *hookcommand.Args           `arg:"subcommand:hook" help:"run repository-owned local GritQL hooks"`
	Graph        *graphcommand.Args          `arg:"subcommand:graph" help:"query navigation graphs"`
	Ask          *askcommand.Args            `arg:"subcommand:ask" help:"research local and indexed repositories with an AI provider"`
	Anchors      *anchorscommand.Args        `arg:"subcommand:anchors" help:"diagnose edit-anchor providers"`
	Examples     *examplescommand.Args       `arg:"subcommand:examples" help:"print task-oriented CLI workflows"`
	Artifacts    *artifactscommand.Args      `arg:"subcommand:artifacts" help:"manage spilled output artifacts"`
	Context      *contextcommand.Args        `arg:"subcommand:context" help:"manage context deduplication"`
	Architecture *architecturecommand.Args   `arg:"subcommand:architecture" help:"inspect directory architecture"`
	Sources      *sourcescommand.Args        `arg:"subcommand:sources" help:"explain source selection"`
	Init         *initcommand.Args           `arg:"subcommand:init" help:"generate directory metadata"`
	Verify       *verifycommand.Args         `arg:"subcommand:verify" help:"verify directory metadata"`
	Languages    *languagescommand.Args      `arg:"subcommand:languages" help:"show language capabilities"`
	Metrics      *metricscommand.Args        `arg:"subcommand:metrics" help:"analyze explicit agent utility journals"`
	Rules        *rulescommand.Args          `arg:"subcommand:rules" help:"manage saved remote rules"`
	Repos        *reposcommand.Args          `arg:"subcommand:repos" help:"list indexed repositories"`
	Get          *getcommand.Args            `arg:"subcommand:get" help:"read one indexed repository file"`
	Tree         *treecommand.Request        `arg:"subcommand:tree" help:"show a local or indexed repository tree"`
	Refs         *refscommand.Args           `arg:"subcommand:refs" help:"list indexed repository refs"`
	AIProvider   *authcommand.AIProviderArgs `arg:"subcommand:ai-provider" help:"authenticate AI model providers"`
	Login        *authcommand.LoginArgs      `arg:"subcommand:login" help:"authenticate with the remote service"`
	Logout       *authcommand.LogoutArgs     `arg:"subcommand:logout" help:"remove remote authentication"`
	Version      *versioncommand.Args        `arg:"subcommand:version" help:"print version information"`
}

func (values *Arguments) repositoryOptions() cliruntime.RepositoryInvocationOptions {
	return cliruntime.RepositoryInvocationOptions{NoRepositoryConfig: values.NoRepositoryConfig, NoConfigIgnore: values.NoConfigIgnore, ProductionOnly: values.ProductionOnly}
}

func (values *Arguments) spillOptions() outputspill.Options {
	return outputspill.Options{Disabled: values.NoSpill, Threshold: values.SpillThreshold, Directory: values.ArtifactDirectory}
}

func executeArguments(context cliruntime.Context, values *Arguments) error {
	if values.Daemon && values.Architecture == nil && values.Graph == nil {
		return fmt.Errorf("--daemon supports only architecture and focused graph commands")
	}
	switch {
	case values.VersionFlag || values.Version != nil:
		return versioncommand.Execute(context, &versioncommand.Args{})
	case values.Search != nil:
		return searchcommand.Execute(context, values.Search)
	case values.Write != nil:
		return writecommand.Execute(context, values.Write)
	case values.Grit != nil:
		return gritcommand.Execute(context, values.Grit)
	case values.Hook != nil:
		return hookcommand.Execute(context, values.Hook)
	case values.Graph != nil:
		return graphcommand.ExecuteWithDaemon(context, values.Graph, values.Daemon)
	case values.Ask != nil:
		return askcommand.Execute(context, values.Ask)
	case values.Anchors != nil:
		return anchorscommand.Execute(context, values.Anchors)
	case values.Examples != nil:
		return examplescommand.Execute(context, values.Examples)
	case values.Artifacts != nil:
		return artifactscommand.Execute(context, values.Artifacts)
	case values.Context != nil:
		return contextcommand.Execute(context, values.Context)
	case values.Architecture != nil:
		return architecturecommand.ExecuteWithDaemon(context, values.Architecture, values.Daemon)
	case values.Sources != nil:
		return sourcescommand.Execute(context, values.Sources)
	case values.Init != nil:
		return initcommand.Execute(context, values.Init)
	case values.Verify != nil:
		return verifycommand.Execute(context, values.Verify)
	case values.Languages != nil:
		return languagescommand.Execute(context, values.Languages)
	case values.Metrics != nil:
		return metricscommand.Execute(context, values.Metrics)
	case values.Rules != nil:
		return rulescommand.Execute(context, values.Rules)
	case values.Repos != nil:
		return reposcommand.Execute(context, values.Repos)
	case values.Get != nil:
		return getcommand.Execute(context, values.Get)
	case values.Tree != nil:
		return treecommand.Execute(context, values.Tree)
	case values.Refs != nil:
		return refscommand.Execute(context, values.Refs)
	case values.AIProvider != nil:
		return authcommand.ExecuteAIProvider(context, values.AIProvider)
	case values.Login != nil:
		return authcommand.ExecuteLogin(context, values.Login)
	case values.Logout != nil:
		return authcommand.ExecuteLogout(context, values.Logout)
	default:
		return searchcommand.Execute(context, &searchcommand.Args{})
	}
}

func newCommandContextWith(repository cliruntime.RepositoryInvocationOptions, threshold int, exit func(int)) cliruntime.Context {
	return cliruntime.NewContext(cliruntime.ContextOptions{Repository: repository, InlineOutputThreshold: threshold, Exit: exit})
}
