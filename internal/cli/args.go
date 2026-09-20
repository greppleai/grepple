package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"

	askcommand "github.com/greppleai/grepple/internal/cli/ask"
	"github.com/greppleai/grepple/internal/cli/examples"
	cliruntime "github.com/greppleai/grepple/internal/cli/runtime"
	"github.com/greppleai/grepple/search"

	"github.com/alexflint/go-arg"
)

// DefaultResultLimit caps how many ranked result files a search returns when the
// caller does not pass --limit. It keeps broad queries from dumping every match;
// --limit 0 restores unbounded local results (a remote server still caps the
// page at search.MaxPageLimit). Alias of the server-side default so both stay
// in sync.
const DefaultResultLimit = search.DefaultPageLimit

// DefaultTextOutputBytes keeps human-readable search output comfortably below
// common agent tool-result limits. JSON remains uncapped so it is never partial.
const DefaultTextOutputBytes = 16 * 1024

type searchArgs struct {
	Local     bool `arg:"--local" help:"search only the local working directory (this is the default)"`
	Remote    bool `arg:"-R,--remote" help:"also query the remote shard/router (default: local only)"`
	Recursive bool `arg:"-r,--recursive" help:"search directories recursively (compatibility alias; already the default)"`
	commonArgs
	LineNumber       bool     `arg:"-n,--line-number" help:"include line numbers (enabled by default)"`
	LineOnly         bool     `arg:"--line-only" help:"print only matching lines; include construct end lines when available"`
	Enclosing        bool     `arg:"--enclosing" help:"line-only: annotate body matches with the nearest enclosing multi-line syntax range"`
	OnlyMatching     bool     `arg:"-o,--only-matching" help:"print each matched substring"`
	Files            bool     `arg:"-l,--files" help:"list files by path; unlike grep -l, does not search contents (use --files-with-matches)"`
	FilesWithMatches bool     `arg:"--files-with-matches" help:"list paths whose contents match (grep -l equivalent); accepts multiple PATHs"`
	Outline          bool     `arg:"-O,--outline" help:"print each file's structural outline (classes, funcs, interfaces) instead of searching"`
	Depth            int      `arg:"--depth" placeholder:"N" help:"outline: cap nesting depth for JSON/YAML (0 = unlimited)"`
	Count            bool     `arg:"-c,--count" help:"print matching-line counts per file"`
	CountByRepo      bool     `arg:"--count-by-repo" help:"print complete counts grouped by repository (compatibility name for --count-summary)"`
	CountSummary     bool     `arg:"--count-summary" help:"print complete aggregate file/match counts independent of paging"`
	JSON             bool     `arg:"--json" help:"print full JSON results"`
	JSONMatches      bool     `arg:"--json-matches" help:"print compact JSON matches"`
	Regex            bool     `arg:"-E,--regex" help:"use JavaScript regular expressions (default; -E is a compatibility alias)"`
	Fixed            bool     `arg:"-F,--fixed-strings" help:"treat the pattern as a literal string"`
	IgnoreCase       bool     `arg:"-i,--ignore-case" help:"ignore case distinctions"`
	InvertMatch      bool     `arg:"-v,--invert-match" help:"select lines that do not match"`
	Context          int      `arg:"-C,--context" placeholder:"N" help:"print N lines before and after matches"`
	AfterContext     int      `arg:"-A,--after-context" placeholder:"N" help:"print N lines after matches"`
	BeforeContext    int      `arg:"-B,--before-context" placeholder:"N" help:"print N lines before matches"`
	MaxFiles         int      `arg:"--max-files" placeholder:"N" help:"admit at most N matching result files before paging (0 = unlimited)"`
	MaxOutputBytes   int      `arg:"--max-output-bytes" placeholder:"N" help:"cap human-readable output (default 16384; 0 = unlimited; JSON is uncapped)"`
	Related          bool     `arg:"--related" help:"show repository-local types, callees, and callers (default for structural search)"`
	NoRelated        bool     `arg:"--no-related" help:"disable automatic code navigation"`
	RepeatSource     bool     `arg:"--repeat-source" help:"bypass session source deduplication and emit complete focused source again"`
	FollowRelated    int      `arg:"--follow-related" placeholder:"N" help:"expand up to two callers and callees per level (1-3; default 1)"`
	At               string   `arg:"--at" placeholder:"PATH:LINE[-END]" help:"retrieve a declaration, or exact lines with --line-only; ranges that start in-file clamp at EOF"`
	Skip             int      `arg:"--skip" placeholder:"N" help:"skip the first N ranked result files"`
	Limit            int      `arg:"--limit" placeholder:"N" help:"return at most N ranked result files (default 20; 0 = all local; servers cap a page at 100 — page further with --skip)"`
	Sort             string   `arg:"--sort" placeholder:"ORDER" help:"order result files by path (default) or matching-line count (matches)"`
	Repos            []string `arg:"--repo,separate" placeholder:"PATTERN" help:"restrict remote repositories; repeatable"`
	Query            string   `arg:"positional" placeholder:"PATTERN"`
	Globs            []string `arg:"positional" placeholder:"PATH" help:"file, directory, or glob to search; repeat for multiple roots"`
}

