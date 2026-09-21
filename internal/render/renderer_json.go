package render

import (
	"github.com/greppleai/grepple/api"
	"github.com/greppleai/grepple/internal/resultanalysis"
)

type jsonResultRenderer struct {
	output       *outputWriter
	matchesOnly  bool
	metadata     *api.ResultMetadata
	contextGuard ContextGuard
}

func (renderer jsonResultRenderer) Render(results []api.FileResult) error {
	if renderer.contextGuard == nil {
		renderer.contextGuard = noopContextGuard{}
	}
	if renderer.matchesOnly {
		response := map[string]any{"matches": flatMatches(results), "metadata": renderer.metadata}
		if ranges := resultLineRanges(results); len(ranges) > 0 {
			response["lineRanges"] = ranges
		}
		return renderer.output.writeJSON(response)
	}
	if err := renderer.output.writeJSON(api.SearchResponse{Results: results, Metadata: renderer.metadata, SourceAnalysis: searchSourceAnalysis(results)}); err != nil {
		return err
	}
	recordJSONResultCoverage(renderer.contextGuard, results)
	return nil
}

func recordJSONResultCoverage(guard ContextGuard, results []api.FileResult) {
	if guard == nil {
		return
	}
	for _, result := range results {
		source := resultSourceIdentity(result)
		for _, segment := range result.Segments {
			guard.Record(source, nil, segment)
		}
		for _, match := range result.Matches {
			guard.RecordSearchLine(source, match.Line, match.Text)
		}
		for _, line := range result.Context {
			guard.RecordSearchLine(source, line.Line, line.Text)
		}
		recordJSONRelatedCoverage(guard, result.Repo, result.Related)
	}
}

func recordJSONRelatedCoverage(guard ContextGuard, repository string, points []api.RelatedSymbol) {
	for _, point := range points {
		source := point.Path
		if point.Artifact == nil && repository != "" {
			source = repository + "\x00" + point.Path
		}
		for _, segment := range point.Segments {
			guard.Record(source, point.Artifact, segment)
		}
		recordJSONRelatedCoverage(guard, repository, point.Related)
	}
}

func searchSourceAnalysis(results []api.FileResult) *api.SourceAnalysis {
	return resultanalysis.Sources(results)
}

func flatMatches(results []api.FileResult) []map[string]any {
	matches := []map[string]any{}
	for _, result := range results {
		for _, match := range result.Matches {
			item := map[string]any{"path": result.Path, "line": match.Line, "text": match.Text}
			if match.StartLine > 0 {
				item["startLine"] = match.StartLine
			}
			if match.EndLine > 0 {
				item["endLine"] = match.EndLine
			}
			if result.Repo != "" {
				item["repo"] = result.Repo
			}
			matches = append(matches, item)
		}
	}
	return matches
}

func resultLineRanges(results []api.FileResult) []map[string]any {
	ranges := []map[string]any{}
	for _, result := range results {
		if result.LineRange != nil {
			ranges = append(ranges, map[string]any{"path": result.Path, "range": result.LineRange})
		}
	}
	return ranges
}
