package search

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/greppleai/grepple/api"
	"github.com/greppleai/grepple/internal/cliruntime"
	rendercommand "github.com/greppleai/grepple/internal/render"
	"github.com/greppleai/grepple/linerange"
	"github.com/greppleai/grepple/search"
)

type remoteFullLineRangeMissError struct{ err error }

func (err remoteFullLineRangeMissError) Error() string { return err.err.Error() }
func (err remoteFullLineRangeMissError) Unwrap() error { return err.err }

func reportLineRangeCommandError(application cliruntime.Context, err error) error {
	var local *linerange.OutsideError
	var remote remoteFullLineRangeMissError
	if !errors.As(err, &local) && !errors.As(err, &remote) {
		return err
	}
	fmt.Fprintln(application.Stderr(), "error:", err)
	application.RequestExit(1)
	return nil
}

func runSearch(application cliruntime.Context, args []string) error {
	options, explicitServer, remote, err := parseSearchArgs(application, args)
	if err != nil || options == nil {
		return err
	}
	return executeSearch(application, options, explicitServer, remote)
}

// Execute runs a search from arguments parsed by the application-level parser.
func Execute(application cliruntime.Context, values *Args) error {
	options, explicitServer, remote, err := optionsFromArgs(application, values, nil)
	if err != nil {
		return err
	}
	return executeSearch(application, options, explicitServer, remote)
}

func executeSearch(application cliruntime.Context, options *Options, explicitServer string, remote bool) error {
	if err := configureStdinSearch(application, options); err != nil {
		return err
	}
	if options.Outline {
		printed, outlineErr := rendercommand.Outlines(rendercommand.OutlineOptions{Params: options.Params, Depth: options.Depth, JSON: options.JSON != "off", MaxOutputBytes: options.MaxOutputBytes, DefaultLimit: DefaultResultLimit, Output: application.Stdout(), ErrorOutput: application.Stderr()})
		if outlineErr == nil && !printed {
			application.RequestExit(1)
		}
		return outlineErr
	}
	if options.CountByRepo {
		// --count-by-repo is a compact per-repository probe: aggregate the full match set
		// server-side (and locally) rather than shipping and windowing file bodies.
		return runCountByRepo(application, options, explicitServer, remote)
	}

	// A server never returns more than search.MaxPageLimit files per page. When a
	// remote source is involved, clamp the request here too so the merged
	// local+remote window is consistent, and say so when the user asked for more.
	if remote && (options.Params.Limit == 0 || options.Params.Limit > search.MaxPageLimit) {
		fmt.Fprintf(application.Stderr(), "note: server pages are capped at %d files; returning the max - page further with --skip N\n", search.MaxPageLimit)
		options.Params.Limit = search.MaxPageLimit
	}

	// When results are merged from multiple sources (local + remote), each
	// source must return enough ranked candidates for the final skip/limit to be
	// applied correctly. Ask sources for the top (skip+limit) with skip=0, then
	// window the merged set below.
	child := *options
	child.Params = childWindowParams(options.Params)

	results, err := initialSearchResults(application, &child, remote)
	if err != nil {
		rendercommand.RecordLineRangeError(err, application.Configuration().ContextGuardEnabled())
		return reportLineRangeCommandError(application, err)
	}
	results, err = appendRemoteResults(application, results, &child, explicitServer, remote)
	if err != nil {
		rendercommand.RecordLineRangeError(err, application.Configuration().ContextGuardEnabled())
		return reportLineRangeCommandError(application, err)
	}
	results = resolveLocalExternalNavigation(application, results, explicitServer)

	results = sortResults(results, options.Params.Sort)
	fetched := len(results)
	totalKnown := !remote && (child.Params.Limit == 0 || fetched < child.Params.Limit)
	results = windowResults(results, options.Params)
	options.ResultMetadata = searchResultMetadata(application, options, fetched, totalKnown, remote, results)
	noteDefaultLimitCap(application, options, results)
	anchorLines, err := prepareAnchors(options, results)
	if err != nil {
		return err
	}
	options.AnchorLines = anchorLines
	// Group the selected page by repo/path for readable output.
	renderOptions := rendercommand.Options{Params: options.Params, LineOnly: options.LineOnly, OnlyMatching: options.OnlyMatching, JSON: options.JSON, Count: options.Count, FilesWithMatches: options.FilesWithMatches, MaxOutputBytes: options.MaxOutputBytes, RepeatSource: options.RepeatSource, Stdin: options.Stdin, Anchors: rendercommand.AnchorLookup(options.AnchorLines), Metadata: options.ResultMetadata}
	if err := rendercommand.Search(rendercommand.SearchOptions{Options: renderOptions, Output: application.Stdout(), ErrorOutput: application.Stderr(), ContextEnabled: application.Configuration().ContextGuardEnabled(), InlineThreshold: application.Configuration().InlineOutputThreshold()}, results); err != nil {
		return err
	}
	return setSearchExit(application, results)
}

