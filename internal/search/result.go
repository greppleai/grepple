package search

import (
	"github.com/greppleai/grepple/internal/navigation"
	"sort"
	"strings"

	"github.com/greppleai/grepple/internal/linerange"
	"github.com/greppleai/grepple/internal/parser"
)

// ToResult converts one internal match into the shared wire FileResult: sorted
// match lines, structural segments, and optional context lines.
func ToResult(m FileMatch, segs []parser.Segment, beforeContext, afterContext int) FileResult {
	lines := SplitLines(m.Content)
	var ns []int
	for n := range m.MatchLines {
		ns = append(ns, n)
	}
	sort.Ints(ns)
	matches := make([]ResultMatch, 0, len(ns))
	for _, n := range ns {
		match := ResultMatch{Line: n, Text: lines[n-1]}
		if structuralRange, ok := m.MatchRanges[n]; ok {
			if structuralRange.StartLine != n {
				match.StartLine = structuralRange.StartLine
			}
			match.EndLine = structuralRange.EndLine
		}
		matches = append(matches, match)
	}
	rs := resultSegments(m.Content, segs)
	related := relatedSymbols(m.Related)
	r := FileResult{
		Path: m.DisplayPath, Language: m.Language, StructureStatus: string(m.StructureStatus), Matches: matches, Segments: rs, Related: related, LineRange: apiLineRange(m.LineRange),
		OmittedRelatedCallers: m.OmittedRelatedCallers, OmittedRelatedCallees: m.OmittedRelatedCallees, OmittedRelatedTypes: m.OmittedRelatedTypes,
	}
	if beforeContext > 0 || afterContext > 0 {
		r.Context = ContextLines(m.Content, m.MatchLines, beforeContext, afterContext)
	}
	return r
}

func apiLineRange(result *linerange.Result) *LineRangeResult {
	if result == nil {
		return nil
	}
	return &LineRangeResult{
		RequestedStart: result.RequestedStart, RequestedEnd: result.RequestedEnd,
		ReturnedStart: result.ReturnedStart, ReturnedEnd: result.ReturnedEnd,
		FileLines: result.FileLines, Outcome: string(result.Outcome), Warning: result.Warning,
	}
}

const maxInlineWhitespaceGap = 2

func resultSegments(content string, segments []parser.Segment) []ResultSegment {
	lines := SplitLines(content)
	result := make([]ResultSegment, 0, len(segments))
	previousEnd := 0
	for _, segment := range segments {
		if len(result) > 0 {
			if gap, ok := inlineWhitespaceGap(lines, previousEnd+1, segment.Start-1); ok {
				result = append(result, gap)
			}
		}
		text := segment.Text
		if segment.Kind == "lines" {
			text = strings.Join(lines[segment.Start-1:segment.End], "\n")
		}
		result = append(result, ResultSegment{Kind: segment.Kind, Start: segment.Start, End: segment.End, Text: text})
		previousEnd = max(previousEnd, segment.End)
	}
	return result
}

func inlineWhitespaceGap(lines []string, start, end int) (ResultSegment, bool) {
	count := end - start + 1
	if count <= 0 || count > maxInlineWhitespaceGap || start < 1 || end > len(lines) {
		return ResultSegment{}, false
	}
	gapLines := lines[start-1 : end]
	for _, line := range gapLines {
		if strings.TrimSpace(line) != "" {
			return ResultSegment{}, false
		}
	}
	return ResultSegment{Kind: "spacing", Start: start, End: end, Text: strings.Join(gapLines, "\n")}, true
}

func relatedSymbols(points []RelatedPoint) []navigation.RelatedSymbol {
	related := make([]navigation.RelatedSymbol, 0, len(points))
	for _, point := range points {
		symbol := navigation.RelatedSymbol{
			Name: point.Name, Path: point.Path, Kind: point.Kind, Direction: point.Direction,
			Start: point.Start, End: point.End, CallLine: point.CallLine, Confidence: point.Confidence, Role: point.Role, External: point.External,
		}
		if point.Preview != nil {
			symbol.Segments = resultSegments(point.Preview.Content, []parser.Segment{{Kind: "lines", Start: point.Preview.Start, End: point.Preview.End}})
			symbol.Related = relatedSymbols(point.Preview.Related)
			symbol.OmittedCallers = point.Preview.OmittedCallers
			symbol.OmittedCallees = point.Preview.OmittedCallees
			symbol.OmittedTypes = point.Preview.OmittedTypes
		}
		related = append(related, symbol)
	}
	return related
}

// BuildResults converts matches concurrently while preserving their ranked order.
func BuildResults(matches []FileMatch, beforeContext, afterContext int, includeSegments bool) []FileResult {
	results := make([]FileResult, len(matches))
	runParallel(len(matches), func(index int) {
		var segments []parser.Segment
		if includeSegments {
			segments = matches[index].Segments
			if !matches[index].SegmentsReady {
				segments, matches[index].StructureStatus = parser.BuildSegmentsWithStatus(matches[index].Content, matches[index].Language, matches[index].MatchLines)
			}
		}
		results[index] = ToResult(matches[index], segments, beforeContext, afterContext)
	})
	return results
}
