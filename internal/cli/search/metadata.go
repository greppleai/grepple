package search

import (
	"fmt"
	"sort"
	"strings"

	"github.com/greppleai/grepple/internal/wire"
	"github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/resultanalysis"
	"github.com/greppleai/grepple/internal/shellquote"
	"github.com/greppleai/grepple/internal/search"
)

func resultScope(mode string, paths, repositories, languages []string) wire.ResultScope {
	return wire.ResultScope{
		Mode: mode, Paths: normalizedResultScope(paths, "."), ExcludedPaths: []string{},
		Repositories: normalizedResultScope(repositories, ""), ExcludedRepositories: []string{}, Languages: normalizedResultScope(languages, ""),
	}
}

func normalizedResultScope(values []string, fallback string) []string {
	if len(values) == 0 {
		if fallback == "" {
			return []string{}
		}
		return []string{fallback}
	}
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}
func searchResultMetadata(application cliruntime.Context, options *Options, total int, totalKnown, remote bool, results []wire.FileResult) *wire.ResultMetadata {
	returned := len(results)
	var totalPointer *int
	if totalKnown {
		totalCopy := total
		totalPointer = &totalCopy
	}
	pageComplete := totalKnown && options.Params.Skip+returned >= total
	complete := pageComplete && !searchAnalysisIncomplete(results)
	omitted := 0
	if totalKnown && total > returned {
		omitted = total - returned
	}
	mode := "local"
	if remote {
		mode = "local+remote"
	}
	scopePaths := options.Params.Globs
	if options.Stdin {
		scopePaths = []string{"<stdin>"}
	}
	metadata := &wire.ResultMetadata{
		Scope:   resultScope(mode, scopePaths, options.Params.Repo, nil),
		Order:   options.Params.Sort,
		Page:    wire.ResultPage{Skip: options.Params.Skip, Limit: options.Params.Limit, Returned: returned, Total: totalPointer, Complete: complete},
		Limits:  wire.ResultLimits{MaxFiles: options.Params.MaxFiles, MaxOutputBytes: options.MaxOutputBytes, JSONByteUncapped: options.JSON != "off"},
		Omitted: wire.ResultOmissions{Files: omitted}, Diagnostics: searchResultDiagnostics(options, results),
	}
	metadata.Scope.ExcludedRepositories = normalizedResultScope(options.Params.ExcludeRepo, "")
	if !pageComplete && returned > 0 && options.Params.Limit > 0 {
		metadata.NextCommand = searchNextCommand(application, options, options.Params.Skip+returned, remote)
	}
	return metadata
}

func searchResultDiagnostics(options *Options, results []wire.FileResult) []wire.ResultDiagnostic {
	diagnostics := []wire.ResultDiagnostic{}
	if options.Params.MaxFiles > 0 {
		diagnostics = append(diagnostics, wire.ResultDiagnostic{Code: "source-cap", Message: fmt.Sprintf("source selection is capped at %d files", options.Params.MaxFiles)})
	}
	if analysis := resultanalysis.Sources(results); analysis != nil {
		if analysis.Failed > 0 {
			diagnostics = append(diagnostics, wire.ResultDiagnostic{Code: "source-failed", Message: fmt.Sprintf("%d returned files failed structural analysis", analysis.Failed)})
		}
		if analysis.Recovered > 0 {
			diagnostics = append(diagnostics, wire.ResultDiagnostic{Code: "source-recovered", Message: fmt.Sprintf("%d returned files required parser recovery", analysis.Recovered)})
		}
		if analysis.Unsupported > 0 {
			diagnostics = append(diagnostics, wire.ResultDiagnostic{Code: "source-unsupported", Message: fmt.Sprintf("%d returned files lack structural language support", analysis.Unsupported)})
		}
	}
	return diagnostics
}

func searchAnalysisIncomplete(results []wire.FileResult) bool {
	analysis := resultanalysis.Sources(results)
	return analysis != nil && (analysis.Failed > 0 || analysis.Recovered > 0 || analysis.Unsupported > 0)
}

func searchNextCommand(application cliruntime.Context, options *Options, skip int, remote bool) string {
	parts := application.Repository().AppendScopeFlags([]string{"grepple", "search"})
	if remote {
		parts = append(parts, "--remote")
	}
	if !options.Params.Regex {
		parts = append(parts, "-F")
	}
	if options.Params.IgnoreCase {
		parts = append(parts, "-i")
	}
	if options.Params.InvertMatch {
		parts = append(parts, "-v")
	}
	if options.Params.NoRelated {
		parts = append(parts, "--no-related")
	} else if options.Params.Related && options.Params.FollowRelated == 0 {
		parts = append(parts, "--related")
	} else if options.Params.FollowRelated > 1 {
		parts = append(parts, "--follow-related", fmt.Sprint(options.Params.FollowRelated))
	}
	for _, repository := range options.Params.Repo {
		parts = append(parts, "--repo", shellquote.Argument(repository))
	}
	if options.Params.Sort == search.ResultSortMatches {
		parts = append(parts, "--sort", search.ResultSortMatches)
	}
	for _, repository := range options.Params.ExcludeRepo {
		parts = append(parts, "--exclude-repo", shellquote.Argument(repository))
	}
	parts = append(parts, "--skip", fmt.Sprint(skip), "--limit", fmt.Sprint(options.Params.Limit), "--json", shellquote.Argument(options.Params.Query))
	for _, path := range options.Params.Globs {
		parts = append(parts, shellquote.Argument(path))
	}
	return strings.Join(parts, " ")
}
