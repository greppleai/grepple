package search

import (
	"github.com/greppleai/grepple/api"
	"sort"
	"strings"

	"github.com/greppleai/grepple/parser"
)

// ToResult converts one internal match into the shared wire api.FileResult: sorted
// match lines, structural segments, and optional context lines capped at maxWindows.
func ToResult(m FileMatch, segs []parser.Segment, beforeContext, afterContext, maxWindows int) api.FileResult {
	lines := SplitLines(m.Content)
	var ns []int
	for n := range m.MatchLines {
		ns = append(ns, n)
	}
	sort.Ints(ns)
	matches := make([]api.ResultMatch, 0, len(ns))
	for _, n := range ns {
		matches = append(matches, api.ResultMatch{Line: n, Text: lines[n-1]})
	}
	rs := resultSegments(m.Content, segs)
	related := relatedSymbols(m.Related)
	r := api.FileResult{Path: m.DisplayPath, Language: m.Language, Matches: matches, Segments: rs, Related: related}
	if beforeContext > 0 || afterContext > 0 {
		r.Context = ContextLines(m.Content, m.MatchLines, beforeContext, afterContext, maxWindows)
	}
	return r
}
func resultSegments(content string, segments []parser.Segment) []api.ResultSegment {
	lines := SplitLines(content)
	result := make([]api.ResultSegment, 0, len(segments))
	for _, segment := range segments {
		text := segment.Text
		if segment.Kind == "lines" {
			text = strings.Join(lines[segment.Start-1:segment.End], "\n")
		}
		result = append(result, api.ResultSegment{Kind: segment.Kind, Start: segment.Start, End: segment.End, Text: text})
	}
	return result
}

func relatedSymbols(points []RelatedPoint) []api.RelatedSymbol {
	related := make([]api.RelatedSymbol, 0, len(points))
	for _, point := range points {
		symbol := api.RelatedSymbol{
			Name: point.Name, Path: point.Path, Kind: point.Kind, Direction: point.Direction,
			Start: point.Start, End: point.End, CallLine: point.CallLine, Confidence: point.Confidence,
		}
		if point.Preview != nil {
			symbol.Segments = resultSegments(point.Preview.Content, []parser.Segment{{Kind: "lines", Start: point.Preview.Start, End: point.Preview.End}})
			symbol.Related = relatedSymbols(point.Preview.Related)
		}
		related = append(related, symbol)
	}
	return related
}

// BuildResults converts matches concurrently while preserving their ranked order.
func BuildResults(matches []FileMatch, beforeContext, afterContext, maxSegments int, includeSegments bool) []api.FileResult {
	results := make([]api.FileResult, len(matches))
	runParallel(len(matches), func(index int) {
		var segments []parser.Segment
		if includeSegments {
			segments = matches[index].Segments
			if !matches[index].SegmentsReady {
				segments = parser.BuildSegments(matches[index].Content, matches[index].Language, matches[index].MatchLines, maxSegments)
			}
		}
		results[index] = ToResult(matches[index], segments, beforeContext, afterContext, maxSegments)
	})
	return results
}
