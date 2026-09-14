package parser

import (
	"sort"
	"strings"
	"unicode/utf8"
)

// SegmentBuildStatus reports whether structural context was parsed or safely degraded.
type SegmentBuildStatus string

// Segment build status values distinguish intentional plain text from incomplete syntax.
const (
	SegmentBuildStructured  SegmentBuildStatus = "structured"
	SegmentBuildRecovered   SegmentBuildStatus = "recovered"
	SegmentBuildPlain       SegmentBuildStatus = "plain"
	SegmentBuildUnsupported SegmentBuildStatus = "unsupported"
	SegmentBuildFailed      SegmentBuildStatus = "failed"
)

// BuildSegments constructs structural segments for content using its detected
// language and 1-based hit lines. Unsupported languages and parse failures fall
// back to one-line plain-text segments.
func BuildSegments(content, language string, hitLines map[int]bool, maxSegments int) []Segment {
	segments, _ := BuildSegmentsWithStatus(content, language, hitLines, maxSegments)
	return segments
}

// BuildSegmentsWithStatus constructs segments and reports parser completeness.
func BuildSegmentsWithStatus(content, language string, hitLines map[int]bool, maxSegments int) ([]Segment, SegmentBuildStatus) {
	if !utf8.ValidString(content) {
		return buildPlainTextSegments(hitLines, maxSegments), SegmentBuildFailed
	}
	if language == "markdown" {
		return buildMarkdownSegments(content, hitLines, maxSegments), SegmentBuildStructured
	}
	if language == "text" {
		return buildPlainTextSegments(hitLines, maxSegments), SegmentBuildPlain
	}
	adapter := adapterForLanguage(language)
	if adapter == nil {
		return buildPlainTextSegments(hitLines, maxSegments), SegmentBuildUnsupported
	}
	segments, ok, recovered := analyzeStructure(adapter, content, hitLines, maxSegments)
	if !ok {
		return buildPlainTextSegments(hitLines, maxSegments), SegmentBuildFailed
	}
	if recovered {
		return segments, SegmentBuildRecovered
	}
	return segments, SegmentBuildStructured
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

func buildASTSegments(root *syntaxNode, content string, hits matchLines, config *structureRules, maxSegments int) []Segment {
	// Split the file once and thread it through segment building. summarizeNode
	// used to re-split the whole file on every call, which is O(summaries × lines).
	lines := splitLines(content)
	topLevel := nonPunctuationChildren(root)
	matchedIndexes := matchedTopLevelIndexes(topLevel, hits)

	var segments []Segment
	for index, node := range topLevel {
		start, end := node.StartLine(), node.EndLine()
		matchStart := leadingCommentStart(node)
		if hits.hitsRange(matchStart, end) {
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
func nonPunctuationChildren(root *syntaxNode) []*syntaxNode {
	var topLevel []*syntaxNode
	for _, node := range root.NamedChildren() {
		if !isPunctuation(node) {
			topLevel = append(topLevel, node)
		}
	}
	return topLevel
}

// matchedTopLevelIndexes returns the indexes of the top-level nodes whose
// line range contains a hit.
func matchedTopLevelIndexes(topLevel []*syntaxNode, hits matchLines) []int {
	var matched []int
	for index, node := range topLevel {
		if hits.hitsRange(leadingCommentStart(node), node.EndLine()) {
			matched = append(matched, index)
		}
	}
	return matched
}

// leadingCommentStart extends a declaration to an attached leading comment block.
// Comments and declaration metadata may be adjacent to the declaration or separated by one blank line.
func leadingCommentStart(node *syntaxNode) int {
	start := node.StartLine()
	for previous := node.PrevNamedSibling(); previous != nil && isLeadingAttachmentNode(previous); previous = previous.PrevNamedSibling() {
		if start-previous.EndLine() > 2 || isTrailingComment(previous) {
			break
		}
		start = previous.StartLine()
	}
	return start
}

func isCommentNode(node *syntaxNode) bool {
	switch node.Kind() {
	case "comment", "line_comment", "block_comment", "multiline_comment", "documentation_comment", "doc_comment":
		return true
	}
	return false
}

func isLeadingAttachmentNode(node *syntaxNode) bool {
	if isCommentNode(node) {
		return true
	}
	switch node.Kind() {
	case "attribute_item", "attribute_specifier", "attribute_specifier_sequence", "annotation", "decorator":
		return true
	}
	return false
}

func isTrailingComment(comment *syntaxNode) bool {
	previous := comment.PrevNamedSibling()
	return previous != nil && !isCommentNode(previous) && previous.EndLine() == comment.StartLine()
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

func buildMatchingStructureSegments(node *syntaxNode, content string, lines []string, hits matchLines, config *structureRules) []Segment {
	start := leadingCommentStart(node)
	declarationStart := node.StartLine()
	node = unwrapExport(node, config)
	end := node.EndLine()
	if shouldCompactJSXFunction(node, hits, config) {
		return buildCompactFunctionSegments(node, content, lines, hits, config, start, declarationStart)
	}
	if !config.containerTypes.contains(node.Kind()) {
		return []Segment{{Kind: "lines", Start: start, End: end}}
	}

	children := structuralChildren(node, config)
	if !anyChildHasMatch(children, hits) {
		return []Segment{{Kind: "lines", Start: start, End: end}}
	}
	segments := []Segment{{Kind: "lines", Start: start, End: declarationStart}}
	segments = append(segments, containerChildSegments(children, content, lines, hits)...)
	if end > start {
		segments = append(segments, Segment{Kind: "lines", Start: end, End: end})
	}
	return segments
}

// unwrapExport descends through export wrappers to the declaration they
// export (the container or function inside), returning the node itself when
// it is not an export wrapper.
func unwrapExport(node *syntaxNode, config *structureRules) *syntaxNode {
	for config.exportTypes.contains(node.Kind()) {
		next := node
		for _, child := range node.NamedChildren() {
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
func anyChildHasMatch(children []*syntaxNode, hits matchLines) bool {
	for _, child := range children {
		if hits.hitsRange(leadingCommentStart(child), child.EndLine()) {
			return true
		}
	}
	return false
}

// containerChildSegments renders each structural child: full lines when it
// contains a match, a one-line summary otherwise.
func containerChildSegments(children []*syntaxNode, content string, lines []string, hits matchLines) []Segment {
	var segments []Segment
	for _, child := range children {
		childStart, childEnd := child.StartLine(), child.EndLine()
		matchStart := leadingCommentStart(child)
		if hits.hitsRange(matchStart, childEnd) {
			segments = append(segments, Segment{Kind: "lines", Start: matchStart, End: childEnd})
		} else {
			segments = append(segments, Segment{Kind: "summary", Start: childStart, End: childEnd, Text: summarizeNode(child, content, lines)})
		}
	}
	return segments
}

func shouldCompactJSXFunction(node *syntaxNode, hits matchLines, config *structureRules) bool {
	return config.functionLikeTypes.contains(node.Kind()) &&
		containsNodeType(node, config.jsxElementTypes) &&
		nodeHasMatchInTypes(node, hits, config.jsxElementTypes) &&
		node.EndLine()-node.StartLine() >= 6
}

func buildCompactFunctionSegments(node *syntaxNode, content string, lines []string, hits matchLines, config *structureRules, start, declarationStart int) []Segment {
	end := node.EndLine()
	body := node.ChildByFieldName("body")
	if body == nil {
		for _, child := range node.NamedChildren() {
			if config.blockTypes.contains(child.Kind()) {
				body = child
				break
			}
		}
	}
	if body == nil {
		return []Segment{{Kind: "lines", Start: start, End: end}}
	}
	segments := []Segment{{Kind: "lines", Start: start, End: declarationStart}}
	for _, child := range body.NamedChildren() {
		childStart, childEnd := child.StartLine(), child.EndLine()
		matchStart := leadingCommentStart(child)
		if hits.hitsRange(matchStart, childEnd) {
			if containsNodeType(child, config.jsxElementTypes) {
				segments = append(segments, focusedLineSegments(childStart, childEnd, hits)...)
			} else {
				segments = append(segments, Segment{Kind: "lines", Start: matchStart, End: childEnd})
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

func containsNodeType(node *syntaxNode, types stringSet) bool {
	if len(types) == 0 {
		return false
	}
	found := false
	node.WalkNamed(func(candidate *syntaxNode) {
		if !found && types.contains(candidate.Kind()) {
			found = true
		}
	})
	return found
}

func nodeHasMatchInTypes(node *syntaxNode, hits matchLines, types stringSet) bool {
	found := false
	node.WalkNamed(func(candidate *syntaxNode) {
		if !found && types.contains(candidate.Kind()) && hits.hitsRange(candidate.StartLine(), candidate.EndLine()) {
			found = true
		}
	})
	return found
}

func structuralChildren(node *syntaxNode, config *structureRules) []*syntaxNode {
	if config.classDeclarationTypes.contains(node.Kind()) {
		body := node.ChildByFieldName("body")
		if body == nil {
			for _, child := range node.NamedChildren() {
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

func childrenInTypes(node *syntaxNode, types stringSet) []*syntaxNode {
	var children []*syntaxNode
	for _, child := range node.NamedChildren() {
		if types.contains(child.Kind()) {
			children = append(children, child)
		}
	}
	return children
}

func summarizeNode(node *syntaxNode, _ string, lines []string) string {
	start, end := node.StartLine(), node.EndLine()
	firstLine := ""
	if start >= 1 && start <= len(lines) {
		firstLine = lines[start-1]
	}
	if strings.TrimSpace(firstLine) == "" {
		for _, line := range strings.Split(node.Text(), "\n") {
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