func (searchArgs) Description() string {
	return "Search the local working directory, one or more PATHs, or piped stdin; add --remote (or --server) to also query the shard/router."
}

func parseSearchArgs(args []string) (*cliOptions, string, bool, error) {
	// Limit and text output default to bounded values so broad queries do not
	// overflow agent tool results. Pass zero explicitly to opt out of either cap.
	values := searchArgs{
		MaxOutputBytes: DefaultTextOutputBytes,
		Limit:          DefaultResultLimit,
		Sort:           search.ResultSortPath,
	}
	parser, err := arg.NewParser(arg.Config{Program: "grepple"}, &values)
	if err != nil {
		return nil, "", false, err
	}
	if err := parser.Parse(args); err != nil {
		if errors.Is(err, arg.ErrHelp) {
			parser.WriteHelp(os.Stdout)
			return nil, "", false, nil
		}
		return nil, "", false, err
	}
	applyCountSummaryAlias(&values)
	if err := configureRelatedDefaults(&values); err != nil {
		return nil, "", false, err
	}
	anchorsEnabled, err := defaultAnchorsEnabled(&values)
	if err != nil {
		return nil, "", false, err
	}
	if err := validateSearchArgs(&values); err != nil {
		return nil, "", false, err
	}
	params, err := configureResolvedSearchParams(buildSearchParams(parser, &values))
	if err != nil {
		return nil, "", false, err
	}

	// Skip full structural segments for compact output modes. Line-only still asks
	// parser for focused construct-end metadata without rendering source bodies.
	params.SkipSegments = values.Files || values.FilesWithMatches || values.Count || values.CountByRepo || values.LineOnly || values.OnlyMatching || values.JSONMatches || params.BeforeContext > 0 || params.AfterContext > 0
	params.LineRanges = values.LineOnly
	params.EnclosingRanges = values.Enclosing

	// Local-first: only reach out to the shard/router when the user explicitly opts
	// in with --remote or by passing a --server URL. A configured GREPPLE_SERVER / config
	// server just supplies the URL; it no longer forces every search to hit remote.
	remoteEnabled := !values.Local && (values.Remote || values.Server != "")
	return &cliOptions{
		Params:           params,
		LineOnly:         values.LineOnly,
		OnlyMatching:     values.OnlyMatching,
		JSON:             jsonModeFor(values),
		Count:            values.Count,
		CountByRepo:      values.CountByRepo,
		CountSummary:     values.CountSummary,
		FilesWithMatches: values.FilesWithMatches,
		Outline:          values.Outline,
		Depth:            values.Depth,
		MaxOutputBytes:   values.MaxOutputBytes,
		RepeatSource:     values.RepeatSource,
		Anchors:          anchorsEnabled,
	}, values.Server, remoteEnabled, nil
}

