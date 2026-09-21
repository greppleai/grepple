package boundaries

import (
	"fmt"
	"sort"
	"strings"

	"github.com/greppleai/grepple/api"
	graphcommand "github.com/greppleai/grepple/internal/cli/graph"
	"github.com/greppleai/grepple/internal/shellquote"
)

func resultMetadata(input MetadataInput, activeScopeFlags func([]string) []string) *api.ResultMetadata {
	report := input.Report
	total := len(report.Candidates) + len(report.TypeBoundaries) + len(report.FacadeBypasses)
	returned := total
	if !input.JSON {
		returned = visibleCount(len(report.Candidates), input.Limit) + visibleCount(len(report.TypeBoundaries), input.Limit) + visibleCount(len(report.FacadeBypasses), input.Limit)
	}
	omittedFindings, omittedSources := total-returned, 0
	if report.Truncation != nil {
		omittedSources = report.Truncation.Skipped
	}
	sources := graphcommand.SourceSummary{Discovered: report.Sources.Discovered, Selected: report.Sources.Selected, Parsed: report.Sources.Parsed, Skipped: report.Sources.Skipped, Failed: report.Sources.Failed, Recovered: report.Sources.Recovered}
	metadata := &api.ResultMetadata{Scope: api.ResultScope{Mode: "local", Paths: normalized(input.Paths, "."), ExcludedPaths: []string{}, Repositories: []string{}, ExcludedRepositories: []string{}, Languages: []string{}}, Order: "risk-breadth", Page: api.ResultPage{Limit: input.Limit, Returned: returned, Total: &total, Complete: omittedFindings == 0 && omittedSources == 0 && sources.Failed == 0 && sources.Recovered == 0}, Limits: api.ResultLimits{MaxFiles: input.MaxFiles, MaxOutputBytes: input.MaxOutputBytes, JSONByteUncapped: input.JSON}, Omitted: api.ResultOmissions{Sources: omittedSources, Findings: omittedFindings}, Diagnostics: sourceDiagnostics(sources)}
	if omittedFindings > 0 || omittedSources > 0 {
		metadata.NextCommand = continuationCommand(input, omittedSources > 0, activeScopeFlags)
	}
	return metadata
}

func visibleCount(total, limit int) int {
	if limit == 0 || total < limit {
		return total
	}
	return limit
}
func normalized(values []string, fallback string) []string {
	if len(values) == 0 {
		return []string{fallback}
	}
	result := append([]string(nil), values...)
	sort.Strings(result)
	return result
}
func sourceDiagnostics(s graphcommand.SourceSummary) []api.ResultDiagnostic {
	var result []api.ResultDiagnostic
	if s.Failed > 0 {
		result = append(result, api.ResultDiagnostic{Code: "source-failed", Message: fmt.Sprintf("%d selected source files failed analysis", s.Failed)})
	}
	if s.Recovered > 0 {
		result = append(result, api.ResultDiagnostic{Code: "source-recovered", Message: fmt.Sprintf("%d source files required parser recovery", s.Recovered)})
	}
	if s.Skipped > 0 {
		result = append(result, api.ResultDiagnostic{Code: "source-skipped", Message: fmt.Sprintf("%d discovered source files were unsupported or binary", s.Skipped)})
	}
	return result
}
func continuationCommand(input MetadataInput, removeSourceCap bool, activeScopeFlags func([]string) []string) string {
	parts := []string{"grepple", "boundaries", "--json", "--min-occurrences", fmt.Sprint(input.MinOccurrences)}
	if activeScopeFlags != nil {
		parts = activeScopeFlags(parts)
	}
	if input.Policy != "" {
		parts = append(parts, "--policy", shellquote.Argument(input.Policy))
	}
	if removeSourceCap {
		parts = append(parts, "--max-files", "0")
	} else if input.MaxFiles > 0 {
		parts = append(parts, "--max-files", fmt.Sprint(input.MaxFiles))
	}
	for _, path := range normalized(input.Paths, ".") {
		parts = append(parts, shellquote.Argument(path))
	}
	return strings.Join(parts, " ")
}

// GraphFromNavigation projects shared graph output for boundary analysis.
func GraphFromNavigation(graph graphcommand.Output) GraphOutput {
	var truncation *Truncation
	if graph.Truncation != nil {
		truncation = &Truncation{Reason: graph.Truncation.Reason, Limit: graph.Truncation.Limit, Skipped: graph.Truncation.Skipped}
	}
	return GraphOutput{Files: graph.Files, Sources: SourceSummary{Discovered: graph.Sources.Discovered, Selected: graph.Sources.Selected, Parsed: graph.Sources.Parsed, Skipped: graph.Sources.Skipped, Failed: graph.Sources.Failed, Recovered: graph.Sources.Recovered}, Declarations: graph.Declarations, Calls: graph.Calls, Fields: graph.Fields, TypeUsages: graph.TypeUsages, MemberAccesses: graph.MemberAccesses, Truncation: truncation}
}
