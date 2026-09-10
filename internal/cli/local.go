package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/greppleai/grepple/api"
	"github.com/greppleai/grepple/search"
)

func runSearch(args []string) error {
	options, explicitServer, remote, err := parseSearchArgs(args)
	if err != nil || options == nil {
		return err
	}
	if err := configureStdinSearch(options); err != nil {
		return err
	}
	if options.Outline {
		return runOutline(options)
	}
	if options.CountByRepo {
		// --count-by-repo is a compact per-repository probe: aggregate the full match set
		// server-side (and locally) rather than shipping and windowing file bodies.
		return runCountByRepo(options, explicitServer, remote)
	}

	// A server never returns more than search.MaxPageLimit files per page. When a
	// remote source is involved, clamp the request here too so the merged
	// local+remote window is consistent, and say so when the user asked for more.
	if remote && (options.Params.Limit == 0 || options.Params.Limit > search.MaxPageLimit) {
		fmt.Fprintf(os.Stderr, "note: server pages are capped at %d files; returning the max - page further with --skip N\n", search.MaxPageLimit)
		options.Params.Limit = search.MaxPageLimit
	}

	// When results are merged from multiple sources (local + remote), each
	// source must return enough ranked candidates for the final skip/limit to be
	// applied correctly. Ask sources for the top (skip+limit) with skip=0, then
	// window the merged set below.
	child := *options
	child.Params = childWindowParams(options.Params)

	results, err := searchLocal(&child)
	if err != nil {
		return err
	}
	results, err = appendRemoteResults(results, &child, explicitServer, remote)
	if err != nil {
		return err
	}

	results = windowResults(sortByPath(results), options.Params)
	noteDefaultLimitCap(options, results)
	if err := prepareResultAnchors(options, results); err != nil {
		return err
	}
	// Group the selected page by repo/path for readable output.
	return renderResults(options, sortByPath(results))
}

func configureStdinSearch(options *cliOptions) error {
	// Piped stdin with no path/glob argument switches the local source from the
	// filesystem to the stream (grep/ripgrep behavior). Automatic anchors are
	// disabled because stdin has no stable path; an explicit --anchors remains an error.
	options.Stdin = stdinSearch(options)
	if options.AnchorsDefaulted && options.Stdin {
		options.Anchors = false
		options.AnchorsDefaulted = false
	}
	if options.Anchors && options.Stdin {
		return fmt.Errorf("--anchors requires local files and cannot anchor piped stdin")
	}
	return nil
}

func appendRemoteResults(results []api.FileResult, options *cliOptions, explicitServer string, remote bool) ([]api.FileResult, error) {
	if !remote {
		return results, nil
	}
	server := serverDefault(explicitServer)
	if repo := currentGitRepoID(); repo != "" {
		options.Params.ExcludeRepo = appendUnique(options.Params.ExcludeRepo, repo)
	}
	remoteResults, err := searchRemote(options, server)
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

// sortByPath orders results deterministically (repo, then path), matching the
// shard/router so local + remote compose into one stable, reproducible list
// and skip/limit paging is consistent across pages. No relevance ranking.
func sortByPath(results []api.FileResult) []api.FileResult {
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Repo != results[j].Repo {
			return results[i].Repo < results[j].Repo
		}
		return results[i].Path < results[j].Path
	})
	return results
}

// noteDefaultLimitCap tells the user how to see more when the default --limit
// is what capped the output (not an explicit --limit or --max-files).
func noteDefaultLimitCap(options *cliOptions, results []api.FileResult) {
	if options.Params.Limit == DefaultResultLimit && options.Params.MaxFiles == 0 && len(results) == DefaultResultLimit {
		fmt.Fprintf(os.Stderr, "note: showing the first %d files (default --limit); raise with --limit N and page with --skip N (servers cap a page at %d; --limit 0 = all local)\n", DefaultResultLimit, search.MaxPageLimit)
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
func stdinSearch(o *cliOptions) bool {
	if o.Outline || o.Params.Files || o.Params.At != "" || len(o.Params.Globs) > 0 {
		return false
	}
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice == 0
}

// searchStdin reads piped standard input and searches it as a single virtual
// file named <stdin>. It must run at most once per process — the stream can
// only be consumed once.
func searchStdin(params search.Params) ([]search.FileMatch, error) {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return nil, err
	}
	match, err := search.Content(params, search.StdinPath, data)
	if err != nil || match == nil {
		return nil, err
	}
	return []search.FileMatch{*match}, nil
}

func searchLocal(options *cliOptions) ([]api.FileResult, error) {
	params := options.Params
	if params.At != "" {
		match, err := search.At(params)
		if err != nil {
			return nil, err
		}
		return search.BuildResults([]search.FileMatch{*match}, 0, 0, params.MaxSegments, true), nil
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
		matches, err := searchStdin(params)
		if err != nil {
			return nil, err
		}
		return search.BuildResults(matches, params.BeforeContext, params.AfterContext, params.MaxSegments, !params.SkipSegments), nil
	}
	matches, err := search.Files(params, nil)
	if err != nil {
		return nil, err
	}
	return search.BuildResults(matches, params.BeforeContext, params.AfterContext, params.MaxSegments, !params.SkipSegments), nil
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func setSearchExit(results []api.FileResult) error {
	if len(results) == 0 {
		setExit(1)
	}
	return nil
}

func setExit(code int) {
	if code != 0 {
		os.Exit(code)
	}
}

func filePathObjects(results []api.FileResult) []map[string]string {
	objects := make([]map[string]string, 0, len(results))
	for _, result := range results {
		object := map[string]string{"path": result.Path}
		if result.Repo != "" {
			object["repo"] = result.Repo
		}
		objects = append(objects, object)
	}
	return objects
}

// runCountByRepo produces per-repository match tallies. Counts must be complete, so
// the local scan and the remote probe both run unbounded (no skip/limit window)
// and the remote side aggregates on the shards to keep the payload tiny.
func runCountByRepo(options *cliOptions, explicitServer string, remote bool) error {
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
		matches, err = searchStdin(local)
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
		server := serverDefault(explicitServer)
		counts, err := countRemote(options, server)
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
	renderer := repoCountRenderer{output: outputForOptions(options), json: options.JSON != "off"}
	if err := renderer.Render(out); err != nil && !errors.Is(err, errOutputTruncated) {
		return err
	}
	if len(out) == 0 {
		setExit(1) // no matches: exit 1, consistent with grep
	}
	return nil
}