func initialSearchResults(application cliruntime.Context, options *Options, remote bool) ([]api.FileResult, error) {
	if remote && options.Params.At != "" {
		return nil, nil
	}
	return searchLocal(application, options)
}

func configureStdinSearch(application cliruntime.Context, options *Options) error {
	// Piped stdin has no stable path, so automatic anchors are disabled.
	options.Stdin = stdinSearch(application, options)
	if options.Stdin {
		options.Anchors = false
	}
	return nil
}

func appendRemoteResults(application cliruntime.Context, results []api.FileResult, options *Options, explicitServer string, remote bool) ([]api.FileResult, error) {
	if !remote {
		return results, nil
	}
	server := application.Configuration().ServerDefault(explicitServer)
	if repo := application.Repository().Current(); repo != "" {
		options.Params.ExcludeRepo = appendUnique(options.Params.ExcludeRepo, repo)
	}
	remoteResults, err := searchRemote(application, options, server)
	if err != nil {
		return nil, err
	}
	return append(results, remoteResults...), nil
}

// childWindowParams returns source-query parameters that fetch the top
// skip+limit window (with skip=0) so the final window applies to the merged
// result set.
func childWindowParams(params search.Params) search.Params {
	skip := params.Skip
	params.Skip = 0
	if params.Limit > 0 {
		params.Limit += skip
	}
	return params
}

// sortResults applies the same deterministic file ranking to merged local and
// remote results. Match count uses path as a stable tie-breaker.
func sortResults(results []api.FileResult, strategy string) []api.FileResult {
	sort.SliceStable(results, func(i, j int) bool {
		if strategy == search.ResultSortMatches && len(results[i].Matches) != len(results[j].Matches) {
			return len(results[i].Matches) > len(results[j].Matches)
		}
		if results[i].Repo != results[j].Repo {
			return results[i].Repo < results[j].Repo
		}
		return results[i].Path < results[j].Path
	})
	return results
}

// noteDefaultLimitCap tells the user how to see more when the default --limit
// is what capped the output (not an explicit --limit or --max-files).
func noteDefaultLimitCap(application cliruntime.Context, options *Options, results []api.FileResult) {
	if options.Params.Limit == DefaultResultLimit && options.Params.MaxFiles == 0 && len(results) == DefaultResultLimit {
		fmt.Fprintf(application.Stderr(), "note: showing the first %d files (default --limit); raise with --limit N and page with --skip N (servers cap a page at %d; --limit 0 = all local)\n", DefaultResultLimit, search.MaxPageLimit)
	}
}

// windowResults drops the first Skip results and caps the remainder to the
// effective limit (the smaller of Limit and MaxFiles).
func windowResults(results []api.FileResult, params search.Params) []api.FileResult {
	if params.Skip > 0 {
		if params.Skip >= len(results) {
			return results[:0]
		}
		results = results[params.Skip:]
	}
	limit := params.Limit
	if params.MaxFiles > 0 && (limit == 0 || params.MaxFiles < limit) {
		limit = params.MaxFiles
	}
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results
}

// stdinSearch reports whether the local search should read standard input
// instead of walking the filesystem: stdin is a pipe or redirect (not a
// terminal) and the request is a content search with no path/glob argument —
// mirroring grep/ripgrep, where `cat f | grepple pattern` searches the stream.
// Filename listing (--files) and --outline always operate on the filesystem,
// as does any search with an explicit path/glob.
func stdinSearch(application cliruntime.Context, o *Options) bool {
	if o.Outline || o.Params.Files || o.Params.At != "" || len(o.Params.Globs) > 0 {
		return false
	}
	stream, ok := application.Stdin().(interface{ Stat() (os.FileInfo, error) })
	if !ok {
		return false
	}
	info, err := stream.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice == 0
}

