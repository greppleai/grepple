package gritql

import (
	"sort"
	"unicode"
)

// AnchorPlan describes source literals that every successful evaluation must
// contain. An empty plan explicitly requires a complete scoped candidate scan.
type AnchorPlan struct {
	required []string
}

// AnalyzeAnchors conservatively derives mandatory source literals from an
// immutable program. It returns an empty plan for nil or unanchored programs.
func AnalyzeAnchors(program *Program) AnchorPlan {
	if program == nil || program.root == nil {
		return AnchorPlan{}
	}
	set := requiredExpressionLiterals(program.root)
	literals := make([]string, 0, len(set))
	for literal := range set {
		literals = append(literals, literal)
	}
	sort.Strings(literals)
	return AnchorPlan{required: literals}
}

// HasSafeAnchor reports whether candidate selection may require at least one
// literal without introducing false negatives.
func (p AnchorPlan) HasSafeAnchor() bool { return len(p.required) != 0 }

// RequiredLiterals returns a sorted copy of literals present in every possible
// match. Callers may require any or all of them when selecting candidates.
func (p AnchorPlan) RequiredLiterals() []string {
	return append([]string(nil), p.required...)
}
func longestRequiredLiteral(program *Program) string {
	literals := AnalyzeAnchors(program).required
	longest := ""
	for _, literal := range literals {
		if len(literal) > len(longest) {
			longest = literal
		}
	}
	return longest
}

func requiredExpressionLiterals(expr *expression) map[string]struct{} {
	if expr == nil {
		return nil
	}
	switch expr.kind {
	case KindSnippet:
		return requiredSnippetLiterals(expr)
	case KindAnd:
		return requiredAndLiterals(expr.children)
	case KindOr:
		return requiredOrLiterals(expr.children)
	case KindContains, KindWithin:
		return requiredFirstChildLiterals(expr)
	case KindWhere:
		return requiredWhereLiterals(expr)
	case KindRegex, KindNot, KindMaybe:
		return nil
	default:
		return nil
	}
}

func requiredSnippetLiterals(expr *expression) map[string]struct{} {
	templates := expr.templates
	if len(templates) == 0 {
		templates = []Template{expr.template}
	}
	if len(templates) == 0 {
		return nil
	}
	literals := cloneLiteralSet(requiredTemplateLiterals(templates[0].root))
	for _, template := range templates[1:] {
		literals = intersectLiteralSets(literals, requiredTemplateLiterals(template.root))
	}
	return literals
}

func requiredTemplateLiterals(root *templateNode) map[string]struct{} {
	if root == nil {
		return nil
	}
	literals := make(map[string]struct{})
	stack := []*templateNode{root}
	for len(stack) > 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if candidateAnchorLiteral(node.token) {
			literals[node.token] = struct{}{}
		}
		for i := len(node.children) - 1; i >= 0; i-- {
			if child := node.children[i].node; child != nil {
				stack = append(stack, child)
			}
		}
	}
	return literals
}

func candidateAnchorLiteral(literal string) bool {
	for _, r := range literal {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || r == '_' {
			return true
		}
	}
	return false
}

func requiredAndLiterals(children []*expression) map[string]struct{} {
	var literals map[string]struct{}
	for _, child := range children {
		literals = unionLiteralSets(literals, requiredExpressionLiterals(child))
	}
	return literals
}

func requiredOrLiterals(children []*expression) map[string]struct{} {
	if len(children) == 0 {
		return nil
	}
	literals := cloneLiteralSet(requiredExpressionLiterals(children[0]))
	for _, child := range children[1:] {
		literals = intersectLiteralSets(literals, requiredExpressionLiterals(child))
	}
	return literals
}

func requiredFirstChildLiterals(expr *expression) map[string]struct{} {
	if len(expr.children) == 0 {
		return nil
	}
	return requiredExpressionLiterals(expr.children[0])
}

func requiredWhereLiterals(expr *expression) map[string]struct{} {
	literals := requiredFirstChildLiterals(expr)
	for _, item := range expr.constraints {
		literals = unionLiteralSets(literals, requiredExpressionLiterals(item.rhs))
	}
	return literals
}

func unionLiteralSets(left, right map[string]struct{}) map[string]struct{} {
	if len(right) == 0 {
		return left
	}
	if left == nil {
		left = make(map[string]struct{}, len(right))
	}
	for literal := range right {
		left[literal] = struct{}{}
	}
	return left
}

func intersectLiteralSets(left, right map[string]struct{}) map[string]struct{} {
	for literal := range left {
		if _, required := right[literal]; !required {
			delete(left, literal)
		}
	}
	return left
}

func cloneLiteralSet(source map[string]struct{}) map[string]struct{} {
	if len(source) == 0 {
		return nil
	}
	clone := make(map[string]struct{}, len(source))
	for literal := range source {
		clone[literal] = struct{}{}
	}
	return clone
}
