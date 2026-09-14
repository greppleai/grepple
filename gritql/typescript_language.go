package gritql

import (
	"fmt"
	"strings"

	"github.com/greppleai/grepple/parser"
)

var typeScriptSnippetAttempts = []snippetAttempt{
	{SnippetContextExpression, "const __grit_value = ", ";\n", selectTypeScriptExpression},
	{SnippetContextType, "type __grit_type = ", ";\n", selectTypeScriptType},
	{SnippetContextStatement, "function __grit_func(){\n", "\n}\n", selectOneTypeScriptStatement},
	{SnippetContextStatementList, "function __grit_func(){\n", "\n}\n", selectTypeScriptStatementList},
	{SnippetContextDeclaration, "class __grit_class {\n", "\n}\n", selectOneTypeScriptMember},
	{SnippetContextDeclarationList, "class __grit_class {\n", "\n}\n", selectTypeScriptMemberList},
	{SnippetContextDeclaration, "", "\n", selectOneTypeScriptDeclaration},
	{SnippetContextDeclarationList, "", "\n", selectTypeScriptDeclarationList},
	{SnippetContextFile, "", "", selectTypeScriptFile},
}

func compileJavaScriptTemplates(decoded decodedSnippet, maxDepth int) ([]Template, string, error) {
	return compileTypeScriptFamilyTemplates("javascript", decoded, maxDepth)
}

func compileTypeScriptTemplates(decoded decodedSnippet, maxDepth int) ([]Template, string, error) {
	return compileTypeScriptFamilyTemplates("typescript", decoded, maxDepth)
}

func compileTSXTemplates(decoded decodedSnippet, maxDepth int) ([]Template, string, error) {
	return compileTypeScriptFamilyTemplates("tsx", decoded, maxDepth)
}

func compileTypeScriptFamilyTemplates(language string, decoded decodedSnippet, maxDepth int) ([]Template, string, error) {
	roles := typeScriptPlaceholderRoles(decoded)
	assignments := [][]placeholderRole{roles}
	if typeScriptWholePlaceholder(decoded) {
		declarationRoles := append([]placeholderRole(nil), roles...)
		declarationRoles[0] = roleTypeScriptDeclaration
		assignments = append(assignments, declarationRoles)
	}
	var templates []Template
	for _, attempt := range typeScriptSnippetAttempts {
		candidates, tooDeep := parseInferredTemplates(language, decoded, attempt, assignments, maxDepth)
		if tooDeep {
			return nil, "LIMIT_PARSE_DEPTH", fmt.Errorf("%s template exceeds effective depth limit", language)
		}
		templates = append(templates, candidates...)
	}
	templates = dedupeTemplates(templates)
	if len(templates) == 0 {
		return nil, "PATTERN_INVALID_SNIPPET", fmt.Errorf("snippet is not valid %s in any supported context", language)
	}
	return templates, "", nil
}
func typeScriptPlaceholderRoles(decoded decodedSnippet) []placeholderRole {
	roles := make([]placeholderRole, len(decoded.placeholders))
	for index, placeholder := range decoded.placeholders {
		before := strings.TrimSpace(decoded.text[:placeholder.start])
		after := strings.TrimSpace(decoded.text[placeholder.end:])
		if hasTrailingWord(before, "from") || before == "import" && (after == "" || after == ";") {
			roles[index] = roleImportPath
		}
	}
	return roles
}

func hasTrailingWord(source, word string) bool {
	if !strings.HasSuffix(source, word) {
		return false
	}
	start := len(source) - len(word)
	if start == 0 {
		return true
	}
	previous := source[start-1]
	return !(previous == '_' || previous >= '0' && previous <= '9' || previous >= 'A' && previous <= 'Z' || previous >= 'a' && previous <= 'z')
}

func typeScriptWholePlaceholder(decoded decodedSnippet) bool {
	if len(decoded.placeholders) != 1 {
		return false
	}
	placeholder := decoded.placeholders[0]
	return strings.TrimSpace(decoded.text) == decoded.text[placeholder.start:placeholder.end]
}

func selectTypeScriptExpression(root parser.Node, start, end int) (selectedRoot, bool) {
	children := directNamed(root)
	if len(children) != 1 || children[0].Kind() != "lexical_declaration" {
		return selectedRoot{}, false
	}
	declarator := childKind(children[0], "variable_declarator")
	value := declarator.ChildByFieldName("value")
	return selectedRoot{node: value}, rangeWithin(value, start, end)
}

func selectTypeScriptType(root parser.Node, start, end int) (selectedRoot, bool) {
	children := directNamed(root)
	if len(children) != 1 || children[0].Kind() != "type_alias_declaration" {
		return selectedRoot{}, false
	}
	value := children[0].ChildByFieldName("value")
	return selectedRoot{node: value}, rangeWithin(value, start, end)
}

func typeScriptStatementBlock(root parser.Node) (parser.Node, bool) {
	children := directNamed(root)
	if len(children) != 1 || children[0].Kind() != "function_declaration" {
		return parser.Node{}, false
	}
	body := children[0].ChildByFieldName("body")
	return body, body.Valid() && body.Kind() == "statement_block"
}

