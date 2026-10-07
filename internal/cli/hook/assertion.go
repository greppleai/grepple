// Package hook executes repository-owned, read-only structural validation.
package hook

import (
	"fmt"
	"path"
	"strings"

	"github.com/greppleai/grepple/internal/gritql"
)

const maxAssertionText = 32 << 10

// assertionConfig compares bounded string expressions for each structural match.
type assertionConfig struct {
	Equals []stringExpression `yaml:"equals"`
}

// stringExpression is a closed, language-independent expression tree.
type stringExpression struct {
	Literal  *string            `yaml:"literal"`
	Binding  *string            `yaml:"binding"`
	Path     *bool              `yaml:"path"`
	Concat   []stringExpression `yaml:"concat"`
	Lower    *stringExpression  `yaml:"lower"`
	Upper    *stringExpression  `yaml:"upper"`
	Trim     *stringExpression  `yaml:"trim"`
	Basename *stringExpression  `yaml:"basename"`
	Dirname  *stringExpression  `yaml:"dirname"`
	Replace  *stringReplace     `yaml:"replace"`
	Segment  *stringSegment     `yaml:"segment"`
}

type stringReplace struct {
	Input stringExpression `yaml:"input"`
	Old   string           `yaml:"old"`
	New   string           `yaml:"new"`
}
type stringSegment struct {
	Input     stringExpression `yaml:"input"`
	Separator string           `yaml:"separator"`
	Index     int              `yaml:"index"`
}

func unaryStrings(e stringExpression) map[string]*stringExpression {
	return map[string]*stringExpression{"lower": e.Lower, "upper": e.Upper, "trim": e.Trim, "basename": e.Basename, "dirname": e.Dirname}
}

func assertionChildren(e stringExpression) ([]stringExpression, int) {
	children := []stringExpression{}
	count := 0
	for _, present := range []bool{e.Literal != nil, e.Binding != nil, e.Path != nil} {
		if present {
			count++
		}
	}
	if e.Concat != nil {
		count++
		children = append(children, e.Concat...)
	}
	for _, child := range unaryStrings(e) {
		if child != nil {
			count++
			children = append(children, *child)
		}
	}
	if e.Replace != nil {
		count++
		children = append(children, e.Replace.Input)
	}
	if e.Segment != nil {
		count++
		children = append(children, e.Segment.Input)
	}
	return children, count
}

func validateAssertionConfig(r rule) error {
	if r.Assert == nil {
		return nil
	}
	if r.Engine != "gritql-v1" || r.Annotation != nil {
		return fmt.Errorf("assert requires a file-local gritql-v1 rule without annotation")
	}
	if len(r.Assert.Equals) != 2 {
		return fmt.Errorf("assert.equals requires exactly two expressions")
	}
	return nil
}

func validateAssertionProgram(r rule, program *gritql.Program) error {
	if r.Assert == nil {
		return nil
	}
	bindings := map[string]bool{}
	for _, v := range program.Variables() {
		bindings[strings.TrimPrefix(v.Name, "$")] = true
	}
	nodes := 0
	for _, e := range r.Assert.Equals {
		if err := validateStringExpression(e, bindings, 0, &nodes); err != nil {
			return err
		}
	}
	message := strings.NewReplacer("{{actual}}", "", "{{expected}}", "", "{{path}}", "").Replace(r.Message)
	if strings.Contains(message, "{{") || strings.Contains(message, "}}") {
		return fmt.Errorf("assert message supports only {{actual}}, {{expected}} and {{path}}")
	}
	return nil
}

func validateStringExpression(e stringExpression, bindings map[string]bool, depth int, nodes *int) error {
	*nodes++
	if depth > 8 || *nodes > 64 {
		return fmt.Errorf("assert expression exceeds depth/node limit")
	}
	children, count := assertionChildren(e)
	if count != 1 {
		return fmt.Errorf("each assert expression requires exactly one operator")
	}
	if err := validateStringOperator(e, bindings); err != nil {
		return err
	}
	for _, child := range children {
		if err := validateStringExpression(child, bindings, depth+1, nodes); err != nil {
			return err
		}
	}
	return nil
}

func validateStringOperator(e stringExpression, bindings map[string]bool) error {
	if e.Binding != nil && !bindings[*e.Binding] {
		return fmt.Errorf("assert binding %q is not declared in query", *e.Binding)
	}
	if e.Path != nil && !*e.Path {
		return fmt.Errorf("assert path must be true")
	}
	if e.Concat != nil && len(e.Concat) == 0 {
		return fmt.Errorf("assert concat must not be empty")
	}
	if e.Replace != nil && (e.Replace.Old == "" || len(e.Replace.Old) > maxAssertionText || len(e.Replace.New) > maxAssertionText) {
		return fmt.Errorf("assert replace requires a nonempty, bounded old string and bounded new string")
	}
	if e.Segment != nil && (e.Segment.Separator == "" || len(e.Segment.Separator) > 128) {
		return fmt.Errorf("assert segment requires a short, nonempty separator")
	}
	if e.Literal != nil && len(*e.Literal) > maxAssertionText {
		return fmt.Errorf("assert literal exceeds text limit")
	}
	return nil
}

