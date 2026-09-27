package gritql

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

// MetricQuery is a source-authored GritQL metric rule. Scores strictly greater
// than Above trigger the rule; zero is a valid threshold.
type MetricQuery struct {
	Spec  MetricSpec
	Above int
}

// CompileMetric compiles a closed, versioned GritQL metric document. Selectors
// within the document are compiled through the ordinary gritql-v1 compiler;
// other upstream map/list expressions are not enabled for structural search.
// Example: language go\n{ metric: { scope: function_declaration(), base: 1,
// above: 10, rules: [{id: "if", query: if_statement(), points: 1}] } }
func CompileMetric(source []byte, options CompileOptions) (*MetricQuery, error) {
	options = normalizedOptions(options)
	if len(source) > options.MaxPatternBytes {
		return nil, metricCompileFailure("LIMIT_PATTERN_BYTES", "resource", "metric query exceeds byte limit")
	}
	if bytes.IndexByte(source, 0) >= 0 || validQueryEncoding(source) != nil || hasBareCarriageReturn(source) {
		return nil, metricCompileFailure("PATTERN_PARSE", "pattern", "invalid metric query encoding")
	}
	owned := bytes.Clone(source)
	tree, err := parsePinnedQuery(owned)
	if err != nil {
		return nil, metricCompileFailure("PATTERN_PARSE", "pattern", err.Error())
	}
	doc := &queryDocument{source: owned, lineStarts: lineStarts(owned), tree: tree}
	defer doc.close()
	if diagnostics := doc.diagnostics(); len(diagnostics) != 0 {
		return nil, metricCompileFailure("PATTERN_PARSE", "pattern", diagnostics[0].Message)
	}
	root := doc.root()
	children := withoutComments(root.children())
	if root.kind() != "source_file" || len(children) != 2 || !isNode(children[0], "langdecl", "language", true) || !isNode(children[1], "map", "pattern", true) || !validEnvelopeSpacing(doc.source, children[0], children[1]) {
		return nil, metricCompileFailure("PATTERN_INVALID_CONTEXT", "pattern", "expected language header followed by a metric map")
	}
	language := children[0].childByFieldName("name").text()
	if _, ok := targetLanguageByID(language); !ok {
		return nil, metricCompileFailure("PATTERN_UNSUPPORTED", "unsupported", "unsupported metric language "+language)
	}
	outer, err := metricMap(children[1], []string{"metric"})
	if err != nil {
		return nil, err
	}
	config := outer["metric"]
	values, err := metricMap(config, []string{"scope", "boundary", "name", "base", "above", "rules"})
	if err != nil {
		return nil, err
	}
	if !values["scope"].valid() || !values["base"].valid() || !values["above"].valid() || !values["rules"].valid() {
		return nil, metricCompileFailure("PATTERN_INVALID_CONTEXT", "pattern", "metric requires scope, base, above, and rules")
	}
	spec := MetricSpec{}
	if spec.Scope, err = metricSelector(doc, language, values["scope"], options); err != nil {
		return nil, err
	}
	if values["boundary"].valid() {
		if spec.Boundary, err = metricSelector(doc, language, values["boundary"], options); err != nil {
			return nil, err
		}
	}
	if values["name"].valid() {
		if spec.NameField, err = metricString(values["name"]); err != nil {
			return nil, err
		}
	}
	if spec.Base, err = metricInt(values["base"]); err != nil {
		return nil, err
	}
	above, err := metricInt(values["above"])
	if err != nil {
		return nil, err
	}
	if above > 1_000_000 {
		return nil, metricCompileFailure("PATTERN_INVALID_CONTEXT", "pattern", "metric threshold exceeds 1000000")
	}
	if values["rules"].kind() != "list" {
		return nil, metricCompileFailure("PATTERN_INVALID_CONTEXT", "pattern", "metric rules must be a list")
	}
	for _, entry := range withoutComments(values["rules"].namedChildren()) {
		if entry.kind() != "map" {
			return nil, metricCompileFailure("PATTERN_INVALID_CONTEXT", "pattern", "metric rule must be a map")
		}
		rule, err := compileMetricRule(doc, language, entry, options)
		if err != nil {
			return nil, err
		}
		spec.Rules = append(spec.Rules, rule)
		if len(spec.Rules) > maxMetricRules {
			return nil, metricCompileFailure("LIMIT_PATTERN_BYTES", "resource", "too many metric rules")
		}
	}
	if err := spec.Validate(); err != nil {
		return nil, metricCompileFailure("PATTERN_INVALID_CONTEXT", "pattern", err.Error())
	}
	return &MetricQuery{Spec: spec, Above: above}, nil
}

func metricCompileFailure(code, class, message string) *CompileError {
	return &CompileError{Code: code, Class: class, Message: message}
}

