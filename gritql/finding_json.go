package gritql

import (
	"encoding/json"
	"strings"

	"github.com/greppleai/grepple/parser"
)

type rangeJSON struct {
	StartByte   int `json:"start_byte"`
	EndByte     int `json:"end_byte"`
	StartLine   int `json:"start_line"`
	StartColumn int `json:"start_column"`
	EndLine     int `json:"end_line"`
	EndColumn   int `json:"end_column"`
}

func rangeJSONValue(r parser.Range) rangeJSON {
	return rangeJSON{r.StartByte, r.EndByte, r.Start.Line, r.Start.Column, r.End.Line, r.End.Column}
}

type findingJSON struct {
	Path string `json:"path"`
	rangeJSON
	PatternID string                     `json:"pattern_id"`
	Message   string                     `json:"message"`
	Bindings  map[string]json.RawMessage `json:"bindings"`
}

func findingJSONValue(f Finding) findingJSON {
	bindings := make(map[string]json.RawMessage, len(f.bindings))
	for _, binding := range f.bindings {
		bindings[binding.name] = marshalBinding(binding)
	}
	return findingJSON{Path: f.path, rangeJSON: rangeJSONValue(f.rng), PatternID: f.patternID, Message: f.message, Bindings: bindings}
}

func marshalBindings(bindings []FindingBinding) []byte {
	values := make(map[string]json.RawMessage, len(bindings))
	for _, binding := range bindings {
		values[binding.name] = marshalBinding(binding)
	}
	data, _ := json.Marshal(values)
	return data
}

func marshalBinding(binding FindingBinding) []byte {
	structures := binding.structural
	if binding.kind == BindingNode {
		var structural StructuralNode
		if len(structures) != 0 {
			structural = structures[0]
		}
		value := struct {
			Kind       string         `json:"kind"`
			Range      rangeJSON      `json:"range"`
			Structural StructuralNode `json:"structural"`
		}{"node", rangeJSONValue(binding.rng), structural}
		data, _ := json.Marshal(value)
		return data
	}
	ranges := make([]rangeJSON, len(binding.ranges))
	for i := range binding.ranges {
		ranges[i] = rangeJSONValue(binding.ranges[i])
	}
	value := struct {
		Kind       string           `json:"kind"`
		Range      rangeJSON        `json:"range"`
		Ranges     []rangeJSON      `json:"ranges"`
		Structural []StructuralNode `json:"structural"`
	}{"list", rangeJSONValue(binding.rng), ranges, structures}
	data, _ := json.Marshal(value)
	return data
}

func marshalNormalizedNode(n normalizedNode) ([]byte, error) {
	if !n.named {
		return json.Marshal(struct {
			TokenKind string `json:"token_kind"`
			Lexeme    string `json:"lexeme"`
		}{n.kind, n.lexeme})
	}
	children := n.children
	if len(children) == 0 {
		children = []normalizedNode{{kind: normalizedLeafTokenKind(n.kind), lexeme: n.lexeme}}
	}
	return json.Marshal(struct {
		NodeKind string           `json:"node_kind"`
		Children []normalizedNode `json:"children"`
	}{n.kind, children})
}

func normalizedLeafTokenKind(kind string) string {
	if kind == "identifier" || strings.HasSuffix(kind, "_identifier") {
		return "IDENT"
	}
	return strings.ToUpper(kind)
}

type diagnosticJSON struct {
	Code      string     `json:"code"`
	Class     string     `json:"class"`
	Severity  string     `json:"severity"`
	Message   string     `json:"message"`
	PatternID *string    `json:"pattern_id"`
	Path      *string    `json:"path"`
	Range     *rangeJSON `json:"range"`
}
