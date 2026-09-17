package parser

import (
	"sort"
	"strings"
	"unicode/utf8"
)

// matchLines carries matched lines as both a membership set and an ascending
// slice so repeated range queries avoid scanning every match.
type matchLines struct {
	set    map[int]bool
	sorted []int
}

func newMatchLines(hits map[int]bool) matchLines {
	sorted := make([]int, 0, len(hits))
	for line := range hits {
		sorted = append(sorted, line)
	}
	sort.Ints(sorted)
	return matchLines{set: hits, sorted: sorted}
}

func (m matchLines) hitsRange(start, end int) bool {
	index := sort.Search(len(m.sorted), func(index int) bool { return m.sorted[index] >= start })
	return index < len(m.sorted) && m.sorted[index] <= end
}

func isPunctuation(node *syntaxNode) bool {
	kind := node.Kind()
	return len(kind) == 1 && !strings.ContainsAny(kind, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
}

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

// BuildSegmentsFromDocument constructs structural segments without reparsing the caller-owned document.
func BuildSegmentsFromDocument(document *Document, hitLines map[int]bool, maxSegments int) ([]Segment, SegmentBuildStatus) {
	if document == nil {
		return buildPlainTextSegments(hitLines, maxSegments), SegmentBuildFailed
	}
	document.mu.RLock()
	defer document.mu.RUnlock()
	if document.tree == nil || !utf8.ValidString(document.source) {
		return buildPlainTextSegments(hitLines, maxSegments), SegmentBuildFailed
	}
	adapter := adapterForLanguage(document.language)
	if adapter == nil {
		return buildPlainTextSegments(hitLines, maxSegments), SegmentBuildUnsupported
	}
	root := document.tree.RootNode()
	segments := buildASTSegments(root, document.source, newMatchLines(hitLines), adapter.Rules(), maxSegments)
	if root.HasError() {
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
	lines := splitLines(content)
	topLevel := nonPunctuationChildren(root)

	var segments []Segment
	for _, node := range topLevel {
		end := node.EndLine()
		matchStart := leadingCommentStart(node)
		if hits.hitsRange(matchStart, end) {
			segments = append(segments, buildMatchingStructureSegments(node, hits, config)...)
			continue
		}
	}
	segments = uncoveredLineSegments(segments, hits)
	return limitMatchedSegments(mergeSegments(segments, len(lines)), maxSegments, hits)
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

func buildMatchingStructureSegments(node *syntaxNode, hits matchLines, config *structureRules) []Segment {
	start := leadingCommentStart(node)
	declarationStart := node.StartLine()
	node = unwrapExport(node, config)
	if config.functionLikeTypes.contains(node.Kind()) {
		return []Segment{{Kind: "lines", Start: start, End: node.EndLine()}}
	}
	end := node.EndLine()
	if !config.containerTypes.contains(node.Kind()) {
		return []Segment{{Kind: "lines", Start: start, End: end}}
	}

	children := structuralChildren(node, config)
	if !anyChildHasMatch(children, hits) {
		return []Segment{{Kind: "lines", Start: start, End: end}}
	}
	segments := []Segment{{Kind: "lines", Start: start, End: declarationStart}}
	segments = append(segments, matchingContainerChildSegments(children, hits)...)
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

// matchingContainerChildSegments renders only structural children containing direct matches.
func matchingContainerChildSegments(children []*syntaxNode, hits matchLines) []Segment {
	var segments []Segment
	for _, child := range children {
		childEnd := child.EndLine()
		matchStart := leadingCommentStart(child)
		if hits.hitsRange(matchStart, childEnd) {
			segments = append(segments, Segment{Kind: "lines", Start: matchStart, End: childEnd})
		}
	}
	return segments
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

func limitMatchedSegments(segments []Segment, maxSegments int, hits matchLines) []Segment {
	if len(segments) <= maxSegments {
		return segments
	}
	direct, context := make([]Segment, 0, len(segments)), make([]Segment, 0, len(segments))
	for _, segment := range segments {
		if hits.hitsRange(segment.Start, segment.End) {
			direct = append(direct, segment)
		} else {
			context = append(context, segment)
		}
	}
	kept := append([]Segment{}, direct[:min(len(direct), maxSegments)]...)
	remaining := maxSegments - len(kept)
	if remaining > 0 {
		kept = append(kept, context[:min(len(context), remaining)]...)
	}
	sort.SliceStable(kept, func(i, j int) bool { return kept[i].Start < kept[j].Start })
	return kept
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