func applyCountSummaryAlias(values *searchArgs) {
	if values.CountSummary {
		values.CountByRepo = true
	}
}

func defaultAnchorsEnabled(values *searchArgs) (bool, error) {
	if !supportsDefaultAnchors(values) {
		return false, nil
	}
	_, err := loadUserSettings()
	if err != nil {
		return false, err
	}
	return true, nil
}

func supportsDefaultAnchors(values *searchArgs) bool {
	if values.Remote || values.Server != "" || values.JSON || values.JSONMatches {
		return false
	}
	unsupportedCompact := values.Files || values.FilesWithMatches || values.Outline || values.Count || values.CountByRepo || values.OnlyMatching || values.Enclosing
	return !unsupportedCompact
}

func usesCompactSearchOutput(values *searchArgs) bool {
	return values.Files || values.FilesWithMatches || values.Outline || values.Count || values.CountByRepo || values.LineOnly || values.OnlyMatching || values.JSONMatches
}

func configureRelatedDefaults(values *searchArgs) error {
	if values.NoRelated {
		if values.Related || values.FollowRelated > 0 {
			return fmt.Errorf("--no-related cannot be combined with --related or --follow-related")
		}
		return nil
	}
	if values.Related || values.FollowRelated > 0 {
		values.Related = true
		if values.FollowRelated == 0 {
			values.FollowRelated = 1
		}
		return nil
	}
	contextOutput := values.Context > 0 || values.BeforeContext > 0 || values.AfterContext > 0
	if !usesCompactSearchOutput(values) && !contextOutput {
		values.Related = true
		values.FollowRelated = 1
	}
	return nil
}

// validateRelatedArgs keeps experimental source navigation scoped to modes
// validateRelatedArgs limits navigation to output modes that can preserve its
func validateRelatedArgs(values *searchArgs) error {
	if !values.Related {
		return nil
	}
	compactOutput := usesCompactSearchOutput(values)
	contextOutput := values.Context > 0 || values.BeforeContext > 0 || values.AfterContext > 0
	if compactOutput || contextOutput {
		return fmt.Errorf("--related requires default structural output or --json")
	}
	return nil
}

func validateAtArgs(values *searchArgs) error {
	if values.At == "" {
		return nil
	}
	if !values.Local && (values.Remote || values.Server != "") && len(values.Repos) != 1 {
		return fmt.Errorf("remote --at requires exactly one --repo OWNER/REPO[@REF]")
	}
	if values.Query != "" || len(values.Globs) > 0 {
		return fmt.Errorf("--at cannot be combined with a search pattern or path")
	}
	incompatibleOutput := values.Files || values.FilesWithMatches || values.Outline || values.Count || values.CountByRepo || values.OnlyMatching || values.Enclosing || values.JSONMatches
	if incompatibleOutput {
		return fmt.Errorf("--at requires default structural, contextual, line-only, or JSON output")
	}
	return nil
}

func validateEnclosingArgs(values *searchArgs) error {
	if !values.Enclosing {
		return nil
	}
	if !values.LineOnly {
		return fmt.Errorf("--enclosing requires --line-only")
	}
	incompatibleOutput := values.Files || values.FilesWithMatches || values.Outline || values.Count || values.CountByRepo || values.OnlyMatching
	contextOutput := values.Context > 0 || values.BeforeContext > 0 || values.AfterContext > 0
	if incompatibleOutput || contextOutput {
		return fmt.Errorf("--enclosing requires line-only or line-only JSON output")
	}
	return nil
}

