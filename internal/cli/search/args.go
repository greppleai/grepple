package search

import (
	"errors"
	"fmt"
	"io"

	cliruntime "github.com/greppleai/grepple/internal/cliruntime"
	rendercommand "github.com/greppleai/grepple/internal/render"
	"github.com/greppleai/grepple/internal/search"

	"github.com/alexflint/go-arg"
)

// DefaultResultLimit caps how many ranked result files a search returns when the
// caller does not pass --limit. It keeps broad queries from dumping every match;
// --limit 0 restores unbounded local results (a remote server still caps the
// page at search.MaxPageLimit). Alias of the server-side default so both stay
// in sync.
const DefaultResultLimit = search.DefaultPageLimit

// DefaultTextOutputBytes keeps human-readable search output below common agent tool-result limits. JSON remains uncapped so it is never partial.
const DefaultTextOutputBytes = 16 * 1024

type Args struct {
	Local      bool   `arg:"--local" help:"search only the local working directory (this is the default)"`
	Remote     bool   `arg:"-R,--remote" help:"also query the remote shard/router (default: local only)"`
	RemoteOnly bool   `arg:"--remote-only" help:"search only the remote corpus; use snapshot cursor pages for content searches"`
	Cursor     string `arg:"--cursor" placeholder:"TOKEN" help:"resume an account-bound remote content-search page; no local search"`
	Recursive  bool   `arg:"-r,--recursive" help:"search directories recursively (compatibility alias; already the default)"`
	cliruntime.CommonArgs
	LineNumber       bool     `arg:"-n,--line-number" help:"include line numbers (enabled by default)"`
	LineOnly         bool     `arg:"--line-only" help:"print only matching lines; include construct end lines when available"`
	Enclosing        bool     `arg:"--enclosing" help:"line-only: annotate body matches with the nearest enclosing multi-line syntax range"`
	OnlyMatching     bool     `arg:"-o,--only-matching" help:"print each matched substring"`
	Files            bool     `arg:"-l,--files" help:"list files by path; unlike grep -l, does not search contents (use --files-with-matches)"`
	FilesWithMatches bool     `arg:"--files-with-matches" help:"list paths whose contents match (grep -l equivalent); accepts multiple PATHs"`
	Outline          bool     `arg:"-O,--outline" help:"print each file's structural outline (classes, funcs, interfaces) instead of searching"`
	Kinds            []string `arg:"--kind,separate" placeholder:"CATEGORY" help:"outline: show only types, functions, or variables; repeat or comma-separate"`
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
	MaxOutputBytes   int      `arg:"--max-output-bytes" default:"16384" placeholder:"N" help:"cap human-readable output (default 16384; 0 = unlimited; JSON is uncapped)"`
	Related          bool     `arg:"--related" help:"show repository-local types, callees, and callers (default for structural search)"`
	NoRelated        bool     `arg:"--no-related" help:"disable automatic code navigation"`
	RepeatSource     bool     `arg:"--repeat-source" help:"bypass session source deduplication and emit complete focused source again"`
	FollowRelated    int      `arg:"--follow-related" placeholder:"N" help:"follow 1-3 outgoing call hops; search also shows direct callers of matches"`
	At               string   `arg:"--at" placeholder:"PATH:LINE[-END]" help:"retrieve a declaration at PATH:LINE or exact lines for PATH:START-END; ranges that start in-file clamp at EOF"`
	Skip             int      `arg:"--skip" placeholder:"N" help:"skip the first N ranked result files"`
	Limit            int      `arg:"--limit" default:"20" placeholder:"N" help:"return at most N ranked result files (default 20; 0 = all local; servers cap a page at 100 — page further with --skip)"`
	Sort             string   `arg:"--sort" default:"path" placeholder:"ORDER" help:"order result files by path (default) or matching-line count (matches)"`
	Repos            []string `arg:"--repo,separate" placeholder:"PATTERN" help:"restrict remote repositories; repeatable"`
	Query            string   `arg:"positional" placeholder:"PATTERN"`
	Globs            []string `arg:"positional" placeholder:"PATH" help:"file, directory, or glob to search; repeat for multiple roots"`
}

// searchArgs remains as a package-local compatibility name.
type searchArgs = Args

func (Args) Description() string {
	return "Search the local working directory, one or more PATHs, or piped stdin; add --remote (or --server) to also query the shard/router."
}

// DefaultArgs returns search arguments with command defaults applied before parsing.
func DefaultArgs() Args {
	return Args{MaxOutputBytes: DefaultTextOutputBytes, Limit: DefaultResultLimit, Sort: search.ResultSortPath}
}

func parseSearchArgs(application cliruntime.Context, args []string) (*Options, string, bool, error) {
	values := DefaultArgs()
	parser, err := arg.NewParser(arg.Config{Program: "grepple"}, &values)
	if err != nil {
		return nil, "", false, err
	}
	if err := parser.Parse(args); err != nil {
		if errors.Is(err, arg.ErrHelp) {
			parser.WriteHelp(application.Stdout())
			return nil, "", false, nil
		}
		return nil, "", false, err
	}
	return optionsFromArgs(application, &values, parser)
}

func optionsFromArgs(application cliruntime.Context, values *Args, usage interface{ WriteUsage(io.Writer) }) (*Options, string, bool, error) {
	applyCountSummaryAlias(values)
	if err := configureRelatedDefaults(values); err != nil {
		return nil, "", false, err
	}
	anchorsEnabled, err := defaultAnchorsEnabled(application, values)
	if err != nil {
		return nil, "", false, err
	}
	if err := validateSearchArgs(values); err != nil {
		return nil, "", false, err
	}
	kinds, err := rendercommand.ParseOutlineKinds(values.Kinds, values.Outline)
	if err != nil {
		return nil, "", false, err
	}
	params, err := buildSearchParams(application, usage, values)
	if err == nil {
		if repository := application.Repository(); repository != nil {
			err = search.ConfigureSourcePolicy(&params, repository)
		}
	}
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
	remoteEnabled := !values.Local && (values.Remote || values.RemoteOnly || values.Cursor != "" || values.Server != "")
	return &Options{
		Params:           params,
		RemoteOnly:       values.RemoteOnly || values.Cursor != "",
		Cursor:           values.Cursor,
		LineOnly:         values.LineOnly,
		OnlyMatching:     values.OnlyMatching,
		JSON:             jsonModeFor(*values),
		Count:            values.Count,
		CountByRepo:      values.CountByRepo,
		CountSummary:     values.CountSummary,
		FilesWithMatches: values.FilesWithMatches,
		Outline:          values.Outline,
		Kinds:            kinds,
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

func defaultAnchorsEnabled(application cliruntime.Context, values *searchArgs) (bool, error) {
	if !supportsDefaultAnchors(values) {
		return false, nil
	}
	_, err := application.Configuration().UserSettings()
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
	if err := validateCursorArguments(values); err != nil {
		return err
	}
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
func buildSearchParams(application cliruntime.Context, parser interface{ WriteUsage(io.Writer) }, values *Args) (search.Params, error) {
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
		if parser != nil {
			parser.WriteUsage(application.Stderr())
		}
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