func selectOneTypeScriptStatement(root parser.Node, start, end int) (selectedRoot, bool) {
	block, ok := typeScriptStatementBlock(root)
	if !ok {
		return selectedRoot{}, false
	}
	children := directNamed(block)
	if len(children) != 1 || !rangeWithin(children[0], start, end) {
		return selectedRoot{}, false
	}
	if hasExplicitSemicolon(block, start, end) {
		return selectedRoot{node: block, sequenceKind: "statement_sequence"}, true
	}
	return selectedRoot{node: children[0]}, true
}

func selectTypeScriptStatementList(root parser.Node, start, end int) (selectedRoot, bool) {
	block, ok := typeScriptStatementBlock(root)
	children := directNamed(block)
	if !ok || len(children) < 2 || !rangeWithin(children[0], start, end) || !rangeWithin(children[len(children)-1], start, end) {
		return selectedRoot{}, false
	}
	return selectedRoot{node: block, sequenceKind: "statement_sequence"}, true
}

func selectOneTypeScriptDeclaration(root parser.Node, start, end int) (selectedRoot, bool) {
	children := directNamed(root)
	if len(children) != 1 || !isTypeScriptDeclaration(children[0].Kind()) || !rangeWithin(children[0], start, end) {
		return selectedRoot{}, false
	}
	return selectedRoot{node: children[0]}, true
}

func typeScriptClassBody(root parser.Node) (parser.Node, bool) {
	children := directNamed(root)
	if len(children) != 1 || children[0].Kind() != "class_declaration" {
		return parser.Node{}, false
	}
	body := children[0].ChildByFieldName("body")
	return body, body.Valid() && body.Kind() == "class_body"
}

func selectOneTypeScriptMember(root parser.Node, start, end int) (selectedRoot, bool) {
	body, ok := typeScriptClassBody(root)
	if !ok {
		return selectedRoot{}, false
	}

	children := directNamed(body)
	if len(children) != 1 || !rangeWithin(children[0], start, end) {
		return selectedRoot{}, false
	}
	if hasExplicitSemicolon(body, start, end) {
		return selectedRoot{node: body, sequenceKind: "list_sequence"}, true
	}
	return selectedRoot{node: children[0]}, true
}

func selectTypeScriptMemberList(root parser.Node, start, end int) (selectedRoot, bool) {
	body, ok := typeScriptClassBody(root)
	children := directNamed(body)
	if !ok || len(children) < 2 || !rangeWithin(children[0], start, end) || !rangeWithin(children[len(children)-1], start, end) {
		return selectedRoot{}, false
	}
	return selectedRoot{node: body, sequenceKind: "list_sequence"}, true
}

func selectTypeScriptDeclarationList(root parser.Node, start, end int) (selectedRoot, bool) {
	children := directNamed(root)
	if len(children) < 2 || !allTypeScriptDeclarations(children) || !rangeWithin(children[0], start, end) || !rangeWithin(children[len(children)-1], start, end) {
		return selectedRoot{}, false
	}
	return selectedRoot{node: root, sequenceKind: "declaration_sequence"}, true
}

func selectTypeScriptFile(root parser.Node, start, end int) (selectedRoot, bool) {
	return selectedRoot{node: root}, start == 0 && root.Valid() && root.Kind() == "program" && rangeWithin(root, start, end)
}

func isTypeScriptDeclaration(kind string) bool {
	switch kind {
	case "ambient_declaration", "class_declaration", "enum_declaration", "export_statement", "function_declaration", "generator_function_declaration", "import_alias", "import_statement", "interface_declaration", "internal_module", "lexical_declaration", "module", "type_alias_declaration", "variable_declaration":
		return true
	default:
		return false
	}
}

func isTypeScriptMember(kind string) bool {
	switch kind {
	case "abstract_method_signature", "abstract_property_signature", "field_definition", "method_definition", "method_signature", "public_field_definition", "property_signature":
		return true
	default:
		return false
	}
}

func allTypeScriptDeclarations(nodes []parser.Node) bool {
	for _, node := range nodes {
		if !isTypeScriptDeclaration(node.Kind()) {
			return false
		}
	}
	return true
}

func javaScriptRootCategoryAccepts(context SnippetContext, kind string) bool {
	return typeScriptFamilyRootCategoryAccepts("javascript", context, kind)
}

func typeScriptRootCategoryAccepts(context SnippetContext, kind string) bool {
	return typeScriptFamilyRootCategoryAccepts("typescript", context, kind)
}

func tsxRootCategoryAccepts(context SnippetContext, kind string) bool {
	return typeScriptFamilyRootCategoryAccepts("tsx", context, kind)
}

func typeScriptFamilyRootCategoryAccepts(language string, context SnippetContext, kind string) bool {
	switch context {
	case SnippetContextExpression:
		return parser.GrammarSubtype(language, "expression", kind)
	case SnippetContextType:
		return parser.GrammarSubtype(language, "type", kind)
	case SnippetContextStatement, SnippetContextStatementList:
		return parser.GrammarSubtype(language, "statement", kind)
	case SnippetContextDeclaration, SnippetContextDeclarationList:
		return isTypeScriptDeclaration(kind) || isTypeScriptMember(kind) || parser.GrammarSubtype(language, "declaration", kind)
	case SnippetContextFile:
		return kind == "program"
	default:
		return false
	}
}