// validateSearchArgs rejects contradictory or out-of-range flag combinations
// before any searching starts.
func validateSearchArgs(values *searchArgs) error {
	if err := validateSearchBounds(values); err != nil {
		return err
	}
	if err := validateEnclosingArgs(values); err != nil {
		return err
	}
	if err := validateRelatedArgs(values); err != nil {
		return err
	}
	if err := validateAtArgs(values); err != nil {
		return err
	}
	contextOutput := values.Context > 0 || values.BeforeContext > 0 || values.AfterContext > 0
	focusedLineOutput := values.LineOnly && values.At != ""
	unsupportedRepeatOutput := (!focusedLineOutput && usesCompactSearchOutput(values)) || contextOutput || values.JSON
	if values.RepeatSource && unsupportedRepeatOutput {
		return fmt.Errorf("--repeat-source requires default structural human output or focused --at --line-only output")
	}
	return validateSearchCombinations(values)
}
func validateSearchCombinations(values *searchArgs) error {
	if values.Regex && values.Fixed {
		return fmt.Errorf("--regex and --fixed-strings cannot be used together")
	}
	if values.InvertMatch && values.OnlyMatching {
		return fmt.Errorf("--invert-match and --only-matching cannot be used together")
	}
	if values.Files && values.FilesWithMatches {
		return fmt.Errorf("--files (filename glob) and --files-with-matches (content) cannot be used together")
	}
	if values.Count && values.CountByRepo {
		return fmt.Errorf("--count and --count-summary cannot be used together")
	}
	if values.Local && values.Remote {
		return fmt.Errorf("--local and --remote cannot be used together")
	}
	if values.Skip < 0 {
		return fmt.Errorf("--skip must not be negative")
	}
	if values.Limit < 0 {
		return fmt.Errorf("--limit must not be negative")
	}
	if values.Depth < 0 {
		return fmt.Errorf("--depth must not be negative")
	}
	if values.FollowRelated < 0 || values.FollowRelated > 3 {
		return fmt.Errorf("--follow-related must be between 1 and 3")
	}
	return nil
}

func validateSearchBounds(values *searchArgs) error {
	if values.Context < 0 {
		return fmt.Errorf("--context requires a non-negative integer")
	}
	if values.BeforeContext < 0 {
		return fmt.Errorf("--before-context requires a non-negative integer")
	}
	if values.AfterContext < 0 {
		return fmt.Errorf("--after-context requires a non-negative integer")
	}
	if values.MaxFiles < 0 {
		return fmt.Errorf("--max-files must be a positive number")
	}
	if values.MaxOutputBytes < 0 {
		return fmt.Errorf("--max-output-bytes must not be negative")
	}
	if values.Sort != search.ResultSortPath && values.Sort != search.ResultSortMatches {
		return fmt.Errorf("--sort must be %q or %q", search.ResultSortPath, search.ResultSortMatches)
	}
	return nil
}

// buildSearchParams maps the parsed flags onto the wire search params; under
// --files/--outline positionals are path globs, not a content pattern.
func buildSearchParams(parser *arg.Parser, values *searchArgs) (search.Params, error) {
	params := search.Params{
		Query:         values.Query,
		Globs:         values.Globs,
		Regex:         !values.Fixed,
		IgnoreCase:    values.IgnoreCase,
		InvertMatch:   values.InvertMatch,
		Files:         values.Files,
		Context:       values.Context,
		BeforeContext: values.Context,
		AfterContext:  values.Context,
		MaxFiles:      values.MaxFiles,
		Related:       values.Related,
		FollowRelated: values.FollowRelated,
		NoRelated:     values.NoRelated,
		At:            values.At,
		Skip:          values.Skip,
		Limit:         values.Limit,
		Sort:          values.Sort,
		Repo:          values.Repos,
	}
	if values.BeforeContext > 0 {
		params.BeforeContext = values.BeforeContext
	}
	if values.AfterContext > 0 {
		params.AfterContext = values.AfterContext
	}
	if values.Regex {
		params.Regex = true
	}
	if params.Files || values.Outline {
		// Outline and --files both treat positionals as path globs, not a pattern.
		if params.Query != "" {
			params.Globs = append([]string{params.Query}, params.Globs...)
			params.Query = ""
		}
	} else if params.Query == "" && params.At == "" {
		parser.WriteUsage(os.Stderr)
		return params, fmt.Errorf("search requires a pattern")
	}
	if values.Outline && (values.Count || values.CountByRepo || values.FilesWithMatches) {
		return params, fmt.Errorf("--outline cannot be combined with --count, --count-by-repo, or --files-with-matches")
	}
	return params, nil
}

