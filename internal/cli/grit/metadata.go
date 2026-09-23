package grit

import (
	"fmt"
	"sort"
	"strings"

	"github.com/greppleai/grepple/api"
	"github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/shellquote"
)

func gritResultMetadata(application cliruntime.Context, values gritArgs, response api.GritResponse, remote bool) *api.ResultMetadata {
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
		Scope:   api.ResultScope{Mode: mode, Paths: normalizedScope(values.Globs, "."), ExcludedPaths: normalizedScope(values.ExcludeGlobs, ""), Repositories: normalizedScope(values.Repositories, ""), ExcludedRepositories: normalizedScope(values.ExcludeRepositories, ""), Languages: gritResultLanguages(response)},
		Order:   "repository-path-range",
		Page:    api.ResultPage{Skip: values.Skip, Limit: values.Limit, Returned: len(response.Findings), Total: &total, Complete: pageComplete && len(response.Truncations) == 0 && len(response.ShardErrors) == 0},
		Limits:  api.ResultLimits{MaxFiles: values.MaxFiles, MaxOutputBytes: values.MaxOutputBytes, MaxSourceBytes: values.MaxSourceBytes, MaxTotalBytes: values.MaxTotalBytes, JSONByteUncapped: values.JSON},
		Omitted: api.ResultOmissions{Findings: max(0, total-len(response.Findings)), Sources: omittedSources}, Diagnostics: diagnostics,
	}
	if !pageComplete || omittedSources > 0 {
		metadata.NextCommand = gritContinuationCommand(application, values, values.Skip+len(response.Findings), omittedSources > 0)
	}
	return metadata
}

func gritResultLanguages(response api.GritResponse) []string {
	languages := []string{response.Metadata.Language}
	for _, finding := range response.Findings {
		languages = append(languages, finding.Language)
	}
	return normalizedScope(languages, "")
}

func normalizedScope(values []string, fallback string) []string {
	if len(values) == 0 {
		if fallback == "" {
			return []string{}
		}
		return []string{fallback}
	}
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}

func gritContinuationCommand(application cliruntime.Context, values gritArgs, nextSkip int, removeSourceCaps bool) string {
	parts := application.Repository().AppendScopeFlags([]string{"grepple", "grit", "--json"})
	if values.Remote {
		parts = append(parts, "--remote")
	}
	if values.Server != "" {
		parts = append(parts, "--server", shellquote.Argument(values.Server))
	}
	if removeSourceCaps {
		parts = append(parts, "--max-files", "0", "--max-total-bytes", "0")
		nextSkip = values.Skip
	} else {
		parts = append(parts, "--skip", fmt.Sprint(nextSkip), "--limit", fmt.Sprint(values.Limit))
	}
	for _, repository := range values.Repositories {
		parts = append(parts, "--repo", shellquote.Argument(repository))
	}
	for _, repository := range values.ExcludeRepositories {
		parts = append(parts, "--exclude-repo", shellquote.Argument(repository))
	}
	for _, glob := range values.ExcludeGlobs {
		parts = append(parts, "--exclude-glob", shellquote.Argument(glob))
	}
	if values.QueryFile != "" {
		parts = append(parts, "--query-file", shellquote.Argument(values.QueryFile))
	} else {
		parts = append(parts, shellquote.Argument(values.Query))
	}
	for _, glob := range values.Globs {
		parts = append(parts, shellquote.Argument(glob))
	}
	return strings.Join(parts, " ")
}
