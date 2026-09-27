package gritql

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/greppleai/grepple/parser"
)

// MetricRule assigns a nonnegative contribution to each syntax node matched by
// Query. NestingWeight multiplies the number of enclosing OpensNesting rules.
// Logical is "", "each", or "runs": the latter counts only the first of each
// consecutive run of identical anonymous operators (parentheses break a run).
// Operators limits the relevant anonymous operator tokens, e.g. && and ||.
// FlatAlternativeField scores an else-if at Points rather than Points + nesting
// and does not add another nesting level. SelfCallField compares a direct name
// child with the enclosing scope's NameField; it is lexical, not name resolution.
type MetricRule struct {
	ID                   string
	Query                *Program
	Points               int
	NestingWeight        int
	OpensNesting         bool
	FlatAlternativeField string
	Logical              string
	Operators            []string
	SelfCallField        string
	RequireChildKind     string
}

// MetricSpec describes a bounded, per-function AST reduction over GritQL
// selectors. Boundary excludes nested subtrees (e.g. function literals) unless
// they themselves match Scope. Selectors must yield concrete node candidates.
type MetricSpec struct {
	Scope     *Program
	Boundary  *Program
	NameField string
	Base      int
	Rules     []MetricRule
}

// MetricContribution explains one contribution without retaining syntax handles.
type MetricContribution struct {
	Rule   string
	Range  parser.Range
	Points int
}

// MetricResult belongs to one scope matched by a GritQL program.
type MetricResult struct {
	Kind          string
	Range         parser.Range
	Score         int
	Contributions []MetricContribution
	Name          string // source-written function name when configured
}

// MetricsCompatibility identifies the versioned scoped numeric-analysis API.
const MetricsCompatibility = "gritql-metric-v1"

const maxMetricRules = 32

func (spec MetricSpec) Validate() error {
	if spec.Scope == nil || len(spec.Rules) == 0 || len(spec.Rules) > maxMetricRules || spec.Base < 0 || spec.Base > 1000 {
		return fmt.Errorf("metric requires a scope, 1..%d rules, and base 0..1000", maxMetricRules)
	}
	language := spec.Scope.Language()
	if language == "" || spec.Boundary != nil && spec.Boundary.Language() != language {
		return fmt.Errorf("metric selectors must use the same language")
	}
	if spec.NameField != "" && (!validNodeSelectorName(spec.NameField) || !metricSelectorHasField(spec.Scope, spec.NameField)) {
		return fmt.Errorf("invalid metric scope name field %q", spec.NameField)
	}
	seen := make(map[string]bool, len(spec.Rules))
	for _, rule := range spec.Rules {
		if rule.Query == nil || rule.Query.Language() != language || rule.ID == "" || seen[rule.ID] || rule.Points < 0 || rule.Points > 1000 || rule.NestingWeight < 0 || rule.NestingWeight > 1000 {
			return fmt.Errorf("metric rule %q has an invalid selector, id, or weight", rule.ID)
		}
		seen[rule.ID] = true
		if rule.Logical != "" && rule.Logical != "each" && rule.Logical != "runs" || rule.Logical == "" && len(rule.Operators) != 0 || rule.Logical != "" && len(rule.Operators) == 0 {
			return fmt.Errorf("metric rule %q has an invalid logical operator mode", rule.ID)
		}
		if rule.FlatAlternativeField != "" && (!validNodeSelectorName(rule.FlatAlternativeField) || !rule.OpensNesting) || rule.SelfCallField != "" && (spec.NameField == "" || !validNodeSelectorName(rule.SelfCallField)) {
			return fmt.Errorf("metric rule %q has invalid scope or nesting fields", rule.ID)
		}
		if rule.FlatAlternativeField != "" && !metricSelectorHasField(rule.Query, rule.FlatAlternativeField) || rule.SelfCallField != "" && !metricSelectorHasField(rule.Query, rule.SelfCallField) {
			return fmt.Errorf("metric rule %q uses an unknown grammar field", rule.ID)
		}
		if rule.RequireChildKind != "" && !parser.GrammarNodeKind(language, rule.RequireChildKind) {
			return fmt.Errorf("metric rule %q requires an unknown child kind %q", rule.ID, rule.RequireChildKind)
		}
		for _, op := range rule.Operators {
			if !parser.GrammarTokenKind(language, op) {
				return fmt.Errorf("metric rule %q uses unknown operator %q", rule.ID, op)
			}
		}
	}
	return nil
}

