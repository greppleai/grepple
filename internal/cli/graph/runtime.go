package graph

import (
	"context"
	"fmt"
	"strings"

	"github.com/greppleai/grepple/internal/wire"
	"github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/shellquote"
	"github.com/greppleai/grepple/internal/search"
)

// metadataInput describes one graph result page.
type metadataInput struct {
	Paths                              []string
	Returned, MaxFiles, MaxOutputBytes int
	JSON                               bool
	Sources                            SourceSummary
	Truncation                         *Truncation
	NextCommand                        string
}

// diffMetadataInput describes one graph diff result.
type diffMetadataInput struct {
	BeforePath, AfterPath    string
	MaxFiles, MaxOutputBytes int
	JSON                     bool
	Before, After            Output
	Diff                     search.NavigationGraphDiff
}

// ContinuationCommand builds the uncapped continuation for a truncated graph.
func ContinuationCommand(application cliruntime.Context, mode string, paths []string, truncation *Truncation) string {
	return graphContinuationCommand(application, mode, paths, truncation)
}

func requestRemoteAnalysis(application cliruntime.Context, ctx context.Context, request wire.AnalysisRequest, server string) (wire.AnalysisResponse, error) {
	options := application.Repository().InvocationOptions()
	request.ProductionOnly = request.ProductionOnly || options.ProductionOnly
	request.NoConfigIgnore = request.NoConfigIgnore || options.NoConfigIgnore
	request.NoRepoConfig = request.NoRepoConfig || options.NoRepositoryConfig
	response, err := application.APIClient().Analysis(ctx, server, request)
	if err != nil {
		return wire.AnalysisResponse{}, err
	}
	for _, notice := range response.Notices {
		fmt.Fprintln(application.Stderr(), "analysis notice:", notice)
	}
	for _, shardError := range response.ShardErrors {
		fmt.Fprintln(application.Stderr(), "partial analysis:", shardError)
	}
	if !response.Complete {
		fmt.Fprintln(application.Stderr(), "analysis is incomplete; inspect result source counts and truncation")
	}
	return response, nil
}

func graphResultMetadata(input metadataInput) *wire.ResultMetadata {
	omitted := 0
	if input.Truncation != nil {
		omitted = input.Truncation.Skipped
	}
	return &wire.ResultMetadata{
		Scope:   wire.ResultScope{Mode: "local", Paths: normalizedScope(input.Paths, "."), ExcludedPaths: []string{}, Repositories: []string{}, ExcludedRepositories: []string{}, Languages: []string{}},
		Order:   "source",
		Page:    wire.ResultPage{Returned: input.Returned, Complete: omitted == 0 && input.Sources.Failed == 0 && input.Sources.Recovered == 0},
		Limits:  wire.ResultLimits{MaxFiles: input.MaxFiles, MaxOutputBytes: input.MaxOutputBytes, JSONByteUncapped: input.JSON},
		Omitted: wire.ResultOmissions{Sources: omitted}, Diagnostics: sourceDiagnostics(input.Sources), NextCommand: input.NextCommand,
	}
}

func graphDiffResultMetadata(application cliruntime.Context, input diffMetadataInput) *wire.ResultMetadata {
	omitted := truncatedSources(input.Before.Truncation) + truncatedSources(input.After.Truncation)
	sources := SourceSummary{
		Discovered: input.Before.Sources.Discovered + input.After.Sources.Discovered, Selected: input.Before.Sources.Selected + input.After.Sources.Selected,
		Parsed: input.Before.Sources.Parsed + input.After.Sources.Parsed, Skipped: input.Before.Sources.Skipped + input.After.Sources.Skipped,
		Failed: input.Before.Sources.Failed + input.After.Sources.Failed, Recovered: input.Before.Sources.Recovered + input.After.Sources.Recovered,
	}
	metadata := &wire.ResultMetadata{
		Scope:   wire.ResultScope{Mode: "local-diff", Paths: normalizedScope([]string{input.BeforePath, input.AfterPath}, "."), ExcludedPaths: []string{}, Repositories: []string{}, ExcludedRepositories: []string{}, Languages: []string{}},
		Order:   "semantic-identity",
		Page:    wire.ResultPage{Returned: sources.Parsed, Complete: omitted == 0 && sources.Failed == 0},
		Limits:  wire.ResultLimits{MaxFiles: input.MaxFiles, MaxOutputBytes: input.MaxOutputBytes, JSONByteUncapped: input.JSON},
		Omitted: wire.ResultOmissions{Sources: omitted}, Diagnostics: sourceDiagnostics(sources),
	}
	if !metadata.Page.Complete {
		parts := application.Repository().AppendScopeFlags([]string{"grepple", "graph", "diff", "--before", shellquote.Argument(input.BeforePath), "--after", shellquote.Argument(input.AfterPath), "--max-files", "0", "--json"})
		metadata.NextCommand = strings.Join(parts, " ")
	}
	return metadata
}

func truncatedSources(truncation *Truncation) int {
	if truncation == nil {
		return 0
	}
	return truncation.Skipped
}
