package parser

// HCL blocks are configuration declarations, not functions. Only blocks with
// stable, literal Terraform addresses become navigable declarations. Calls are
// actual HCL function invocations; resource traversals are not function calls.
func hclNavigationAdapter(rules *structureRules) navigationAdapter {
	return &navigationAdapterConfig{
		rules:     rules,
		callTypes: newStringSet("function_call"),
		isCallable: func(node *syntaxNode) bool {
			if node.Kind() != "block" || !hclTopLevelBlock(node) {
				return false
			}
			switch hclBlockType(node) {
			case "resource", "data", "module", "variable", "output", "provider", "terraform", "locals":
				return true
			}
			return false
		},
		declarationName: func(node *syntaxNode, _ string, _ *navigationEnvelope) string {
			return hclBlockName(node)
		},
		declarationKind: func(_ *syntaxNode, _ string) string { return "block" },
	}
}

func hclTopLevelBlock(node *syntaxNode) bool {
	body := node.Parent()
	if body == nil || body.Kind() != "body" {
		return false
	}
	root := body.Parent()
	return root != nil && root.Kind() == "config_file"
}

func hclBlockType(node *syntaxNode) string {
	for _, child := range node.NamedChildren() {
		if child.Kind() == "identifier" {
			return child.Text()
		}
		break
	}
	return ""
}