func assertionBinding(match gritql.Finding, name string) (string, error) {
	for _, b := range match.Bindings() {
		if b.Name() != name {
			continue
		}
		start, end := b.Range().StartByte-match.StartByte(), b.Range().EndByte-match.StartByte()
		if start < 0 || end < start || end > len(match.Text()) {
			return "", fmt.Errorf("assert binding %q is outside the matched source range", name)
		}
		return match.Text()[start:end], nil
	}
	return "", fmt.Errorf("assert binding %q is absent in this match", name)
}

func evaluateString(e stringExpression, match gritql.Finding) (string, error) {
	value, err := evaluateStringOperator(e, match)
	if err != nil {
		return "", err
	}
	if len(value) > maxAssertionText {
		return "", fmt.Errorf("assert result exceeds text limit")
	}
	return value, nil
}

func evaluateStringOperator(e stringExpression, match gritql.Finding) (string, error) {
	switch {
	case e.Literal != nil:
		return *e.Literal, nil
	case e.Binding != nil:
		return assertionBinding(match, *e.Binding)
	case e.Path != nil:
		return match.Path(), nil
	case e.Concat != nil:
		return concatenateStrings(e.Concat, match)
	case e.Replace != nil:
		return replaceString(*e.Replace, match)
	case e.Segment != nil:
		return segmentString(*e.Segment, match)
	default:
		return unaryString(e, match)
	}
}

func concatenateStrings(expressions []stringExpression, match gritql.Finding) (string, error) {
	var out strings.Builder
	for _, e := range expressions {
		value, err := evaluateString(e, match)
		if err != nil {
			return "", err
		}
		if out.Len()+len(value) > maxAssertionText {
			return "", fmt.Errorf("assert concat exceeds text limit")
		}
		out.WriteString(value)
	}
	return out.String(), nil
}

func replaceString(e stringReplace, match gritql.Finding) (string, error) {
	value, err := evaluateString(e.Input, match)
	if err != nil {
		return "", err
	}
	size := int64(len(value)) + int64(strings.Count(value, e.Old))*int64(len(e.New)-len(e.Old))
	if size > maxAssertionText {
		return "", fmt.Errorf("assert replace exceeds text limit")
	}
	return strings.ReplaceAll(value, e.Old, e.New), nil
}

func segmentString(e stringSegment, match gritql.Finding) (string, error) {
	value, err := evaluateString(e.Input, match)
	if err != nil {
		return "", err
	}
	parts := strings.Split(value, e.Separator)
	index := e.Index
	if index < 0 {
		index = len(parts) + index
	}
	if index < 0 || index >= len(parts) {
		return "", fmt.Errorf("assert segment index %d out of range for %q", e.Index, value)
	}
	return parts[index], nil
}

func unaryString(e stringExpression, match gritql.Finding) (string, error) {
	for op, child := range unaryStrings(e) {
		if child == nil {
			continue
		}
		value, err := evaluateString(*child, match)
		if err != nil {
			return "", err
		}
		switch op {
		case "lower":
			return strings.ToLower(value), nil
		case "upper":
			return strings.ToUpper(value), nil
		case "trim":
			return strings.TrimSpace(value), nil
		case "basename":
			return path.Base(value), nil
		case "dirname":
			return path.Dir(value), nil
		}
	}
	return "", fmt.Errorf("assert expression has no operator")
}

func assertedFinding(config rule, match gritql.Finding) (*Finding, error) {
	message := config.Message
	if config.Assert != nil {
		actual, err := evaluateString(config.Assert.Equals[0], match)
		if err != nil {
			return nil, err
		}
		expected, err := evaluateString(config.Assert.Equals[1], match)
		if err != nil {
			return nil, err
		}
		if actual == expected {
			return nil, nil
		}
		message, err = assertionMessage(message, actual, expected, match.Path())
		if err != nil {
			return nil, err
		}
	}
	start := match.Start()
	return &Finding{ID: config.ID, Path: match.Path(), Line: start.Line, Column: start.Column, Severity: config.Severity, Message: message}, nil
}

func assertionMessage(template, actual, expected, sourcePath string) (string, error) {
	replacements := []string{"{{actual}}", actual, "{{expected}}", expected, "{{path}}", sourcePath}
	size := int64(len(template))
	for i := 0; i < len(replacements); i += 2 {
		size += int64(strings.Count(template, replacements[i])) * int64(len(replacements[i+1])-len(replacements[i]))
	}
	if size > maxAssertionText {
		return "", fmt.Errorf("assert message exceeds text limit")
	}
	return strings.NewReplacer(replacements...).Replace(template), nil
}

func appendStructuralMatches(found []Finding, config rule, matches []gritql.Finding, assertionBytes *int64) ([]Finding, error) {
	for _, match := range matches {
		finding, err := assertedFinding(config, match)
		if err != nil {
			return nil, fmt.Errorf("hook %s: %s: %w", config.ID, match.Path(), err)
		}
		if finding == nil {
			continue
		}
		if config.Assert != nil {
			*assertionBytes += int64(len(finding.Message) + len(finding.Path) + 256)
			if *assertionBytes > maxParallelResultBytes {
				return nil, fmt.Errorf("assert findings exceed shared result byte limit")
			}
		}
		found = append(found, *finding)
	}
	return found, nil
}
