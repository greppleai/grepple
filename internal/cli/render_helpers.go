package cli

import (
	"github.com/greppleai/grepple/api"
	"github.com/greppleai/grepple/parser"
	"strings"
)

func quoteCommandArgument(value string) string {
	if value != "" && !strings.ContainsAny(value, " \t\r\n'\"") {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func searchSourceAnalysis(results []api.FileResult) *api.SourceAnalysis {
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
