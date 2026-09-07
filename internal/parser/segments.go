package parser

import (
	"sort"
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

// BuildSegments constructs structural segments for content using its detected
// language and 1-based hit lines. Unsupported languages and parse failures fall
// back to one-line plain-text segments.
func BuildSegments(content, language string, hitLines map[int]bool, maxSegments int) []Segment {
	if language == "markdown" {
		return buildMarkdownSegments(content, hitLines, maxSegments)
	}
	if language == "text" {
		return buildPlainTextSegments(hitLines, maxSegments)
	}
	config := configForLanguage(language)
	segments, ok := analyzeStructure(config, content, hitLines, maxSegments)
	if !ok {
		return buildPlainTextSegments(hitLines, maxSegments)
	}
	return segments
}

func buildPlainTextSegments(hits map[int]bool, maxSegments int) []Segment {
	lines := make([]int, 0, len(hits))
	for line := range hits {
		lines = append(lines, line)
	}
	sort.Ints(lines)
	if len(lines) > maxSegments {
		lines = lines[:maxSegments]
	}
	segments := make([]Segment, 0, len(lines))
	for _, line := range lines {
		segments = append(segments, Segment{Kind: "lines", Start: line, End: line})
	}
	return segments
}

func buildASTSegments(root *sitter.Node, content string, hits matchLines, config *languageConfig, maxSegments int) []Segment {
	// Split the file once and thread it through segment building. summarizeNode
	// used to re-split the whole file on every call, which is O(summaries × lines).
	lines := splitLines(content)
	topLevel := nonPunctuationChildren(root)
	matchedIndexes := matchedTopLevelIndexes(topLevel, hits)

	var segments []Segment
	for index, node := range topLevel {
		start, end := nodeStart(node), nodeEnd(node)
		if hits.hitsRange(start, end) {
			segments = append(segments, buildMatchingStructureSegments(node, content, lines, hits, config)...)
			continue
		}
		if config.structuralTypes.contains(node.Kind()) && nearMatchedTopLevel(index, matchedIndexes) {
			segments = append(segments, Segment{Kind: "summary", Start: start, End: end, Text: summarizeNode(node, content, lines)})
		}
	}
	segments = uncoveredLineSegments(segments, hits)
	return limitSegments(mergeSegments(segments, len(lines)), maxSegments)
}

// nonPunctuationChildren returns root's named children with punctuation nodes
// (braces, parens, commas) dropped.
func nonPunctuationChildren(root *sitter.Node) []*sitter.Node {
	var topLevel []*sitter.Node
	for _, node := range namedChildren(root) {
		if !isPunctuation(node) {
			topLevel = append(topLevel, node)
		}
	}
	return topLevel
}

// matchedTopLevelIndexes returns the indexes of the top-level nodes whose
// line range contains a hit.
func matchedTopLevelIndexes(topLevel []*sitter.Node, hits matchLines) []int {
	var matched []int
	for index, node := range topLevel {
		if hits.hitsRange(nodeStart(node), nodeEnd(node)) {
			matched = append(matched, index)
		}
	}
	return matched
}

// uncoveredLineSegments appends a lines segment for every hit not already
// covered by an existing lines segment.
func uncoveredLineSegments(segments []Segment, hits matchLines) []Segment {
	for _, line := range hits.sorted {
		covered := false
		for _, segment := range segments {
			if segment.Kind == "lines" && line >= segment.Start && line <= segment.End {
				covered = true
				break
			}
		}
		if !covered {
			segments = append(segments, Segment{Kind: "lines", Start: line, End: line})
		}
	}
	return segments
}

func nearMatchedTopLevel(index int, matchedIndexes []int) bool {
	for _, matched := range matchedIndexes {
		if abs(index-matched) <= 2 {
			return true
		}
	}
	return false
}

func buildMatchingStructureSegments(node *sitter.Node, content string, lines []string, hits matchLines, config *languageConfig) []Segment {
	node = unwrapExport(node, config)
	start, end := nodeStart(node), nodeEnd(node)
	if shouldCompactJSXFunction(node, hits, config) {
		return buildCompactFunctionSegments(node, content, lines, hits, config)
	}
	if !config.containerTypes.contains(node.Kind()) {
		return []Segment{{Kind: "lines", Start: start, End: end}}
	}

	children := structuralChildren(node, config)
	if !anyChildHasMatch(children, hits) {
		return []Segment{{Kind: "lines", Start: start, End: end}}
	}
	segments := []Segment{{Kind: "lines", Start: start, End: start}}
	segments = append(segments, containerChildSegments(children, content, lines, hits)...)
	if end > start {
		segments = append(segments, Segment{Kind: "lines", Start: end, End: end})
	}
	return segments
}

// unwrapExport descends through export wrappers to the declaration they
// export (the container or function inside), returning the node itself when
// it is not an export wrapper.
func unwrapExport(node *sitter.Node, config *languageConfig) *sitter.Node {
	for config.exportTypes.contains(node.Kind()) {
		next := node
		for _, child := range namedChildren(node) {
			if config.containerTypes.contains(child.Kind()) || config.functionLikeTypes.contains(child.Kind()) {
				next = child
				break
			}
		}
		if next == node {
			return node
		}
		node = next
	}
	return node
}

// anyChildHasMatch reports whether any structural child's line range contains
// a hit.
func anyChildHasMatch(children []*sitter.Node, hits matchLines) bool {
	for _, child := range children {
		if hits.hitsRange(nodeStart(child), nodeEnd(child)) {
			return true
		}
	}
	return false
}

// containerChildSegments renders each structural child: full lines when it
// contains a match, a one-line summary otherwise.
func containerChildSegments(children []*sitter.Node, content string, lines []string, hits matchLines) []Segment {
	var segments []Segment
	for _, child := range children {
		childStart, childEnd := nodeStart(child), nodeEnd(child)
		if hits.hitsRange(childStart, childEnd) {
			segments = append(segments, Segment{Kind: "lines", Start: childStart, End: childEnd})
		} else {
			segments = append(segments, Segment{Kind: "summary", Start: childStart, End: childEnd, Text: summarizeNode(child, content, lines)})
		}
	}
	return segments
}

func shouldCompactJSXFunction(node *sitter.Node, hits matchLines, config *languageConfig) bool {
	return config.functionLikeTypes.contains(node.Kind()) &&
		containsNodeType(node, config.jsxElementTypes) &&
		nodeHasMatchInTypes(node, hits, config.jsxElementTypes) &&
		nodeEnd(node)-nodeStart(node) >= 6
}

func buildCompactFunctionSegments(node *sitter.Node, content string, lines []string, hits matchLines, config *languageConfig) []Segment {
	start, end := nodeStart(node), nodeEnd(node)
	body := node.ChildByFieldName("body")
	if body == nil {
		for _, child := range namedChildren(node) {
			if config.blockTypes.contains(child.Kind()) {
				body = child
				break
			}
		}
	}
	if body == nil {
		return []Segment{{Kind: "lines", Start: start, End: end}}
	}
	segments := []Segment{{Kind: "lines", Start: start, End: start}}
	for _, child := range namedChildren(body) {
		childStart, childEnd := nodeStart(child), nodeEnd(child)
		if hits.hitsRange(childStart, childEnd) {
			if containsNodeType(child, config.jsxElementTypes) {
				segments = append(segments, focusedLineSegments(childStart, childEnd, hits)...)
			} else {
				segments = append(segments, Segment{Kind: "lines", Start: childStart, End: childEnd})
			}
		} else {
			segments = append(segments, Segment{Kind: "summary", Start: childStart, End: childEnd, Text: summarizeNode(child, content, lines)})
		}
	}
	if end > start {
		segments = append(segments, Segment{Kind: "lines", Start: end, End: end})
	}
	return segments
}

func focusedLineSegments(start, end int, hits matchLines) []Segment {
	var lines []int
	for _, line := range hits.sorted {
		if line >= start && line <= end {
			lines = append(lines, line)
		}
	}
	sort.Ints(lines)
	segments := make([]Segment, 0, len(lines))
	for _, line := range lines {
		segments = append(segments, Segment{Kind: "lines", Start: max(start, line-1), End: min(end, line+1)})
	}
	return segments
}

func containsNodeType(node *sitter.Node, types stringSet) bool {
	if len(types) == 0 {
		return false
	}
	found := false
	walkNodes(node, func(candidate *sitter.Node) {
		if !found && types.contains(candidate.Kind()) {
			found = true
		}
	})
	return found
}

func nodeHasMatchInTypes(node *sitter.Node, hits matchLines, types stringSet) bool {
	found := false
	walkNodes(node, func(candidate *sitter.Node) {
		if !found && types.contains(candidate.Kind()) && hits.hitsRange(nodeStart(candidate), nodeEnd(candidate)) {
			found = true
		}
	})
	return found
}

func structuralChildren(node *sitter.Node, config *languageConfig) []*sitter.Node {
	if config.classDeclarationTypes.contains(node.Kind()) {
		body := node.ChildByFieldName("body")
		if body == nil {
			for _, child := range namedChildren(node) {
				if config.classBodyTypes.contains(child.Kind()) {
					body = child
					break
				}
			}
		}
		if body == nil {
			return nil
		}
		return childrenInTypes(body, config.contextTypes)
	}
	return childrenInTypes(node, config.contextTypes)
}

func childrenInTypes(node *sitter.Node, types stringSet) []*sitter.Node {
	var children []*sitter.Node
	for _, child := range namedChildren(node) {
		if types.contains(child.Kind()) {
			children = append(children, child)
		}
	}
	return children
}

func summarizeNode(node *sitter.Node, content string, lines []string) string {
	start, end := nodeStart(node), nodeEnd(node)
	firstLine := ""
	if start >= 1 && start <= len(lines) {
		firstLine = lines[start-1]
	}
	if strings.TrimSpace(firstLine) == "" {
		for _, line := range strings.Split(nodeText(node, content), "\n") {
			if strings.TrimSpace(line) != "" {
				firstLine = line
				break
			}
		}
	}
	if start == end {
		return strings.TrimRight(firstLine, " \t")
	}
	if index := strings.Index(firstLine, "{"); index >= 0 {
		return strings.TrimRight(firstLine[:index], " \t") + " { … }"
	}
	return strings.TrimRight(firstLine, " \t") + " …"
}

func mergeSegments(segments []Segment, lineCount int) []Segment {
	for index := range segments {
		if segments[index].Kind == "lines" {
			segments[index].Start = max(1, min(lineCount, segments[index].Start))
			segments[index].End = max(1, min(lineCount, segments[index].End))
		}
	}
	sort.SliceStable(segments, func(i, j int) bool { return segments[i].Start < segments[j].Start })
	var merged []Segment
	for _, segment := range segments {
		if len(merged) > 0 {
			previous := &merged[len(merged)-1]
			if previous.Kind == "lines" && segment.Kind == "lines" && segment.Start <= previous.End+1 {
				previous.End = max(previous.End, segment.End)
				continue
			}
			if segment.Start == previous.Start && previous.Kind == "lines" {
				continue
			}
		}
		merged = append(merged, segment)
	}
	return merged
}

func limitSegments(segments []Segment, maxSegments int) []Segment {
	if len(segments) <= maxSegments {
		return segments
	}
	var matching, context []Segment
	for _, segment := range segments {
		if segment.Kind == "lines" {
			matching = append(matching, segment)
		} else {
			context = append(context, segment)
		}
	}
	kept := append([]Segment{}, matching[:min(len(matching), maxSegments)]...)
	remaining := maxSegments - len(kept)
	if remaining > 0 {
		kept = append(kept, context[:min(len(context), remaining)]...)
	}
	if len(kept) == 0 {
		kept = append(kept, segments[:min(len(segments), maxSegments)]...)
	}
	sort.SliceStable(kept, func(i, j int) bool { return kept[i].Start < kept[j].Start })
	return kept
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
