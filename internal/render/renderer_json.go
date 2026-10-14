package render

import (
	"github.com/greppleai/grepple/internal/resultanalysis"
	"github.com/greppleai/grepple/internal/search"
	"github.com/greppleai/grepple/internal/wire"
)

type jsonResultRenderer struct {
	output      *outputWriter
	matchesOnly bool
	metadata    *wire.ResultMetadata
}

func (renderer jsonResultRenderer) Render(results []search.FileResult) error {
	if renderer.matchesOnly {
		response := map[string]any{"matches": flatMatches(results), "metadata": renderer.metadata}
		if ranges := resultLineRanges(results); len(ranges) > 0 {
			response["lineRanges"] = ranges
		}
		return renderer.output.writeJSON(response)
	}
	return renderer.output.writeJSON(wire.SearchResponse{Results: results, Metadata: renderer.metadata, SourceAnalysis: searchSourceAnalysis(results)})
}
func searchSourceAnalysis(results []search.FileResult) *wire.SourceAnalysis {
	return resultanalysis.Sources(results)
}

func flatMatches(results []search.FileResult) []map[string]any {
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

func resultLineRanges(results []search.FileResult) []map[string]any {
	ranges := []map[string]any{}
	for _, result := range results {
		if result.LineRange != nil {
			ranges = append(ranges, map[string]any{"path": result.Path, "range": result.LineRange})
		}
	}
	return ranges
}
