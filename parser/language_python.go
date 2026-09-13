package parser

import (
	sitter "github.com/tree-sitter/go-tree-sitter"
	python "github.com/tree-sitter/tree-sitter-python/bindings/go"
)

type pythonLanguage struct {
	grammar *sitter.Language
	rules   structureRules
}

func newPythonLanguage() languageAdapter {
	return &pythonLanguage{
		grammar: sitter.NewLanguage(python.Language()),
		rules: structureRules{
			structuralTypes:       newStringSet("import_statement", "import_from_statement", "function_definition", "class_definition", "decorated_definition"),
			contextTypes:          newStringSet("function_definition", "class_definition", "decorated_definition"),
			containerTypes:        newStringSet("class_definition"),
			classDeclarationTypes: newStringSet("class_definition"),
			classBodyTypes:        newStringSet("block"),
			exportTypes:           newStringSet(),
			functionLikeTypes:     newStringSet("function_definition", "lambda"),
			blockTypes:            newStringSet("block"),
			jsxElementTypes:       newStringSet(),
			nameFieldCandidates:   newStringSet("identifier"),
		},
	}
}

func (*pythonLanguage) ID() string                         { return "python" }
func (language *pythonLanguage) Grammar() *sitter.Language { return language.grammar }
func (language *pythonLanguage) Rules() *structureRules    { return &language.rules }
func (language *pythonLanguage) Navigation() navigationAdapter {
	return pythonNavigationAdapter(&language.rules)
}
func (language *pythonLanguage) Outline(root *sitter.Node, content string) []Symbol {
	return pythonDeclarations(namedChildren(root), content, &language.rules)
}

func pythonDeclarations(nodes []*sitter.Node, content string, rules *structureRules) []Symbol {
	var symbols []Symbol
	for _, node := range nodes {
		if node.Kind() == "decorated_definition" {
			symbols = append(symbols, pythonDeclarations(namedChildren(node), content, rules)...)
			continue
		}
		switch node.Kind() {
		case "function_definition":
			symbols = append(symbols, symbolFrom("function", extractNodeName(node, content, rules), node, content))
		case "class_definition":
			symbol := symbolFrom("class", extractNodeName(node, content, rules), node, content)
			if body := node.ChildByFieldName("body"); body != nil {
				symbol.Children = pythonDeclarations(namedChildren(body), content, rules)
			}
			symbols = append(symbols, symbol)
		}
	}
	return symbols
}