// searchStdin reads piped standard input and searches it as a single virtual
// file named <stdin>. It must run at most once per process — the stream can
// only be consumed once.
func searchStdin(application cliruntime.Context, params search.Params) ([]search.FileMatch, error) {
	data, err := io.ReadAll(application.Stdin())
	if err != nil {
		return nil, err
	}
	match, err := search.Content(params, search.StdinPath, data)
	if err != nil || match == nil {
		return nil, err
	}
	return []search.FileMatch{*match}, nil
}

func searchLocal(application cliruntime.Context, options *Options) ([]api.FileResult, error) {
	params := options.Params
	if params.At != "" {
		match, err := search.At(params)
		if err != nil {
			return nil, err
		}
		return search.BuildResults([]search.FileMatch{*match}, params.BeforeContext, params.AfterContext, true), nil
	}
	if params.Files {
		// Filename glob listing only. --files-with-matches is a content search, so
		// it falls through to Files below and renders paths from the matches.
		paths, err := search.ListFilePaths(params, nil)
		if err != nil {
			return nil, err
		}
		results := make([]api.FileResult, 0, len(paths))
		for _, path := range paths {
			results = append(results, api.FileResult{
				Path:     path,
				Matches:  []api.ResultMatch{},
				Segments: []api.ResultSegment{},
			})
		}
		return results, nil
	}

	if options.Stdin {
		matches, err := searchStdin(application, params)
		if err != nil {
			return nil, err
		}
		return search.BuildResults(matches, params.BeforeContext, params.AfterContext, !params.SkipSegments), nil
	}
	matches, err := search.Files(params, nil)
	if err != nil {
		return nil, err
	}
	return search.BuildResults(matches, params.BeforeContext, params.AfterContext, !params.SkipSegments), nil
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func setSearchExit(application cliruntime.Context, results []api.FileResult) error {
	if len(results) == 0 {
		application.RequestExit(1)
	}
	return nil
}

// runCountByRepo produces per-repository match tallies. Counts must be complete, so
// the local scan and the remote probe both run unbounded (no skip/limit window)
// and the remote side aggregates on the shards to keep the payload tiny.
func runCountByRepo(application cliruntime.Context, options *Options, explicitServer string, remote bool) error {
	type agg struct{ files, matches int }
	repos := map[string]*agg{}
	add := func(repo string, files, matches int) {
		a := repos[repo]
		if a == nil {
			a = &agg{}
			repos[repo] = a
		}
		a.files += files
		a.matches += matches
	}

	// Local matches (unbounded).
	local := options.Params
	local.Skip, local.Limit, local.MaxFiles = 0, 0, 0
	local.CountByRepo = false
	var matches []search.FileMatch
	var err error
	if options.Stdin {
		matches, err = searchStdin(application, local)
	} else {
		matches, err = search.Files(local, nil)
	}
	if err != nil {
		return err
	}
	for _, m := range matches {
		add(search.RepoID(m.DisplayPath), 1, len(m.MatchLines))
	}

	// Remote counts (only when the user opted in).
	if remote {
		server := application.Configuration().ServerDefault(explicitServer)
		counts, err := countRemote(application, options, server)
		if err != nil {
			return err
		}
		for _, c := range counts {
			add(c.Repo, c.Files, c.Matches)
		}
	}

	out := make([]api.RepoCount, 0, len(repos))
	for repo, a := range repos {
		out = append(out, api.RepoCount{Repo: repo, Files: a.files, Matches: a.matches})
	}
	maxBytes := 0
	if options.JSON == "off" {
		maxBytes = options.MaxOutputBytes
	}
	if err := rendercommand.RepoCounts(out, application.Stdout(), options.JSON != "off", options.CountSummary, maxBytes); err != nil {
		return err
	}
	if len(out) == 0 {
		application.RequestExit(1)
	}
	return nil
}
