package gritql

import (
	"fmt"
	"strings"

	"github.com/greppleai/grepple/internal/parser"
)

var pythonSnippetAttempts = []snippetAttempt{
	{SnippetContextExpression, "", "\n", selectPythonExpression},
	{SnippetContextStatement, "", "\n", selectOnePythonStatement},
	{SnippetContextStatementList, "", "\n", selectPythonStatementList},
	{SnippetContextDeclaration, "", "\n", selectOnePythonDeclaration},
	{SnippetContextDeclarationList, "", "\n", selectPythonDeclarationList},
	{SnippetContextFile, "", "", selectPythonFile},
}

func compilePythonTemplates(decoded decodedSnippet, maxDepth int) ([]Template, string, error) {
	roles := pythonPlaceholderRoles(decoded)
	assignments := [][]placeholderRole{roles}
	if wholeSnippetPlaceholder(decoded) {
		statementRoles := append([]placeholderRole(nil), roles...)
		statementRoles[0] = rolePythonStatement
		declarationRoles := append([]placeholderRole(nil), roles...)
		declarationRoles[0] = rolePythonDeclaration
		assignments = append(assignments, statementRoles, declarationRoles)
	}
	var templates []Template
	for _, attempt := range pythonSnippetAttempts {
		candidates, tooDeep := parseInferredTemplates("python", decoded, attempt, assignments, maxDepth)
		if tooDeep {
			return nil, "LIMIT_PARSE_DEPTH", fmt.Errorf("python template exceeds effective depth limit")
		}
		templates = append(templates, candidates...)
	}
	templates = dedupeTemplates(templates)
	if len(templates) == 0 {
		return nil, "PATTERN_INVALID_SNIPPET", fmt.Errorf("snippet is not valid python in any supported context")
	}
	return templates, "", nil
}
func pythonPlaceholderRoles(decoded decodedSnippet) []placeholderRole {
	roles := make([]placeholderRole, len(decoded.placeholders))
	for index, placeholder := range decoded.placeholders {
		before := strings.TrimSpace(decoded.text[:placeholder.start])
		if hasTrailingWord(before, "from") || hasTrailingWord(before, "import") {
			roles[index] = rolePythonDottedName
		}
	}
	return roles
}

func wholeSnippetPlaceholder(decoded decodedSnippet) bool {
	if len(decoded.placeholders) != 1 {
		return false
	}
	placeholder := decoded.placeholders[0]
	return strings.TrimSpace(decoded.text) == decoded.text[placeholder.start:placeholder.end]
}

func selectPythonExpression(root parser.Node, start, end int) (selectedRoot, bool) {
	children := directNamed(root)
	if len(children) != 1 || children[0].Kind() != "expression_statement" {
		return selectedRoot{}, false
	}
	expressions := directNamed(children[0])
	if len(expressions) != 1 || !rangeWithin(expressions[0], start, end) {
		return selectedRoot{}, false
	}
	return selectedRoot{node: expressions[0]}, true
}

func selectOnePythonStatement(root parser.Node, start, end int) (selectedRoot, bool) {
	children := directNamed(root)
	if len(children) != 1 || !isPythonStatement(children[0].Kind()) || !rangeWithin(children[0], start, end) {
		return selectedRoot{}, false
	}
	return selectedRoot{node: children[0]}, true
}

func selectPythonStatementList(root parser.Node, start, end int) (selectedRoot, bool) {
	children := directNamed(root)
	if len(children) < 2 || !allPythonStatements(children) || !rangeWithin(children[0], start, end) || !rangeWithin(children[len(children)-1], start, end) {
		return selectedRoot{}, false
	}
	return selectedRoot{node: root, sequenceKind: "statement_sequence"}, true
}

func selectOnePythonDeclaration(root parser.Node, start, end int) (selectedRoot, bool) {
	children := directNamed(root)
	if len(children) != 1 || !isPythonDeclaration(children[0].Kind()) || !rangeWithin(children[0], start, end) {
		return selectedRoot{}, false
	}
	return selectedRoot{node: children[0]}, true
}

func selectPythonDeclarationList(root parser.Node, start, end int) (selectedRoot, bool) {
	children := directNamed(root)
	if len(children) < 2 || !allPythonDeclarations(children) || !rangeWithin(children[0], start, end) || !rangeWithin(children[len(children)-1], start, end) {
		return selectedRoot{}, false
	}
	return selectedRoot{node: root, sequenceKind: "statement_sequence"}, true
}

func selectPythonFile(root parser.Node, start, end int) (selectedRoot, bool) {
	return selectedRoot{node: root}, start == 0 && root.Valid() && root.Kind() == "module" && rangeWithin(root, start, end)
}

func isPythonStatement(kind string) bool {
	return parser.GrammarSubtype("python", "_simple_statement", kind) || parser.GrammarSubtype("python", "_compound_statement", kind)
}

func allPythonStatements(nodes []parser.Node) bool {
	for _, node := range nodes {
		if !isPythonStatement(node.Kind()) {
			return false
		}
	}
	return true
}

func isPythonDeclaration(kind string) bool {
	switch kind {
	case "class_definition", "decorated_definition", "function_definition", "future_import_statement", "import_from_statement", "import_statement":
		return true
	default:
		return false
	}
}

func allPythonDeclarations(nodes []parser.Node) bool {
	for _, node := range nodes {
		if !isPythonDeclaration(node.Kind()) {
			return false
		}
	}
	return true
}

func pythonRootCategoryAccepts(context SnippetContext, kind string) bool {
	switch context {
	case SnippetContextExpression:
		return parser.GrammarSubtype("python", "expression", kind) || parser.GrammarSubtype("python", "primary_expression", kind) || parser.GrammarSubtype("python", "pattern", kind)
	case SnippetContextStatement, SnippetContextStatementList:
		return isPythonStatement(kind)
	case SnippetContextDeclaration, SnippetContextDeclarationList:
		return isPythonDeclaration(kind)
	case SnippetContextFile:
		return kind == "module"
	default:
		return false
	}
}
