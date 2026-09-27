package gritql

import "fmt"

// GoCyclomaticMetricSpec models McCabe complexity as one independent path plus
// one per if, loop, non-default switch/select arm, and short-circuit operator.
// Several case labels sharing a body form one arm. Nested function literals do
// not contribute to the enclosing named function's score.
func GoCyclomaticMetricSpec() (MetricSpec, error) {
	scope, err := compileGoMetricSelector("or { function_declaration(), method_declaration() }")
	if err != nil {
		return MetricSpec{}, err
	}
	boundary, err := compileGoMetricSelector("func_literal()")
	if err != nil {
		return MetricSpec{}, err
	}
	spec := MetricSpec{Scope: scope, Boundary: boundary, NameField: "name", Base: 1}
	for _, item := range []struct{ id, query, logical string }{
		{"if", "if_statement()", ""},
		{"loop", "for_statement()", ""},
		{"arm", "or { expression_case(), type_case(), communication_case() }", ""},
		{"short-circuit", "binary_expression()", "each"},
	} {
		query, err := compileGoMetricSelector(item.query)
		if err != nil {
			return MetricSpec{}, err
		}
		rule := MetricRule{ID: item.id, Query: query, Points: 1, Logical: item.logical}
		if item.logical != "" {
			rule.Operators = []string{"&&", "||"}
		}
		spec.Rules = append(spec.Rules, rule)
	}
	return spec, spec.Validate()
}

// GoCognitiveMetricSpec models Revive's structural nesting, else-if, logical
// runs, labeled jumps, and direct self-call increments. Self-calls use lexical
// names, not Go object resolution; therefore this spec MUST NOT replace Revive
// until semantic recursion parity is established (notably under shadowing).
func GoCognitiveMetricSpec() (MetricSpec, error) {
	scope, err := compileGoMetricSelector("or { function_declaration(), method_declaration() }")
	if err != nil {
		return MetricSpec{}, err
	}
	spec := MetricSpec{Scope: scope, NameField: "name"}
	for _, item := range []struct {
		id, query     string
		points, depth int
		opens         bool
	}{
		{"if", "if_statement()", 1, 1, true},
		{"loop", "for_statement()", 1, 1, true},
		{"switch", "or { expression_switch_statement(), type_switch_statement(), select_statement() }", 1, 1, true},
		{"closure", "func_literal()", 0, 1, true},
		{"boolean-run", "binary_expression()", 1, 0, false},
		{"labeled-jump", "or { break_statement(), continue_statement(), goto_statement() }", 1, 0, false},
		{"direct-recursion", "call_expression()", 1, 0, false},
	} {
		query, err := compileGoMetricSelector(item.query)
		if err != nil {
			return MetricSpec{}, err
		}
		rule := MetricRule{ID: item.id, Query: query, Points: item.points, NestingWeight: item.depth, OpensNesting: item.opens}
		switch item.id {
		case "if":
			rule.FlatAlternativeField = "alternative"
		case "boolean-run":
			rule.Logical = "runs"
			rule.Operators = []string{"&&", "||"}
		case "labeled-jump":
			rule.RequireChildKind = "label_name"
		case "direct-recursion":
			rule.SelfCallField = "function"
		}
		spec.Rules = append(spec.Rules, rule)
	}
	return spec, spec.Validate()
}

func compileGoMetricSelector(query string) (*Program, error) {
	program, err := Compile([]byte("language go\n"+query), CompileOptions{})
	if err != nil {
		return nil, fmt.Errorf("invalid Go metric selector %q: %w", query, err)
	}
	return program, nil
}
