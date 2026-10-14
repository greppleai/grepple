package parser

import (
	"encoding/json"
	yaml "go.yaml.in/yaml/v3"
	"io"
	"sort"
	"strings"
)

type structuredPosition struct{ line, column int }
type structuredContainer struct {
	span          ScopeLineRange
	column, depth int
}
type structuredEnd struct {
	line int
	ok   bool
}
type structuredProjection struct {
	lines         []int
	first, second map[int]scopeCandidate
	ambiguous     map[int]bool
	bounds        map[structuredPosition]int
	ends          map[*yaml.Node]structuredEnd
	content       []string
	format        string
	// Exact selectors share the ordinary full-stream validation and container
	// bounds, but need not allocate a hit for every line of a large scope.
	target          *ScopeLineRange
	found, conflict bool
	column, depth   int
}

func structuredLineScopeAnnotations(content, language string, hits map[int]bool) map[int]LineScopeAnnotation {
	result, _, _ := projectStructuredScopes(content, language, hits, nil)
	return result
}

func structuredExactScope(content, language string, selector ScopeSelector) (ScopeLineRange, bool, bool) {
	target := ScopeLineRange{Kind: selector.Kind, StartLine: selector.StartLine, EndLine: selector.EndLine}
	_, found, conflict := projectStructuredScopes(content, language, map[int]bool{selector.StartLine: true}, &target)
	return target, found, conflict
}

func projectStructuredScopes(content, language string, hits map[int]bool, target *ScopeLineRange) (map[int]LineScopeAnnotation, bool, bool) {
	if language == "json" && !json.Valid([]byte(content)) {
		return nil, false, false
	}
	p := &structuredProjection{first: make(map[int]scopeCandidate), second: make(map[int]scopeCandidate), ambiguous: make(map[int]bool), ends: make(map[*yaml.Node]structuredEnd), content: splitLines(content), format: language, target: target}
	for line := range hits {
		if line >= 1 && line <= len(p.content) && structuredPayloadLine(p.content[line-1], language) {
			p.lines = append(p.lines, line)
		}
	}
	if len(p.lines) == 0 && target == nil {
		return nil, false, false
	}
	sort.Ints(p.lines)
	if language == "json" {
		p.bounds = jsonContainerEnds(content)
		if p.bounds == nil {
			return nil, false, false
		}
	}
	// Decode the whole stream before annotating; malformed later documents invalidate all.
	dec := yaml.NewDecoder(strings.NewReader(content))
	var roots []*yaml.Node
	for {
		var doc yaml.Node
		err := dec.Decode(&doc)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, false, false
		}
		if root := documentRoot(&doc); root != nil {
			roots = append(roots, root)
		}
	}
	if len(roots) == 0 || language == "json" && len(roots) != 1 {
		return nil, false, false
	}
	for _, root := range roots {
		end, ok := p.containerEnd(root)
		if ok && validStructuredRange(root.Line, end, len(p.content)) && (len(roots) > 1 || !yamlIsContainer(root)) {
			p.add(structuredContainer{span: ScopeLineRange{"document", root.Line, end}, depth: 0, column: root.Column})
		}
		// An unbounded flow or multiline value invalidates its parent, but
		// independently bounded sibling containers may still be trustworthy.
		p.walk(root, root.Line, 1)
	}
	result := make(map[int]LineScopeAnnotation, len(p.first))
	for _, line := range p.lines {
		first, ok := p.first[line]
		if !ok || p.ambiguous[line] {
			continue
		}
		nearest, scope := first.span, first.span
		if second, ok := p.second[line]; ok {
			scope = second.span
		}
		result[line] = LineScopeAnnotation{Nearest: &nearest, Scope: &scope}
	}
	return result, p.found, p.conflict
}

func structuredPayloadLine(line, language string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}
	if language == "yaml" && (strings.HasPrefix(trimmed, "#") || trimmed == "---" || trimmed == "...") {
		return false
	}
	return true
}
func validStructuredRange(start, end, maxLine int) bool {
	return start > 0 && start <= end && end <= maxLine
}