func metricMap(node queryNode, allowed []string) (map[string]queryNode, error) {
	if node.kind() != "map" {
		return nil, metricCompileFailure("PATTERN_INVALID_CONTEXT", "pattern", "expected a metric map")
	}
	fields := make(map[string]queryNode)
	for _, entry := range withoutComments(node.namedChildren()) {
		if entry.kind() != "mapElement" {
			return nil, metricCompileFailure("PATTERN_INVALID_CONTEXT", "pattern", "invalid metric map member")
		}
		key := entry.childByFieldName("key").text()
		known := false
		for _, name := range allowed {
			if name == key {
				known = true
				break
			}
		}
		value := entry.childByFieldName("value")
		if !known || fields[key].valid() || !value.valid() {
			return nil, metricCompileFailure("PATTERN_INVALID_CONTEXT", "pattern", fmt.Sprintf("unknown or duplicate metric field %q", key))
		}
		fields[key] = value
	}
	return fields, nil
}

func metricSelector(doc *queryDocument, language string, node queryNode, options CompileOptions) (*Program, error) {
	if !node.valid() {
		return nil, metricCompileFailure("PATTERN_INVALID_CONTEXT", "pattern", "missing metric selector")
	}
	rng := node.byteRange()
	if rng.EndByte-rng.StartByte > options.MaxPatternBytes-32 {
		return nil, metricCompileFailure("LIMIT_PATTERN_BYTES", "resource", "metric selector exceeds byte limit")
	}
	query := append([]byte("language "+language+"\n"), doc.source[rng.StartByte:rng.EndByte]...)
	program, err := Compile(query, options)
	if err != nil {
		return nil, fmt.Errorf("metric selector: %w", err)
	}
	return program, nil
}

func metricString(node queryNode) (string, error) {
	if node.kind() != "stringConstant" {
		return "", metricCompileFailure("PATTERN_INVALID_CONTEXT", "pattern", "metric field must be a quoted string")
	}
	value, err := strconv.Unquote(node.text())
	if err != nil || strings.IndexByte(value, 0) >= 0 {
		return "", metricCompileFailure("PATTERN_INVALID_CONTEXT", "pattern", "invalid metric string")
	}
	return value, nil
}

func metricInt(node queryNode) (int, error) {
	if node.kind() != "intConstant" {
		return 0, metricCompileFailure("PATTERN_INVALID_CONTEXT", "pattern", "metric number must be a nonnegative integer")
	}
	value, err := strconv.Atoi(node.text())
	if err != nil {
		return 0, metricCompileFailure("PATTERN_INVALID_CONTEXT", "pattern", "metric integer overflow")
	}
	return value, nil
}

func metricBool(node queryNode) (bool, error) {
	if node.kind() != "booleanConstant" {
		return false, metricCompileFailure("PATTERN_INVALID_CONTEXT", "pattern", "metric flag must be true or false")
	}
	return node.text() == "true", nil
}

func metricStrings(node queryNode) ([]string, error) {
	if node.kind() != "list" {
		return nil, metricCompileFailure("PATTERN_INVALID_CONTEXT", "pattern", "metric operators must be a list")
	}
	var values []string
	for _, child := range withoutComments(node.namedChildren()) {
		value, err := metricString(child)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
		if len(values) > 32 {
			return nil, metricCompileFailure("PATTERN_INVALID_CONTEXT", "pattern", "too many operators")
		}
	}
	return values, nil
}

func compileMetricRule(doc *queryDocument, language string, node queryNode, options CompileOptions) (MetricRule, error) {
	values, err := metricMap(node, []string{"id", "query", "points", "depth", "opens", "flat", "logical", "operators", "child", "self"})
	if err != nil {
		return MetricRule{}, err
	}
	if !values["id"].valid() || !values["query"].valid() || !values["points"].valid() {
		return MetricRule{}, metricCompileFailure("PATTERN_INVALID_CONTEXT", "pattern", "metric rule requires id, query, and points")
	}
	rule := MetricRule{}
	if rule.ID, err = metricString(values["id"]); err != nil {
		return rule, err
	}
	if rule.Query, err = metricSelector(doc, language, values["query"], options); err != nil {
		return rule, err
	}
	if rule.Points, err = metricInt(values["points"]); err != nil {
		return rule, err
	}
	if values["depth"].valid() {
		if rule.NestingWeight, err = metricInt(values["depth"]); err != nil {
			return rule, err
		}
	}
	if values["opens"].valid() {
		if rule.OpensNesting, err = metricBool(values["opens"]); err != nil {
			return rule, err
		}
	}
	for _, item := range []struct {
		key    string
		target *string
	}{{"flat", &rule.FlatAlternativeField}, {"logical", &rule.Logical}, {"child", &rule.RequireChildKind}, {"self", &rule.SelfCallField}} {
		if values[item.key].valid() {
			if *item.target, err = metricString(values[item.key]); err != nil {
				return rule, err
			}
		}
	}
	if values["operators"].valid() {
		if rule.Operators, err = metricStrings(values["operators"]); err != nil {
			return rule, err
		}
	}
	return rule, nil
}
