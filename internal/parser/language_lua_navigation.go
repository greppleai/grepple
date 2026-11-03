package parser

// Lua calls and named function declarations have syntax-backed navigation facts.
// Dynamic require(), table assignments and inferred module exports are not resolved.
func luaNavigationAdapter(rules *structureRules) navigationAdapter {
	return &navigationAdapterConfig{
		rules: rules, callTypes: newStringSet("function_call"),
		isCallable:              func(node *syntaxNode) bool { return node.Kind() == "function_declaration" },
		nestedBindingScopeTypes: newStringSet("block"),
		terminalName: func(node *syntaxNode, _ string) string {
			if node.Kind() == "identifier" {
				return node.Text()
			}
			return defaultNavigationTerminalName(node)
		},
	}
}
