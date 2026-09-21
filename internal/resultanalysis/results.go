// Package resultanalysis summarizes structural search result outcomes.
package resultanalysis

import (
	"github.com/greppleai/grepple/api"
	"github.com/greppleai/grepple/parser"
)

// Sources returns structural source-analysis totals when results contain classifications.
func Sources(results []api.FileResult) *api.SourceAnalysis {
	analysis := &api.SourceAnalysis{Returned: len(results)}
	classified := 0
	for _, result := range results {
		switch parser.SegmentBuildStatus(result.StructureStatus) {
		case parser.SegmentBuildStructured:
			analysis.Structured++
		case parser.SegmentBuildRecovered:
			analysis.Recovered++
		case parser.SegmentBuildPlain:
			analysis.Plain++
		case parser.SegmentBuildUnsupported:
			analysis.Unsupported++
		case parser.SegmentBuildFailed:
			analysis.Failed++
		default:
			continue
		}
		classified++
	}
	if classified == 0 {
		return nil
	}
	return analysis
}
