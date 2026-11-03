package parser

import lua "github.com/tree-sitter-grammars/tree-sitter-lua/bindings/go"

type luaLanguage struct {
	grammar syntaxLanguage
	rules   structureRules
}

func newLuaLanguage() languageAdapter {
	return &luaLanguage{
		grammar: newSyntaxLanguage(lua.Language()),
		rules: structureRules{
			structuralTypes:     newStringSet("function_declaration", "variable_declaration", "assignment_statement"),
			contextTypes:        newStringSet("function_declaration", "function_definition", "if_statement", "for_statement", "while_statement", "repeat_statement", "do_statement"),
			containerTypes:      newStringSet("function_declaration"),
			classBodyTypes:      newStringSet("block"),
			functionLikeTypes:   newStringSet("function_declaration", "function_definition"),
			nameFieldCandidates: newStringSet("identifier"),
		},
	}
}

func (*luaLanguage) ID() string                       { return "lua" }
func (language *luaLanguage) Grammar() syntaxLanguage { return language.grammar }
func (language *luaLanguage) Parse(content string) (*syntaxTree, error) {
	return parseSyntaxTree(language.grammar, content)
}
func (language *luaLanguage) Rules() *structureRules { return &language.rules }
func (language *luaLanguage) Navigation() navigationAdapter {
	return luaNavigationAdapter(&language.rules)
}
func (language *luaLanguage) Outline(root *syntaxNode, content string) []Symbol {
	var symbols []Symbol
	if root == nil {
		return symbols
	}
	for _, child := range root.NamedChildren() {
		if child.Kind() == "function_declaration" {
			symbols = append(symbols, symbolFrom("function", extractNodeName(child, content, &language.rules), child, content))
		} else if child.Kind() == "variable_declaration" {
			for _, declaration := range child.NamedChildren() {
				if declaration.Kind() == "function_declaration" {
					symbols = append(symbols, symbolFrom("function", extractNodeName(declaration, content, &language.rules), declaration, content))
				}
			}
		}
	}
	return symbols
}

func (*luaLanguage) ScopeKind(nodeType string) string {
	switch nodeType {
	case "function_declaration", "function_definition":
		return "func"
	case "if_statement":
		return "if"
	case "while_statement", "repeat_statement":
		return "while"
	case "for_statement":
		return "for"
	}
	return ""
}
func (language *luaLanguage) ScopeClassification(node ViewNode) ScopeClassification {
	return standardScopeClassification(language.ScopeKind(node.Kind()))
}
func (*luaLanguage) ScopeSelectorKind(role string) string {
	return functionSelectorKind(role, "function", false)
}
