package parser

import "sort"

// StructuralLineRange identifies a multi-line syntax construct that begins on
// a matching source line. Lines are one-based and inclusive.
type StructuralLineRange struct {
	StartLine int
	EndLine   int
}

type structuralLineCandidate struct {
	rangeValue StructuralLineRange
	column     int
}

type structuralLineRangeCollector struct {
	lines       map[int]bool
	sortedLines []int
	candidates  map[int]structuralLineCandidate
}

// StructuralLineRanges finds the outer syntax construct that begins on each
// requested line. It is language-neutral across parser-backed grammars. Lines
// without a multi-line construct, unsupported languages, and parse failures are
// omitted so callers can retain their ordinary single-line location.
func StructuralLineRanges(content, language string, lines map[int]bool) map[int]StructuralLineRange {
	if len(lines) == 0 {
		return nil
	}
	document, err := ParseDocument(language, content)
	if err != nil {
		return nil
	}
	defer document.Close()
	collector := newStructuralLineRangeCollector(lines)
	_ = document.Read(func(view DocumentView) error {
		WalkNamedViewBounded(view.Root(), WalkOptions{}, collector.visit)
		return nil
	})
	return collector.result()
}

func newStructuralLineRangeCollector(lines map[int]bool) *structuralLineRangeCollector {
	sortedLines := make([]int, 0, len(lines))
	for line := range lines {
		sortedLines = append(sortedLines, line)
	}
	sort.Ints(sortedLines)
	return &structuralLineRangeCollector{
		lines:       lines,
		sortedLines: sortedLines,
		candidates:  make(map[int]structuralLineCandidate, len(lines)),
	}
}

func (collector *structuralLineRangeCollector) visit(node ViewNode, depth int) bool {
	nodeRange := node.Range()
	endLine := inclusiveEndLine(nodeRange)
	if !rangeContainsLine(collector.sortedLines, nodeRange.Start.Line, endLine) {
		return false
	}
	if depth == 1 {
		return true
	}
	line := nodeRange.Start.Line
	if !collector.lines[line] || endLine <= line {
		return true
	}
	current, exists := collector.candidates[line]
	if !exists || preferableStructuralRange(nodeRange.Start.Column, endLine, current) {
		collector.candidates[line] = structuralLineCandidate{
			rangeValue: StructuralLineRange{StartLine: line, EndLine: endLine},
			column:     nodeRange.Start.Column,
		}
	}
	return true
}

func preferableStructuralRange(column, endLine int, current structuralLineCandidate) bool {
	return column < current.column || (column == current.column && endLine < current.rangeValue.EndLine)
}

func (collector *structuralLineRangeCollector) result() map[int]StructuralLineRange {
	result := make(map[int]StructuralLineRange, len(collector.candidates))
	for line, candidate := range collector.candidates {
		result[line] = candidate.rangeValue
	}
	return result
}

func rangeContainsLine(sortedLines []int, start, end int) bool {
	index := sort.SearchInts(sortedLines, start)
	return index < len(sortedLines) && sortedLines[index] <= end
}

func inclusiveEndLine(nodeRange Range) int {
	endLine := nodeRange.End.Line
	if nodeRange.End.Column == 1 && endLine > nodeRange.Start.Line {
		endLine--
	}
	return endLine
}
