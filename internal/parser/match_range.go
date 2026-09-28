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
	depth      int
}

type structuralLineRangeCollector struct {
	lines       map[int]bool
	sortedLines []int
	candidates  map[int]structuralLineCandidate
	enclosing   bool
}

// StructuralLineRanges finds the syntax construct that begins on each requested
// line. It is language-neutral across parser-backed grammars. Lines without a
// multi-line construct, unsupported languages, and parse failures are omitted so
// callers can retain their ordinary single-line location.
func StructuralLineRanges(content, language string, lines map[int]bool) map[int]StructuralLineRange {
	return syntaxLineRanges(content, language, lines, false)
}

// EnclosingLineRanges finds the nearest meaningful multi-line named syntax
// construct containing each requested line. Transparent child-list wrappers are
// ignored. It is intended for opt-in scope annotation when the matching line is
// inside, rather than at the start of, a construct.
func EnclosingLineRanges(content, language string, lines map[int]bool) map[int]StructuralLineRange {
	return syntaxLineRanges(content, language, lines, true)
}

func syntaxLineRanges(content, language string, lines map[int]bool, enclosing bool) map[int]StructuralLineRange {
	if len(lines) == 0 {
		return nil
	}
	document, err := parseDocument(language, content)
	if err != nil {
		return nil
	}
	defer document.Close()
	collector := newStructuralLineRangeCollector(lines, enclosing)
	_ = document.Read(func(view DocumentView) error {
		walkNamedViewBounded(view.Root(), walkOptions{}, collector.visit)
		return nil
	})
	return collector.result()
}

func newStructuralLineRangeCollector(lines map[int]bool, enclosing bool) *structuralLineRangeCollector {
	sortedLines := make([]int, 0, len(lines))
	for line := range lines {
		sortedLines = append(sortedLines, line)
	}
	sort.Ints(sortedLines)
	return &structuralLineRangeCollector{
		lines:       lines,
		sortedLines: sortedLines,
		candidates:  make(map[int]structuralLineCandidate, len(lines)),
		enclosing:   enclosing,
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
	if collector.enclosing {
		collector.collectEnclosing(node, nodeRange, endLine, depth)
	} else {
		collector.collectStarting(nodeRange, endLine)
	}
	return true
}

func (collector *structuralLineRangeCollector) collectStarting(nodeRange Range, endLine int) {
	line := nodeRange.Start.Line
	if !collector.lines[line] || endLine <= line {
		return
	}
	current, exists := collector.candidates[line]
	if !exists || preferableStructuralRange(nodeRange.Start.Column, endLine, current) {
		collector.candidates[line] = structuralLineCandidate{
			rangeValue: StructuralLineRange{StartLine: line, EndLine: endLine},
			column:     nodeRange.Start.Column,
		}
	}
}

func (collector *structuralLineRangeCollector) collectEnclosing(node ViewNode, nodeRange Range, endLine, depth int) {
	if isTransparentLineContainer(node, nodeRange, endLine) {
		return
	}
	startLine := nodeRange.Start.Line
	if endLine <= startLine {
		return
	}
	index := sort.SearchInts(collector.sortedLines, startLine)
	for index < len(collector.sortedLines) && collector.sortedLines[index] <= endLine {
		line := collector.sortedLines[index]
		current, exists := collector.candidates[line]
		if !exists || preferableEnclosingRange(startLine, endLine, nodeRange.Start.Column, depth, current) {
			collector.candidates[line] = structuralLineCandidate{
				rangeValue: StructuralLineRange{StartLine: startLine, EndLine: endLine},
				column:     nodeRange.Start.Column,
				depth:      depth,
			}
		}
		index++
	}
}

func isTransparentLineContainer(node ViewNode, nodeRange Range, endLine int) bool {
	childCount := node.NamedChildCount()
	if childCount == 0 {
		return false
	}
	firstRange := node.NamedChild(0).Range()
	lastRange := node.NamedChild(childCount - 1).Range()
	sameStart := firstRange.Start.Line == nodeRange.Start.Line && firstRange.Start.Column == nodeRange.Start.Column
	return sameStart && inclusiveEndLine(lastRange) == endLine
}

func preferableEnclosingRange(startLine, endLine, column, depth int, current structuralLineCandidate) bool {
	if depth != current.depth {
		return depth > current.depth
	}
	span, currentSpan := endLine-startLine, current.rangeValue.EndLine-current.rangeValue.StartLine
	return span < currentSpan || (span == currentSpan && column > current.column)
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
