package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/greppleai/grepple/api"
	"github.com/greppleai/grepple/search"
)

func resultScope(mode string, paths, repositories, languages []string) api.ResultScope {
	return api.ResultScope{
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

func sourceResultDiagnostics(sources navigationSourceSummary) []api.ResultDiagnostic {
	diagnostics := []api.ResultDiagnostic{}
	if sources.Failed > 0 {
		diagnostics = append(diagnostics, api.ResultDiagnostic{Code: "source-failed", Message: fmt.Sprintf("%d selected source files failed analysis", sources.Failed)})
	}
	if sources.Recovered > 0 {
		diagnostics = append(diagnostics, api.ResultDiagnostic{Code: "source-recovered", Message: fmt.Sprintf("%d source files required parser recovery", sources.Recovered)})
	}
	if sources.Skipped > 0 {
		diagnostics = append(diagnostics, api.ResultDiagnostic{Code: "source-skipped", Message: fmt.Sprintf("%d discovered source files were unsupported or binary", sources.Skipped)})
	}
	return diagnostics
}

func graphResultMetadata(paths []string, returned, maxFiles, maxOutputBytes int, jsonMode bool, sources navigationSourceSummary, truncation *navigationGraphTruncation, nextCommand string) *api.ResultMetadata {
	omitted := 0
	if truncation != nil {
		omitted = truncation.Skipped
	}
	return &api.ResultMetadata{
		Scope:   resultScope("local", paths, nil, nil),
		Order:   "source",
		Page:    api.ResultPage{Skip: 0, Limit: 0, Returned: returned, Complete: omitted == 0 && sources.Failed == 0 && sources.Recovered == 0},
		Limits:  api.ResultLimits{MaxFiles: maxFiles, MaxOutputBytes: maxOutputBytes, JSONByteUncapped: jsonMode},
		Omitted: api.ResultOmissions{Sources: omitted}, Diagnostics: sourceResultDiagnostics(sources), NextCommand: nextCommand,
	}
}

func searchResultMetadata(options *cliOptions, total int, totalKnown, remote bool, results []api.FileResult) *api.ResultMetadata {
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
	metadata := &api.ResultMetadata{
		Scope:   resultScope(mode, scopePaths, options.Params.Repo, nil),
		Order:   options.Params.Sort,
		Page:    api.ResultPage{Skip: options.Params.Skip, Limit: options.Params.Limit, Returned: returned, Total: totalPointer, Complete: complete},
		Limits:  api.ResultLimits{MaxFiles: options.Params.MaxFiles, MaxOutputBytes: options.MaxOutputBytes, JSONByteUncapped: options.JSON != "off"},
		Omitted: api.ResultOmissions{Files: omitted}, Diagnostics: searchResultDiagnostics(options, results),
	}
	metadata.Scope.ExcludedRepositories = normalizedResultScope(options.Params.ExcludeRepo, "")
	if !pageComplete && returned > 0 && options.Params.Limit > 0 {
		metadata.NextCommand = searchNextCommand(options, options.Params.Skip+returned, remote)
	}
	return metadata
}

func searchResultDiagnostics(options *cliOptions, results []api.FileResult) []api.ResultDiagnostic {
	diagnostics := []api.ResultDiagnostic{}
	if options.Params.MaxFiles > 0 {
		diagnostics = append(diagnostics, api.ResultDiagnostic{Code: "source-cap", Message: fmt.Sprintf("source selection is capped at %d files", options.Params.MaxFiles)})
	}
	if analysis := searchSourceAnalysis(results); analysis != nil {
		if analysis.Failed > 0 {
			diagnostics = append(diagnostics, api.ResultDiagnostic{Code: "source-failed", Message: fmt.Sprintf("%d returned files failed structural analysis", analysis.Failed)})
		}
		if analysis.Recovered > 0 {
			diagnostics = append(diagnostics, api.ResultDiagnostic{Code: "source-recovered", Message: fmt.Sprintf("%d returned files required parser recovery", analysis.Recovered)})
		}
		if analysis.Unsupported > 0 {
			diagnostics = append(diagnostics, api.ResultDiagnostic{Code: "source-unsupported", Message: fmt.Sprintf("%d returned files lack structural language support", analysis.Unsupported)})
		}
	}
	return diagnostics
}

func searchAnalysisIncomplete(results []api.FileResult) bool {
	analysis := searchSourceAnalysis(results)
	return analysis != nil && (analysis.Failed > 0 || analysis.Recovered > 0 || analysis.Unsupported > 0)
}

func searchNextCommand(options *cliOptions, skip int, remote bool) string {
	parts := appendActiveRepositoryScopeFlags([]string{"grepple", "search"})
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
		parts = append(parts, "--repo", quoteCommandArgument(repository))
	}
	if options.Params.Sort == search.ResultSortMatches {
		parts = append(parts, "--sort", search.ResultSortMatches)
	}
	for _, repository := range options.Params.ExcludeRepo {
		parts = append(parts, "--exclude-repo", quoteCommandArgument(repository))
	}
	parts = append(parts, "--skip", fmt.Sprint(skip), "--limit", fmt.Sprint(options.Params.Limit), "--json", quoteCommandArgument(options.Params.Query))
	for _, path := range options.Params.Globs {
		parts = append(parts, quoteCommandArgument(path))
	}
	return strings.Join(parts, " ")
}

func graphContinuationCommand(mode string, paths []string, truncation *navigationGraphTruncation) string {
	if truncation == nil {
		return ""
	}
	parts := appendActiveRepositoryScopeFlags([]string{"grepple", "graph"})
	if mode != "graph" {
		parts = append(parts, mode)
	}
	parts = append(parts, "--max-files", "0", "--json")
	for _, path := range normalizedResultScope(paths, ".") {
		parts = append(parts, quoteCommandArgument(path))
	}
	return strings.Join(parts, " ")
}

func gritResultMetadata(values gritArgs, response api.GritResponse, remote bool) *api.ResultMetadata {
	total := response.Total
	pageComplete := values.Skip+len(response.Findings) >= total
	omittedSources := 0
	diagnostics := make([]api.ResultDiagnostic, 0, len(response.Diagnostics)+len(response.Truncations)+len(response.ShardErrors))
	for _, diagnostic := range response.Diagnostics {
		diagnostics = append(diagnostics, api.ResultDiagnostic{Code: diagnostic.Code, Message: diagnostic.Message})
	}
	for _, truncation := range response.Truncations {
		omittedSources += truncation.Skipped
		diagnostics = append(diagnostics, api.ResultDiagnostic{Code: "truncated-" + truncation.Reason, Message: fmt.Sprintf("%d items omitted by %s=%d", truncation.Skipped, truncation.Reason, truncation.Limit)})
	}
	for _, shardError := range response.ShardErrors {
		diagnostics = append(diagnostics, api.ResultDiagnostic{Code: "shard-error", Message: shardError})
	}
	mode := "local"
	if remote {
		mode = "local+remote"
	}
	metadata := &api.ResultMetadata{
		Scope:   resultScope(mode, values.Globs, values.Repositories, gritResultLanguages(response)),
		Order:   "repository-path-range",
		Page:    api.ResultPage{Skip: values.Skip, Limit: values.Limit, Returned: len(response.Findings), Total: &total, Complete: pageComplete && len(response.Truncations) == 0 && len(response.ShardErrors) == 0},
		Limits:  api.ResultLimits{MaxFiles: values.MaxFiles, MaxOutputBytes: values.MaxOutputBytes, MaxSourceBytes: values.MaxSourceBytes, MaxTotalBytes: values.MaxTotalBytes, JSONByteUncapped: values.JSON},
		Omitted: api.ResultOmissions{Findings: max(0, total-len(response.Findings)), Sources: omittedSources}, Diagnostics: diagnostics,
	}
	metadata.Scope.ExcludedPaths = normalizedResultScope(values.ExcludeGlobs, "")
	metadata.Scope.ExcludedRepositories = normalizedResultScope(values.ExcludeRepositories, "")
	if !pageComplete || omittedSources > 0 {
		metadata.NextCommand = gritContinuationCommand(values, values.Skip+len(response.Findings), omittedSources > 0)
	}
	return metadata
}

func gritResultLanguages(response api.GritResponse) []string {
	languages := []string{response.Metadata.Language}
	for _, finding := range response.Findings {
		languages = append(languages, finding.Language)
	}
	return normalizedResultScope(languages, "")
}

func gritContinuationCommand(values gritArgs, nextSkip int, removeSourceCaps bool) string {
	parts := appendActiveRepositoryScopeFlags([]string{"grepple", "grit", "--json"})
	if values.Remote {
		parts = append(parts, "--remote")
	}
	if values.Server != "" {
		parts = append(parts, "--server", quoteCommandArgument(values.Server))
	}
	if removeSourceCaps {
		parts = append(parts, "--max-files", "0", "--max-total-bytes", "0")
		nextSkip = values.Skip
	} else {
		parts = append(parts, "--skip", fmt.Sprint(nextSkip), "--limit", fmt.Sprint(values.Limit))
	}
	for _, repository := range values.Repositories {
		parts = append(parts, "--repo", quoteCommandArgument(repository))
	}
	for _, repository := range values.ExcludeRepositories {
		parts = append(parts, "--exclude-repo", quoteCommandArgument(repository))
	}
	for _, glob := range values.ExcludeGlobs {
		parts = append(parts, "--exclude-glob", quoteCommandArgument(glob))
	}
	if values.QueryFile != "" {
		parts = append(parts, "--query-file", quoteCommandArgument(values.QueryFile))
	} else {
		parts = append(parts, quoteCommandArgument(values.Query))
	}
	for _, glob := range values.Globs {
		parts = append(parts, quoteCommandArgument(glob))
	}
	return strings.Join(parts, " ")
}

func graphDiffResultMetadata(values graphDiffArgs, before, after navigationGraphOutput) *api.ResultMetadata {
	omitted := graphTruncatedSources(before.Truncation) + graphTruncatedSources(after.Truncation)
	sources := navigationSourceSummary{
		Discovered: before.Sources.Discovered + after.Sources.Discovered, Selected: before.Sources.Selected + after.Sources.Selected,
		Parsed: before.Sources.Parsed + after.Sources.Parsed, Skipped: before.Sources.Skipped + after.Sources.Skipped,
		Failed: before.Sources.Failed + after.Sources.Failed, Recovered: before.Sources.Recovered + after.Sources.Recovered,
	}
	metadata := &api.ResultMetadata{
		Scope:   resultScope("local-diff", []string{values.Before, values.After}, nil, nil),
		Order:   "semantic-identity",
		Page:    api.ResultPage{Returned: sources.Parsed, Complete: omitted == 0 && sources.Failed == 0},
		Limits:  api.ResultLimits{MaxFiles: values.MaxFiles, MaxOutputBytes: values.MaxOutputBytes, JSONByteUncapped: values.JSON},
		Omitted: api.ResultOmissions{Sources: omitted}, Diagnostics: sourceResultDiagnostics(sources),
	}
	if !metadata.Page.Complete {
		parts := appendActiveRepositoryScopeFlags([]string{"grepple", "graph", "diff", "--before", quoteCommandArgument(values.Before), "--after", quoteCommandArgument(values.After), "--max-files", "0", "--json"})
		metadata.NextCommand = strings.Join(parts, " ")
	}
	return metadata
}

func graphTruncatedSources(truncation *navigationGraphTruncation) int {
	if truncation == nil {
		return 0
	}
	return truncation.Skipped
}