func (p *structuredProjection) walk(node *yaml.Node, start, depth int) {
	if node == nil || !yamlIsContainer(node) {
		return
	}
	end, ok := p.containerEnd(node)
	if ok && validStructuredRange(start, end, len(p.content)) {
		kind := "mapping"
		if node.Kind == yaml.SequenceNode {
			kind = "sequence"
		}
		if p.format == "json" {
			if node.Kind == yaml.MappingNode {
				kind = "object"
			} else {
				kind = "array"
			}
		}
		p.add(structuredContainer{span: ScopeLineRange{kind, start, end}, column: node.Column, depth: depth})
	}
	if node.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(node.Content); i += 2 {
			key, value := node.Content[i], node.Content[i+1]
			if key.Line > 0 && key.Line <= value.Line && !strings.Contains(key.Value, "\n") {
				p.walk(value, key.Line, depth+1)
			}
		}
	} else {
		for _, value := range node.Content {
			p.walk(value, value.Line, depth+1)
		}
	}
}

func (p *structuredProjection) containerEnd(node *yaml.Node) (int, bool) {
	if value, ok := p.ends[node]; ok {
		return value.line, value.ok
	}
	line, ok := p.uncachedContainerEnd(node)
	p.ends[node] = structuredEnd{line, ok}
	return line, ok
}

func (p *structuredProjection) uncachedContainerEnd(node *yaml.Node) (int, bool) {
	if node == nil {
		return 0, false
	}
	if p.format == "json" {
		if !yamlIsContainer(node) {
			return node.Line, true
		}
		end, ok := p.bounds[structuredPosition{node.Line, node.Column}]
		return end, ok
	}
	// yaml.v3 stores starts only. Flow collections and multiline scalars lack
	// provable closing bounds; omit them and any ancestors depending on them.
	if node.Style&yaml.FlowStyle != 0 || node.Kind == yaml.AliasNode {
		return 0, false
	}
	if node.Kind == yaml.ScalarNode {
		if node.Style&(yaml.LiteralStyle|yaml.FoldedStyle) != 0 || strings.Contains(node.Value, "\n") {
			return 0, false
		}
		return node.Line, true
	}
	end := node.Line
	for _, child := range node.Content {
		childEnd, ok := p.containerEnd(child)
		if !ok {
			return 0, false
		}
		if childEnd > end {
			end = childEnd
		}
	}
	return end, true
}

func (p *structuredProjection) add(container structuredContainer) {
	candidate := scopeCandidate{span: container.span, depth: container.depth, column: container.column}
	if p.target != nil && *p.target == candidate.span {
		if p.found && p.depth == candidate.depth && p.column != candidate.column {
			p.conflict = true
		}
		p.found, p.column, p.depth = true, candidate.column, candidate.depth
	}
	for index := sort.SearchInts(p.lines, container.span.StartLine); index < len(p.lines) && p.lines[index] <= container.span.EndLine; index++ {
		line := p.lines[index]
		first, hasFirst := p.first[line]
		// Two disjoint same-depth containers can occupy one source line.
		// Without a match column neither is provably the nearest hit owner.
		if hasFirst && first.span == candidate.span && first.depth == candidate.depth && first.column != candidate.column {
			p.ambiguous[line] = true
		}
		if !hasFirst || betterScopeCandidate(candidate, first) {
			if hasFirst && candidate.span != first.span {
				p.considerSecond(line, first)
			}
			p.first[line] = candidate
		} else if candidate.span != first.span {
			p.considerSecond(line, candidate)
		}
		if second, ok := p.second[line]; ok && second.span == p.first[line].span {
			delete(p.second, line)
		}
	}
}
func (p *structuredProjection) considerSecond(line int, candidate scopeCandidate) {
	if second, ok := p.second[line]; !ok || betterScopeCandidate(candidate, second) {
		p.second[line] = candidate
	}
}

// jsonContainerEnds scans delimiters in already-validated JSON, ignoring
// quoted braces. AST node positions must coincide with an opening delimiter.
func jsonContainerEnds(content string) map[structuredPosition]int {
	type opener struct {
		position structuredPosition
		byteKind byte
	}
	bounds := make(map[structuredPosition]int)
	var stack []opener
	line, column := 1, 1
	inString, escaped := false, false
	for i := 0; i < len(content); i++ {
		ch := content[i]
		if inString {
			if escaped {
				escaped = false
			} else if ch == '\\' {
				escaped = true
			} else if ch == '"' {
				inString = false
			}
		} else {
			switch ch {
			case '"':
				inString = true
			case '{', '[':
				stack = append(stack, opener{structuredPosition{line, column}, ch})
			case '}', ']':
				if len(stack) == 0 {
					return nil
				}
				last := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				if last.byteKind == '{' && ch != '}' || last.byteKind == '[' && ch != ']' {
					return nil
				}
				bounds[last.position] = line
			}
		}
		if ch == '\n' {
			line++
			column = 1
		} else {
			column++
		}
	}
	if inString || len(stack) != 0 {
		return nil
	}
	return bounds
}
