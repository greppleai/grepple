package cli

import (
	"fmt"
	"strings"

	"github.com/greppleai/grepple/api"
	boundariescommand "github.com/greppleai/grepple/internal/cli/boundaries"
	"github.com/greppleai/grepple/internal/shellquote"
	"github.com/greppleai/grepple/internal/storagepaths"
	"github.com/greppleai/grepple/search"
)

type boundariesOutput = boundariescommand.Report

const defaultBoundaryPolicyPath = boundariescommand.DefaultPolicyPath

func boundariesDependencies() boundariescommand.Dependencies {
	return boundariescommand.Dependencies{ResolvePaths: navigationInputPaths, BuildGraph: buildBoundaryGraph, CacheDirectory: func() string { return storagepaths.Cache(mustGetwd()) }, Remote: requestAnalysisRemote, ServerDefault: serverDefault, Metadata: boundaryResultMetadata}
}
func runBoundaries(args []string) error {
	return boundariescommand.New(boundariesDependencies()).Run(args)
}
func buildCachedBoundaryGraph(paths []string, maxFiles int, useCache bool) (boundariescommand.GraphOutput, string, error) {
	return boundariescommand.BuildCachedGraph(paths, maxFiles, useCache, boundariesDependencies())
}

func buildBoundaryGraph(paths []string, maxFiles int, options search.NavigationBuildOptions) boundariescommand.GraphOutput {
	graph := buildNavigationGraphOutputFromPathsWithOptions(paths, maxFiles, options)
	var truncation *boundariescommand.Truncation
	if graph.Truncation != nil {
		truncation = &boundariescommand.Truncation{Reason: graph.Truncation.Reason, Limit: graph.Truncation.Limit, Skipped: graph.Truncation.Skipped}
	}
	return boundariescommand.GraphOutput{Files: graph.Files, Sources: boundariescommand.SourceSummary{Discovered: graph.Sources.Discovered, Selected: graph.Sources.Selected, Parsed: graph.Sources.Parsed, Skipped: graph.Sources.Skipped, Failed: graph.Sources.Failed, Recovered: graph.Sources.Recovered}, Declarations: graph.Declarations, Calls: graph.Calls, Fields: graph.Fields, TypeUsages: graph.TypeUsages, MemberAccesses: graph.MemberAccesses, Truncation: truncation}
}

func boundaryResultMetadata(input boundariescommand.MetadataInput) *api.ResultMetadata {
	report := input.Report
	total := len(report.Candidates) + len(report.TypeBoundaries) + len(report.FacadeBypasses)
	returned := total
	if !input.JSON {
		returned = visibleBoundaryCount(len(report.Candidates), input.Limit) + visibleBoundaryCount(len(report.TypeBoundaries), input.Limit) + visibleBoundaryCount(len(report.FacadeBypasses), input.Limit)
	}
	omittedFindings := total - returned
	omittedSources := 0
	if report.Truncation != nil {
		omittedSources = report.Truncation.Skipped
	}
	sources := navigationSourceSummary{Discovered: report.Sources.Discovered, Selected: report.Sources.Selected, Parsed: report.Sources.Parsed, Skipped: report.Sources.Skipped, Failed: report.Sources.Failed, Recovered: report.Sources.Recovered}
	metadata := &api.ResultMetadata{Scope: resultScope("local", report.Paths, nil, nil), Order: "risk-breadth", Page: api.ResultPage{Limit: input.Limit, Returned: returned, Total: &total, Complete: omittedFindings == 0 && omittedSources == 0 && report.Sources.Failed == 0 && report.Sources.Recovered == 0}, Limits: api.ResultLimits{MaxFiles: input.MaxFiles, MaxOutputBytes: input.MaxOutputBytes, JSONByteUncapped: input.JSON}, Omitted: api.ResultOmissions{Sources: omittedSources, Findings: omittedFindings}, Diagnostics: sourceResultDiagnostics(sources)}
	if omittedFindings > 0 || omittedSources > 0 {
		metadata.NextCommand = boundaryContinuationCommand(input, omittedSources > 0)
	}
	return metadata
}
func visibleBoundaryCount(total, limit int) int {
	if limit == 0 || total < limit {
		return total
	}
	return limit
}
func boundaryContinuationCommand(input boundariescommand.MetadataInput, removeSourceCap bool) string {
	parts := appendActiveRepositoryScopeFlags([]string{"grepple", "boundaries", "--json", "--min-occurrences", fmt.Sprint(input.MinOccurrences)})
	if input.Policy != "" {
		parts = append(parts, "--policy", shellquote.Argument(input.Policy))
	}
	if removeSourceCap {
		parts = append(parts, "--max-files", "0")
	} else if input.MaxFiles > 0 {
		parts = append(parts, "--max-files", fmt.Sprint(input.MaxFiles))
	}
	for _, path := range normalizedResultScope(input.Paths, ".") {
		parts = append(parts, shellquote.Argument(path))
	}
	return strings.Join(parts, " ")
}
