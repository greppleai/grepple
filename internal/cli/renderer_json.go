package cli

import (
	"github.com/greppleai/grepple/api"
	"github.com/greppleai/grepple/parser"
)

type jsonResultRenderer struct {
	output      *outputWriter
	matchesOnly bool
}

func (renderer jsonResultRenderer) Render(results []api.FileResult) error {
	if renderer.matchesOnly {
		return renderer.output.writeJSON(map[string]any{"matches": flatMatches(results)})
	}
	return renderer.output.writeJSON(api.SearchResponse{Results: results, SourceAnalysis: searchSourceAnalysis(results)})
}

func searchSourceAnalysis(results []api.FileResult) *api.SourceAnalysis {
	analysis := &api.SourceAnalysis{Returned: len(results)}
	classified := 0
	for _, result := range results {
		switch parser.SegmentBuildStatus(result.StructureStatus) {
		case parser.SegmentBuildStructured:
			analysis.Structured++
			classified++
		case parser.SegmentBuildRecovered:
			analysis.Recovered++
			classified++
		case parser.SegmentBuildPlain:
			analysis.Plain++
			classified++
		case parser.SegmentBuildUnsupported:
			analysis.Unsupported++
			classified++
		case parser.SegmentBuildFailed:
			analysis.Failed++
			classified++
		}
	}
	if classified == 0 {
		return nil
	}
	return analysis
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
