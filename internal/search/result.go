package search

import (
	"grepple/internal/api"
	"sort"
	"strings"

	"grepple/internal/parser"
)

// ToResult converts one internal match into the shared wire api.FileResult: sorted
// match lines, structural segments, and optional context lines capped at maxWindows.
func ToResult(m FileMatch, segs []parser.Segment, context, maxWindows int) api.FileResult {
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
	rs := make([]api.ResultSegment, 0, len(segs))
	for _, s := range segs {
		text := s.Text
		if s.Kind == "lines" {
			text = strings.Join(lines[s.Start-1:s.End], "\n")
		}
		rs = append(rs, api.ResultSegment{Kind: s.Kind, Start: s.Start, End: s.End, Text: text})
	}
	r := api.FileResult{Path: m.DisplayPath, Language: m.Language, Matches: matches, Segments: rs}
	if context > 0 {
		r.Context = ContextLines(m.Content, m.MatchLines, context, maxWindows)
	}
	return r
}

// BuildResults converts matches concurrently while preserving their ranked order.
func BuildResults(matches []FileMatch, context, maxSegments int, includeSegments bool) []api.FileResult {
	results := make([]api.FileResult, len(matches))
	runParallel(len(matches), func(index int) {
		var segments []parser.Segment
		if includeSegments {
			segments = matches[index].Segments
			if !matches[index].SegmentsReady {
				segments = parser.BuildSegments(matches[index].Content, matches[index].Language, matches[index].MatchLines, maxSegments)
			}
		}
		results[index] = ToResult(matches[index], segments, context, maxSegments)
	})
	return results
}
