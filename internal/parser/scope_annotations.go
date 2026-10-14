package parser

import "sort"

// ScopeLineRange is a parser-established, one-based inclusive structural span.
// Kind belongs to the fixed public vocabulary in language_scope_kinds.go.
type ScopeLineRange struct {
	Kind      string `json:"kind"`
	StartLine int    `json:"startLine"`
	EndLine   int    `json:"endLine"`
}

// LineScopeAnnotation keeps the closest classified construct and its containing
// navigable scope separate. Either role may be absent or refer to the same node.
type LineScopeAnnotation struct {
	Nearest *ScopeLineRange `json:"nearest,omitempty"`
	Scope   *ScopeLineRange `json:"scope,omitempty"`
}

type scopeCandidate struct {
	span   ScopeLineRange
	depth  int
	column int
}

type scopeCollector struct {
	lines   []int
	maxLine int
	nearest map[int]scopeCandidate
	scope   map[int]scopeCandidate
}

// CodeLineScopeAnnotations parses one file once and projects typed spans for
// requested hits. Unsupported languages, malformed constructs and unknown kinds
// are omitted without affecting matches. Specialized content has separate owners.
func CodeLineScopeAnnotations(content, language string, lines map[int]bool) map[int]LineScopeAnnotation {
	annotations, _ := codeLineScopeAndRanges(content, language, lines, false, false)
	return annotations
}

// codeLineScopeAndRanges shares one parsed document between the new typed
// projection and the existing range collector when line-only needs both.
func codeLineScopeAndRanges(content, language string, lines map[int]bool, includeRanges, enclosing bool) (map[int]LineScopeAnnotation, map[int]StructuralLineRange) {
	if len(lines) == 0 || languageAdapters[language] == nil {
		return nil, nil
	}
	document, err := parseDocument(language, content)
	if err != nil {
		return nil, nil
	}
	defer document.Close()
	return codeLineScopeAndRangesFromDocument(document, lines, includeRanges, enclosing)
}

func codeLineScopeAndRangesFromDocument(document *Document, lines map[int]bool, includeRanges, enclosing bool) (map[int]LineScopeAnnotation, map[int]StructuralLineRange) {
	if document == nil || len(lines) == 0 || languageAdapters[document.Language()] == nil {
		return nil, nil
	}
	content := document.Source()
	collector := &scopeCollector{
		maxLine: len(splitLines(content)),
		nearest: make(map[int]scopeCandidate, len(lines)),
		scope:   make(map[int]scopeCandidate, len(lines)),
	}
	for line := range lines {
		if line >= 1 && line <= collector.maxLine {
			collector.lines = append(collector.lines, line)
		}
	}
	sort.Ints(collector.lines)
	if len(collector.lines) == 0 {
		return nil, nil
	}
	var oldRanges *structuralLineRangeCollector
	if includeRanges {
		oldRanges = newStructuralLineRangeCollector(lines, enclosing)
	}
	_ = document.Read(func(view DocumentView) error {
		collector.walk(view.Root(), oldRanges)
		return nil
	})
	result := make(map[int]LineScopeAnnotation, len(collector.lines))
	for _, line := range collector.lines {
		nearest, hasNearest := collector.nearest[line]
		scope, hasScope := collector.scope[line]
		if !hasNearest && !hasScope {
			continue
		}
		annotation := LineScopeAnnotation{}
		if hasNearest {
			value := nearest.span
			annotation.Nearest = &value
		}
		if hasScope {
			value := scope.span
			annotation.Scope = &value
		}
		result[line] = annotation
	}
	if oldRanges != nil {
		return result, oldRanges.result()
	}
	return result, nil
}

type scopeWalkItem struct {
	viewWalkItem
	injected bool
}

func (collector *scopeCollector) walk(root ViewNode, oldRanges *structuralLineRangeCollector) {
	stack := []scopeWalkItem{{viewWalkItem: viewWalkItem{node: root, depth: 1}}}
	for len(stack) > 0 {
		index := len(stack) - 1
		current := stack[index]
		stack = stack[:index]
		node := current.node
		if !node.Valid() {
			continue
		}
		r := node.Range()
		end := inclusiveEndLine(r)
		if r.Start.Line < 1 || end > collector.maxLine || !rangeContainsLine(collector.lines, r.Start.Line, end) {
			continue
		}
		// The old collector never traversed injected languages: keep its
		// historical ranges while sharing the containing document walk.
		if oldRanges != nil && !current.injected {
			oldRanges.visit(node, current.depth)
		}
		if current.depth > 1 && !node.IsError() && !node.IsMissing() && !node.HasError() {
			if kind := scopeKindForNode(node); kind != "" {
				collector.add(node, kind, r.Start.Line, end, r.Start.Column, current.depth)
			}
		}
		// Include a Svelte injection exactly once and preserve source bounds.
		if injected := node.raw.injectionRoot(); injected != nil {
			stack = append(stack, scopeWalkItem{viewWalkItem: viewWalkItem{node: ViewNode{view: node.view, raw: injected}, depth: current.depth + 1}, injected: true})
		}
		children := node.NamedChildren() // cursor traversal, not O(siblings²) indexed TSNode lookups
		for i := len(children) - 1; i >= 0; i-- {
			stack = append(stack, scopeWalkItem{viewWalkItem: viewWalkItem{node: children[i], depth: current.depth + 1}, injected: current.injected})
		}
	}
}

func (collector *scopeCollector) add(node ViewNode, kind string, start, end, column, depth int) {
	span := ScopeLineRange{Kind: kind, StartLine: start, EndLine: end}
	language := node.Language()
	if node.raw.tree != nil && node.raw.tree.language != "" {
		language = node.raw.tree.language
	}
	isScope := navigableScopeKind(language, kind)
	index := sort.SearchInts(collector.lines, start)
	for index < len(collector.lines) && collector.lines[index] <= end {
		line := collector.lines[index]
		candidate := scopeCandidate{span: span, depth: depth, column: column}
		if current, ok := collector.nearest[line]; !ok || betterScopeCandidate(candidate, current) {
			collector.nearest[line] = candidate
		}
		if isScope {
			if current, ok := collector.scope[line]; !ok || betterScopeCandidate(candidate, current) {
				collector.scope[line] = candidate
			}
		}
		index++
	}
}

func betterScopeCandidate(next, current scopeCandidate) bool {
	nextWidth := next.span.EndLine - next.span.StartLine
	currentWidth := current.span.EndLine - current.span.StartLine
	if nextWidth != currentWidth {
		return nextWidth < currentWidth
	}
	if next.depth != current.depth {
		return next.depth > current.depth
	}
	return next.column > current.column
}