func metricSelectorHasField(program *Program, field string) bool {
	if !parser.GrammarFieldName(program.Language(), field) {
		return false
	}
	var check func(*expression) bool
	check = func(expr *expression) bool {
		switch expr.kind {
		case KindNodeLike:
			return parser.GrammarFieldCardinality(program.Language(), expr.text, field) != parser.GrammarCardinalityUnknown
		case KindOr:
			for _, branch := range expr.children {
				if !check(branch) {
					return false
				}
			}
			return true
		default:
			return true // a snippet or compound selector need not disclose a fixed root kind
		}
	}
	return check(program.root)
}

func (spec MetricSpec) programs() []*Program {
	programs := make([]*Program, 0, len(spec.Rules)+2)
	programs = append(programs, spec.Scope)
	for _, rule := range spec.Rules {
		programs = append(programs, rule.Query)
	}
	if spec.Boundary != nil {
		programs = append(programs, spec.Boundary)
	}
	return programs
}

type metricKey struct {
	kind       string
	start, end int
}

func metricNodeKey(kind string, rng parser.Range) metricKey {
	return metricKey{kind, rng.StartByte, rng.EndByte}
}

// AnalyzeMetrics evaluates all selectors on one shared, bounded candidate
// traversal, then folds their node facts in source order using that same step
// and deadline budget. Invalid source, cancellation, overflow, and truncation
// return an error and no scores.
func AnalyzeMetrics(ctx context.Context, spec MetricSpec, document *parser.Document, options EvaluateOptions) ([]MetricResult, error) {
	if err := spec.Validate(); err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	programs := spec.programs()
	for _, program := range programs {
		if err := validateEvaluationInputs(program, document); err != nil {
			return nil, err
		}
	}
	options = normalizeEvaluateOptions(options)
	if len(document.Source()) > options.MaxSourceBytes {
		return nil, evaluationFailure("LIMIT_SOURCE_BYTES", "resource", "source exceeds effective byte limit", nil)
	}
	budget := newEvaluationBudget(ctx, options)
	rows, err := evaluateProgramsWithBudget(programs, document, options, budget)
	if err != nil {
		return nil, err
	}
	scopes := make(map[metricKey]struct{})
	events := make(map[metricKey][]int)
	boundaries := make(map[metricKey]bool)
	total := 0
	for index, matches := range rows {
		for _, match := range matches {
			if !budget.take() {
				return nil, budget.err
			}
			kind, rng, valid := match.CandidateNode()
			if !valid {
				return nil, evaluationFailure("INVALID_METRIC_SELECTOR", "pattern", "metric selector matched a sequence rather than a syntax node", nil)
			}
			key := metricNodeKey(kind, rng)
			switch {
			case index == 0:
				scopes[key] = struct{}{}
			case index == len(rows)-1 && spec.Boundary != nil:
				boundaries[key] = true
			default:
				// One program may produce several records for one node; count it once.
				rule := index - 1
				if !containsMetricRule(events[key], rule) {
					events[key] = append(events[key], rule)
					total++
					if total > options.MaxCandidates {
						return nil, evaluationFailure("LIMIT_METRIC_EVENTS", "resource", "metric event limit exceeded", nil)
					}
				}
			}
		}
	}
	results, err := foldMetricTree(document, spec, scopes, events, boundaries, budget, options.MaxDepth)
	if err != nil {
		return nil, err
	}
	return results, nil
}

func containsMetricRule(rules []int, rule int) bool {
	for _, existing := range rules {
		if existing == rule {
			return true
		}
	}
	return false
}

type metricWalkItem struct {
	node           parser.ViewNode
	depth, nesting int
	scope          *MetricResult
}

