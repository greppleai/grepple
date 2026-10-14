package parser

import "strings"

// ScopeSelector is a typed local-only request for current parser-verified lines.
// Exact distinguishes @kind:START-END from the @kind:LINE shorthand.
type ScopeSelector struct {
	Kind      string
	StartLine int
	EndLine   int
	Exact     bool
}

// ScopeSelectionStatus reports verified, absent or ambiguous local syntax.
type ScopeSelectionStatus string

// ScopeVerified, ScopeMissing and ScopeAmbiguous classify a selector outcome.
const (
	ScopeVerified  ScopeSelectionStatus = "verified"
	ScopeMissing   ScopeSelectionStatus = "missing"
	ScopeAmbiguous ScopeSelectionStatus = "ambiguous"
)

var publicScopeKinds = map[string]bool{
	"func": true, "method": true, "class": true, "interface": true,
	"struct": true, "enum": true, "if": true, "while": true, "for": true,
	"switch": true, "case": true, "try": true, "catch": true,
	"block": true, "rule": true, "property": true, "snippet": true,
	"section": true, "object": true, "array": true, "mapping": true,
	"sequence": true, "document": true,
}

// PublicScopeKind reports whether a kind belongs to the published selector vocabulary.
func PublicScopeKind(kind string) bool { return publicScopeKinds[kind] }

// ResolveScopeSelector checks current classified syntax, never annotation-shaped
// source text. The optional document must own content and match language. Code
// selectors reuse its syntax tree; specialized content retains its own parser.
func ResolveScopeSelector(content, language string, selector ScopeSelector, document *Document) (ScopeLineRange, ScopeSelectionStatus) {
	if !PublicScopeKind(selector.Kind) || selector.StartLine < 1 || selector.EndLine < selector.StartLine || strings.IndexByte(content, 0) >= 0 || selector.EndLine > len(splitLines(content)) {
		return ScopeLineRange{}, ScopeMissing
	}
	if document != nil && (document.Source() != content || document.Language() != language) {
		return ScopeLineRange{}, ScopeMissing
	}
	if languageAdapters[language] != nil {
		if document == nil {
			var err error
			document, err = parseDocument(language, content)
			if err != nil {
				return ScopeLineRange{}, ScopeMissing
			}
			defer document.Close()
		}
		if selector.Exact {
			return codeExactScope(document, selector)
		}
		roles, _ := codeLineScopeAndRangesFromDocument(document, map[int]bool{selector.StartLine: true}, false, false)
		return selectScopeRole(roles[selector.StartLine], selector)
	}
	if document != nil && language != "markdown" {
		return ScopeLineRange{}, ScopeMissing
	}
	if language == "markdown" {
		return markdownScopeSelector(content, selector)
	}
	if language == "json" || language == "yaml" {
		if selector.Exact {
			span, found, ambiguous := structuredExactScope(content, language, selector)
			if ambiguous {
				return ScopeLineRange{}, ScopeAmbiguous
			}
			if found {
				return span, ScopeVerified
			}
			return ScopeLineRange{}, ScopeMissing
		}
		return selectScopeRole(structuredLineScopeAnnotations(content, language, map[int]bool{selector.StartLine: true})[selector.StartLine], selector)
	}
	return ScopeLineRange{}, ScopeMissing
}

func selectScopeRole(annotation LineScopeAnnotation, selector ScopeSelector) (ScopeLineRange, ScopeSelectionStatus) {
	var selected *ScopeLineRange
	for _, span := range []*ScopeLineRange{annotation.Nearest, annotation.Scope} {
		if span == nil || span.Kind != selector.Kind || selector.StartLine < span.StartLine || selector.StartLine > span.EndLine {
			continue
		}
		if selector.Exact && (span.StartLine != selector.StartLine || span.EndLine != selector.EndLine) {
			continue
		}
		if selected != nil && *selected != *span {
			return ScopeLineRange{}, ScopeAmbiguous
		}
		selected = span
	}
	if selected == nil {
		return ScopeLineRange{}, ScopeMissing
	}
	return *selected, ScopeVerified
}

// An explicit code span may be hidden by a closer nearest at its own start
// line; walk only nodes containing that span using the same kind/error rules
// as the line-only collector. Never choose a raw tree-sitter kind as a label.
func codeExactScope(document *Document, selector ScopeSelector) (ScopeLineRange, ScopeSelectionStatus) {
	requested := ScopeLineRange{Kind: selector.Kind, StartLine: selector.StartLine, EndLine: selector.EndLine}
	found, ambiguous, column, depth := false, false, 0, 0
	err := document.Read(func(view DocumentView) error {
		stack := []scopeWalkItem{{viewWalkItem: viewWalkItem{node: view.Root(), depth: 1}}}
		for len(stack) > 0 {
			current := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			node := current.node
			if !node.Valid() {
				continue
			}
			r := node.Range()
			end := inclusiveEndLine(r)
			if r.Start.Line > selector.StartLine || end < selector.EndLine {
				continue
			}
			if current.depth > 1 && !node.IsError() && !node.IsMissing() && !node.HasError() && r.Start.Line == selector.StartLine && end == selector.EndLine && scopeKindForNode(node) == selector.Kind {
				if found && depth == current.depth && column != r.Start.Column {
					ambiguous = true
				}
				found, column, depth = true, r.Start.Column, current.depth
			}
			if injected := node.raw.injectionRoot(); injected != nil {
				stack = append(stack, scopeWalkItem{viewWalkItem: viewWalkItem{node: ViewNode{view: node.view, raw: injected}, depth: current.depth + 1}, injected: true})
			}
			children := node.NamedChildren()
			for i := len(children) - 1; i >= 0; i-- {
				stack = append(stack, scopeWalkItem{viewWalkItem: viewWalkItem{node: children[i], depth: current.depth + 1}, injected: current.injected})
			}
		}
		return nil
	})
	if err != nil || !found {
		return ScopeLineRange{}, ScopeMissing
	}
	if ambiguous {
		return ScopeLineRange{}, ScopeAmbiguous
	}
	return requested, ScopeVerified
}