// jsonModeFor maps the JSON flags to the output mode (full beats matches).
func jsonModeFor(values searchArgs) string {
	if values.JSON {
		return "full"
	}
	if values.JSONMatches {
		return "matches"
	}
	return "off"
}

const topLevelHelp = `Grepple searches code and provides source-backed navigation, structural queries, architecture views, and remote repository access.

Usage:
  grepple [SEARCH OPTIONS] PATTERN [PATH ...]
  grepple COMMAND [OPTIONS]

Commands:
  search       Search local or explicitly selected remote code (local default; --remote merges)
  write        Apply local transactional anchored edits, creates, and deletes (local-only)
  grit         Run native structural queries (local default; --remote merges)
  graph        Build or query navigation graphs (local default; --repo selects indexed remote)
  anchors      Diagnose and configure local edit-anchor providers (local-only)
  boundaries   Analyze boundaries (local default; --repo selects indexed remote)
  examples     Print task-oriented, copyable CLI workflows
  artifacts    Manage local spilled output artifacts (local-only)
  context      Manage local structural-segment context deduplication (local-only)
  extract      Generate or check focused Mermaid projections (local-only)
  architecture Inspect directory architecture (local default; --repo selects indexed remote)
  sources      Explain local repository configuration and source selection (local-only)
  languages    Show the language capability matrix (source-independent)
  rules        Manage saved remote rules (remote service)
  repos        List indexed repositories (remote service)
  get          Read one indexed repository file or outline (remote service)
  tree         List an indexed repository tree (remote service)
  refs         List indexed repository branches, tags, and commits (remote service)
  ask          Delegate bounded local or exact-selector remote code research
  ai-provider  Authenticate and inspect AI model providers
  login        Authenticate with the remote service
  logout       Remove stored remote authentication
  version      Print build and source version information

Global output delivery:
	--no-spill                    keep complete output on stdout regardless of size
	--spill-threshold-bytes N     spill output above N bytes (default 65536 or grepple.json)
	--artifact-dir PATH           store spilled artifacts here (default: user cache or GREPPLE_ARTIFACT_DIR)

Global repository scope:
	--no-repo-config              ignore repository-owned grepple.json behavior
	--no-config-ignore            load grepple.json but ignore ignore.paths
	--production-only             recursively select production-classified sources

Search options follow below. Run grepple COMMAND --help for command-specific options.

`

func writeTopLevelHelp() error {
	if _, err := fmt.Fprint(os.Stdout, topLevelHelp); err != nil {
		return err
	}
	_, _, _, err := parseSearchArgs([]string{"--help"})
	return err
}

func runHelp(args []string) error {
	if len(args) == 0 {
		return writeTopLevelHelp()
	}
	if len(args) > 1 {
		switch args[0] {
		case "graph", "extract", "architecture", "sources", "rules", "anchors", "grit", "artifacts", "ai-provider":
			command := append([]string(nil), args...)
			command = append(command, "--help")
			return runCommand(command)
		default:
			return fmt.Errorf("command %q has no help subcommands", args[0])
		}
	}
	switch args[0] {
	case "search":
		return runSearch([]string{"--help"})
	case "extract":
		return runExtract([]string{"--help"})
	case "login":
		return stdoutWriter().writeString("Authenticate with the remote service.\nUsage: grepple login [--url URL] [--scope SCOPES] [--no-browser]\n")
	case "logout":
		return stdoutWriter().writeString("Remove stored remote authentication.\nUsage: grepple logout\n")
	case "version":
		return stdoutWriter().writeString("Print build and source version information.\nUsage: grepple version\n")
	case "grit", "graph", "anchors", "boundaries", "examples", "languages", "rules", "get", "tree", "repos", "refs", "artifacts", "context", "architecture", "sources", "ask", "ai-provider", "write":
		return runCommand([]string{args[0], "--help"})
	default:
		return fmt.Errorf("unknown help topic %q", args[0])
	}
}