func foldMetricTree(document *parser.Document, spec MetricSpec, scopes map[metricKey]struct{}, events map[metricKey][]int, boundaries map[metricKey]bool, budget *evaluationBudget, maxDepth int) ([]MetricResult, error) {
	var owned []*MetricResult
	seen := make(map[metricKey]bool)
	err := document.Read(func(view parser.DocumentView) error {
		stack := []metricWalkItem{{node: view.Root(), depth: 1}}
		for len(stack) != 0 {
			if !budget.take() {
				return budget.err
			}
			item := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if item.depth > maxDepth {
				return evaluationFailure("LIMIT_PARSE_DEPTH", "resource", "source tree exceeds effective depth limit", nil)
			}
			node := item.node
			key := metricNodeKey(node.Kind(), node.Range())
			_, isScope := scopes[key]
			matches := events[key]
			if isScope || len(matches) != 0 || boundaries[key] {
				if seen[key] {
					return evaluationFailure("AMBIGUOUS_METRIC_NODE", "source", "multiple syntax nodes have the same metric selector identity", nil)
				}
				seen[key] = true
			}
			if boundaries[key] && !isScope {
				continue
			}
			if isScope {
				item.scope = &MetricResult{Kind: node.Kind(), Range: node.Range(), Score: spec.Base}
				if spec.NameField != "" {
					name := node.ChildByFieldName(spec.NameField)
					if !name.Valid() || name.Text() == "" {
						return evaluationFailure("INVALID_METRIC_SELECTOR", "pattern", "metric scope has no configured name field", nil)
					}
					item.scope.Name = name.Text()
				}
				owned = append(owned, item.scope)
				item.nesting = 0
			}
			if item.scope != nil {
				increases := 0
				for _, index := range matches {
					rule := spec.Rules[index]
					flat := rule.FlatAlternativeField != "" && node.FieldName() == rule.FlatAlternativeField && containsMetricRule(events[metricNodeKey(node.Parent().Kind(), node.Parent().Range())], index)
					if !metricRuleApplies(rule, node, item.scope) {
						continue
					}
					points := rule.Points
					if !flat {
						if item.nesting > 0 && rule.NestingWeight > (math.MaxInt-points)/item.nesting {
							return evaluationFailure("LIMIT_METRIC_SCORE", "resource", "metric score overflow", nil)
						}
						points += item.nesting * rule.NestingWeight
					}
					if points > math.MaxInt-item.scope.Score {
						return evaluationFailure("LIMIT_METRIC_SCORE", "resource", "metric score overflow", nil)
					}
					item.scope.Score += points
					if points != 0 {
						item.scope.Contributions = append(item.scope.Contributions, MetricContribution{Rule: rule.ID, Range: node.Range(), Points: points})
					}
					if rule.OpensNesting && !flat {
						increases++
					}
				}
				item.nesting += increases
			}
			children := node.Children()
			for i := len(children) - 1; i >= 0; i-- {
				stack = append(stack, metricWalkItem{node: children[i], depth: item.depth + 1, nesting: item.nesting, scope: item.scope})
			}
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, parser.ErrDocumentClosed) {
			return nil, evaluationFailure("INTERNAL_ERROR", "internal", "source document closed during metric evaluation", err)
		}
		return nil, err
	}
	if len(owned) != len(scopes) {
		return nil, evaluationFailure("INTERNAL_ERROR", "internal", "unresolved metric scope selector", nil)
	}
	out := make([]MetricResult, len(owned))
	for i, result := range owned {
		out[i] = *result
	}
	return out, nil
}

func metricRuleApplies(rule MetricRule, node parser.ViewNode, scope *MetricResult) bool {
	if rule.RequireChildKind != "" {
		found := false
		for _, child := range node.NamedChildren() {
			if child.Kind() == rule.RequireChildKind {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if rule.Logical != "" {
		op := metricLogicalOperator(node, rule.Operators)
		if op == "" {
			return false
		}
		if rule.Logical == "runs" && node.Parent().Kind() == node.Kind() && metricLogicalOperator(node.Parent(), rule.Operators) == op {
			return false
		}
	}
	if rule.SelfCallField != "" {
		callee := node.ChildByFieldName(rule.SelfCallField)
		return callee.Valid() && callee.IsNamed() && callee.NamedChildCount() == 0 && callee.Text() != "" && callee.Text() == scope.Name
	}
	return true
}

func metricLogicalOperator(node parser.ViewNode, operators []string) string {
	for _, child := range node.Children() {
		if child.IsNamed() || child.IsExtra() {
			continue
		}
		for _, op := range operators {
			if child.Kind() == op {
				return op
			}
		}
	}
	return ""
}
