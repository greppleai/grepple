package grepplecli

import (
	"errors"
	"fmt"
	"os"

	"grepple/internal/search"

	"github.com/alexflint/go-arg"
)

// DefaultResultLimit caps how many ranked result files a search returns when the
// caller does not pass --limit. It keeps broad queries from dumping every match;
// --limit 0 restores unbounded local results (a remote server still caps the
// page at search.MaxPageLimit). Alias of the server-side default so both stay
// in sync.
const DefaultResultLimit = search.DefaultPageLimit

type searchArgs struct {
	Local            bool     `arg:"--local" help:"search only the local working directory (this is the default)"`
	Remote           bool     `arg:"-R,--remote" help:"also query the remote shard/router (default: local only)"`
	Server           string   `arg:"-s,--server" placeholder:"URL" help:"remote shard/router URL (implies --remote)"`
	LineNumber       bool     `arg:"-n,--line-number" help:"include line numbers (enabled by default)"`
	LineOnly         bool     `arg:"--line-only" help:"print only matching lines"`
	OnlyMatching     bool     `arg:"-o,--only-matching" help:"print each matched substring"`
	Files            bool     `arg:"-l,--files" help:"recursively list files under optional PATHs; glob PATHs filter the listing"`
	FilesWithMatches bool     `arg:"--files-with-matches" help:"list paths whose contents match; accepts multiple file, directory, or glob PATHs"`
	Outline          bool     `arg:"-O,--outline" help:"print each file's structural outline (classes, funcs, interfaces) instead of searching"`
	Depth            int      `arg:"--depth" placeholder:"N" help:"outline: cap nesting depth for JSON/YAML (0 = unlimited)"`
	Count            bool     `arg:"-c,--count" help:"print matching-line counts per file"`
	CountByRepo      bool     `arg:"--count-by-repo" help:"print aggregate file and matching-line counts per repository"`
	JSON             bool     `arg:"--json" help:"print full JSON results"`
	JSONMatches      bool     `arg:"--json-matches" help:"print compact JSON matches"`
	Regex            bool     `arg:"--regex" help:"treat the pattern as a regular expression (default)"`
	Fixed            bool     `arg:"-F,--fixed-strings" help:"treat the pattern as a literal string"`
	IgnoreCase       bool     `arg:"-i,--ignore-case" help:"ignore case distinctions"`
	InvertMatch      bool     `arg:"-v,--invert-match" help:"select lines that do not match"`
	Context          int      `arg:"-C,--context" placeholder:"N" help:"print N lines before and after matches"`
	AfterContext     int      `arg:"-A,--after-context" placeholder:"N" help:"print N lines after matches"`
	BeforeContext    int      `arg:"-B,--before-context" placeholder:"N" help:"print N lines before matches"`
	MaxFiles         int      `arg:"--max-files" placeholder:"N" help:"limit matching files"`
	MaxSegments      int      `arg:"--max-segments" placeholder:"N" help:"limit result segments"`
	Skip             int      `arg:"--skip" placeholder:"N" help:"skip the first N ranked result files"`
	Limit            int      `arg:"--limit" placeholder:"N" help:"return at most N ranked result files (default 20; 0 = all local; servers cap a page at 100 — page further with --skip)"`
	Repos            []string `arg:"--repo,separate" placeholder:"PATTERN" help:"restrict remote repositories; repeatable"`
	Query            string   `arg:"positional" placeholder:"PATTERN"`
	Globs            []string `arg:"positional" placeholder:"PATH" help:"file, directory, or glob to search; repeat for multiple roots"`
}

func (searchArgs) Description() string {
	return "Search the local working directory, one or more PATHs, or piped stdin; add --remote (or --server) to also query the shard/router."
}

func parseSearchArgs(args []string) (*cliOptions, string, bool, error) {
	// Limit defaults to DefaultResultLimit so broad queries don't dump every match;
	// pass --limit 0 to opt back into unbounded results.
	values := searchArgs{MaxSegments: search.DefaultMaxSegments, Limit: DefaultResultLimit}
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
	if err := validateSearchArgs(&values); err != nil {
		return nil, "", false, err
	}
	params, err := buildSearchParams(parser, &values)
	if err != nil {
		return nil, "", false, err
	}

	// Structural parsing is only needed when we render segments (the default
	// display and full --json). Skip it for match-line-only output modes.
	params.SkipSegments = values.Files || values.FilesWithMatches || values.Count || values.CountByRepo || values.LineOnly || values.OnlyMatching || values.JSONMatches || params.BeforeContext > 0 || params.AfterContext > 0

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
		FilesWithMatches: values.FilesWithMatches,
		Outline:          values.Outline,
		Depth:            values.Depth,
	}, values.Server, remoteEnabled, nil
}

// validateSearchArgs rejects contradictory or out-of-range flag combinations
// before any searching starts.
func validateSearchArgs(values *searchArgs) error {
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
	if values.MaxSegments < 1 {
		return fmt.Errorf("--max-segments must be a positive number")
	}
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
		return fmt.Errorf("--count and --count-by-repo cannot be used together")
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
		MaxSegments:   values.MaxSegments,
		Skip:          values.Skip,
		Limit:         values.Limit,
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
	} else if params.Query == "" {
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

// Run executes the grepple search/get/tree command.
func Run(args []string) error {
	if len(args) > 0 {
		switch args[0] {
		case "get":
			return runGet(args[1:])
		case "tree":
			return runTree(args[1:])
		case "repos":
			return runRepos(args[1:])
		case "login":
			return runLogin(args[1:])
		case "logout":
			return runLogout(args[1:])
		case "rules":
			return runRules(args[1:])
		}
	}
	return runSearch(args)
}