// Run executes a Grepple command with repository scope controls and default large-output spilling.
func Run(args []string) error {
	repositoryInvocationMutex.Lock()
	defer repositoryInvocationMutex.Unlock()

	repositoryArgs, repositoryOptions, err := parseRepositoryInvocationOptions(args)
	if err != nil {
		return err
	}
	previousRepositoryOptions := activeRepositoryOptions
	activeRepositoryOptions = repositoryOptions
	defer func() { activeRepositoryOptions = previousRepositoryOptions }()
	resetRequestedExit()
	commandArgs, spill, err := parseSpillOptions(repositoryArgs)
	if err != nil {
		return err
	}
	if len(commandArgs) >= 2 && commandArgs[0] == "artifacts" && commandArgs[1] == "clean" {
		spill.disabled = true
	}
	descriptorArgs := append(activeRepositoryScopeFlags(), commandArgs...)
	err = runWithOutputSpill(descriptorArgs, spill, func() error { return runCommand(commandArgs) })
	if err != nil {
		return err
	}
	if code := requestedExit(); code != 0 {
		return cliruntime.NewExitError(code)
	}
	return nil
}

func runCommand(args []string) error {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		return writeTopLevelHelp()
	}
	if len(args) > 0 && args[0] == "help" {
		return runHelp(args[1:])
	}
	if len(args) == 1 && (args[0] == "--version" || args[0] == "version") {
		fmt.Fprintln(os.Stdout, versionString())
		return nil
	}
	if len(args) > 0 {
		if err := validateCommandAvailability(args[0], args[1:]); err != nil {
			return err
		}
		switch args[0] {
		case "search":
			return runSearch(args[1:])
		case "write":
			return runWrite(args[1:])
		case "graph":
			return runGraph(args[1:])
		case "anchors":
			return runAnchors(args[1:])
		case "boundaries":
			return runBoundaries(args[1:])
		case "examples":
			return examples.Run(args[1:], os.Stdout)
		case "artifacts":
			return runArtifacts(args[1:])
		case "context":
			return runContext(args[1:])
		case "languages":
			return runLanguages(args[1:])
		case "get":
			return runGet(args[1:])
		case "tree":
			return runTree(args[1:])
		case "repos":
			return runRepos(args[1:])
		case "refs":
			return runRefs(args[1:])
		case "ask":
			return askcommand.Run(args[1:], askcommand.Dependencies{Stdout: os.Stdout, Stderr: os.Stderr, RunSession: runAskSession})
		case "ai-provider":
			return runAIProvider(args[1:])
		case "login":
			return runLogin(args[1:])
		case "logout":
			return runLogout(args[1:])
		case "rules":
			return runRules(args[1:])
		case "grit":
			return runGrit(args[1:])
		case "extract":
			return runExtract(args[1:])
		case "architecture":
			return runArchitecture(args[1:])
		case "sources":
			return runSources(args[1:])
		}
	}
	return runSearch(args)
}

func validateCommandAvailability(command string, args []string) error {
	availability := map[string]string{
		"write": "local-only", "anchors": "local-only", "examples": "source-independent", "artifacts": "local-only", "context": "local-only",
		"languages": "source-independent", "extract": "local-only", "sources": "local-only", "ai-provider": "source-independent",
	}
	remoteOnly := map[string]bool{"get": true, "tree": true, "repos": true, "refs": true, "rules": true, "login": true, "logout": true}
	for _, argument := range args {
		if argument == "--" {
			break
		}
		option := argument
		if equals := strings.IndexByte(option, '='); equals >= 0 {
			option = option[:equals]
		}
		if mode, unsupported := availability[command]; unsupported && (option == "--remote" || option == "-R" || option == "--repo" || option == "--server") {
			return fmt.Errorf("%s is %s; remote selector %s is not supported", command, mode, option)
		}
		if remoteOnly[command] && option == "--local" {
			return fmt.Errorf("%s uses the remote service; --local is not supported", command)
		}
	}
	return nil
}
