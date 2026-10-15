package parser

import (
	"strings"
	"testing"
)

// This is the explicit vocabulary supported by the adapters, independent of
// rendered selector names and normalized line-annotation role selection.
func TestAdapterScopeKindVocabulary(t *testing.T) {
	common := map[string]string{
		"if": "if_statement", "while": "while_statement", "for": "for_statement",
		"switch": "switch_statement", "try": "try_statement", "catch": "catch_clause",
	}
	class := map[string]string{
		"class": "class_declaration", "interface": "interface_declaration", "enum": "enum_declaration",
		"method": "method_declaration constructor_declaration",
	}
	specs := map[string]map[string]string{
		"go":         {"case": "expression_case type_case default_case", "for": "range_clause", "func": "function_declaration func_literal", "method": "method_declaration", "struct": "struct_type", "interface": "interface_type"},
		"java":       {"class": "record_declaration"},
		"csharp":     {"struct": "struct_declaration record_declaration", "func": "local_function_statement lambda_expression"},
		"dart":       {"func": "function_declaration function_expression", "method": "getter_declaration setter_declaration", "class": "mixin_declaration extension_declaration extension_type_declaration"},
		"kotlin":     {"func": "function_declaration anonymous_function lambda_literal", "class": "class_declaration object_declaration", "if": "if_expression", "while": "while_statement", "for": "for_statement"},
		"swift":      {"func": "function_declaration protocol_function_declaration", "method": "init_declaration", "class": "class_declaration", "interface": "protocol_declaration"},
		"hcl":        {"block": "block"},
		"svelte":     {"if": "if_statement else_if_block", "for": "each_statement", "snippet": "snippet_statement"},
		"css":        {"rule": "rule_set media_statement supports_statement scope_statement keyframes_statement keyframe_block", "property": "declaration"},
		"javascript": {"for": "for_in_statement", "case": "case_statement"},
		"typescript": {"for": "for_in_statement", "case": "case_statement", "interface": "interface_declaration", "enum": "enum_declaration"},
		"tsx":        {"for": "for_in_statement", "case": "case_statement", "interface": "interface_declaration", "enum": "enum_declaration"},
		"python":     {"func": "function_definition lambda", "class": "class_definition", "for": "for_in_statement"},
		"c":          {"for": "for_in_statement", "case": "case_statement", "func": "function_definition lambda_expression", "struct": "struct_specifier union_specifier", "enum": "enum_specifier"},
		"cpp":        {"for": "for_in_statement", "case": "case_statement", "func": "function_definition lambda_expression", "struct": "struct_specifier union_specifier", "enum": "enum_specifier", "class": "class_specifier"},
		"rust":       {"func": "function_item closure_expression", "struct": "struct_item", "interface": "trait_item", "enum": "enum_item", "if": "if_expression if_let_expression", "while": "while_expression while_let_expression", "for": "for_expression", "case": "match_arm"},
		"php":        {"for": "for_of_statement foreach_statement", "case": "switch_case", "func": "function_definition anonymous_function_creation_expression arrow_function", "class": "trait_declaration"},
		"shell":      {"func": "function_definition", "case": "case_statement"},
	}
	// Expand independently specified common grammars and family differences.
	for _, id := range []string{"go", "java", "csharp", "dart", "swift", "javascript", "typescript", "tsx", "c", "cpp", "php"} {
		for role, kinds := range common {
			specs[id][role] += " " + kinds
		}
	}
	// Python and Shell have only the four common loop/branch forms.
	for _, id := range []string{"python", "shell"} {
		for role, kinds := range map[string]string{"if": "if_statement", "while": "while_statement", "for": "for_statement", "switch": "switch_statement"} {
			specs[id][role] += " " + kinds
		}
	}
	// Record differs by language: Java adds it as a class, C# as a struct.
	for _, id := range []string{"java", "csharp", "dart", "php"} {
		for role, kinds := range class {
			specs[id][role] += " " + kinds
		}
	}
	for _, id := range []string{"javascript", "typescript", "tsx"} {
		for role, kinds := range map[string]string{
			"for": "for_of_statement", "case": "switch_case", "func": "function_declaration generator_function_declaration arrow_function function",
			"method": "method_definition", "class": "class_declaration class",
		} {
			specs[id][role] += " " + kinds
		}
	}
	for id, adapter := range languageAdapters {
		roles, ok := specs[id]
		if !ok {
			t.Fatalf("registered adapter %q lacks scope mapping fixture", id)
		}
		delete(specs, id)
		t.Run(id, func(t *testing.T) {
			known := map[string]string{}
			for role, nodes := range roles {
				for _, node := range strings.Fields(nodes) {
					if previous, exists := known[node]; exists {
						t.Fatalf("duplicate node %q: %s and %s", node, previous, role)
					}
					known[node] = role
					if got := adapter.ScopeKind(node); got != role {
						t.Errorf("%s: got %q, want %q", node, got, role)
					}
				}
			}
			for _, node := range []string{"", "ERROR", "comment", "identifier", "unknown_node", "type_alias_declaration", "record_declaration", "trait_item", "for_of_statement"} {
				if _, supported := known[node]; supported {
					continue
				}
				if got := adapter.ScopeKind(node); got != "" {
					t.Errorf("unsupported %s: got %q", node, got)
				}
			}
		})
	}
	if len(specs) != 0 {
		t.Fatalf("unregistered fixture adapters: %v", specs)
	}
	if got := contentAdapters["markdown"].ScopeKind("atx_heading"); got != "" {
		t.Fatalf("markdown code kind: %q", got)
	}
}
