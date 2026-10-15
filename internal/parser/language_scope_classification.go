package parser

// scopeKindForNode asks the effective registered language adapter for the
// grammar vocabulary it owns. There is no cross-language scope-kind registry.
func scopeKindForNode(node ViewNode) string {
	language := effectiveScopeLanguage(node)
	adapter := languageAdapters[language]
	if adapter == nil {
		return ""
	}
	kind := adapter.ScopeKind(node.Kind())
	if incompleteGoFunctionPrefix(node, language) {
		return ""
	}
	if kind == "func" && language != "go" && language != "shell" && memberFunctionDeclaration(language, node.Kind()) {
		for parent := node.Parent(); parent.Valid(); parent = parent.Parent() {
			if language == "rust" && parent.Kind() == "impl_item" {
				return "method"
			}
			parentKind := adapter.ScopeKind(parent.Kind())
			if parentKind == "func" || parentKind == "method" {
				break
			}
			if parentKind == "class" || parentKind == "struct" || parentKind == "interface" {
				return "method"
			}
		}
	}
	return kind
}

func effectiveScopeLanguage(node ViewNode) string {
	if node.raw != nil && node.raw.tree != nil && node.raw.tree.language != "" {
		return node.raw.tree.language
	}
	return node.Language()
}
func nativeScopeKind(language, kind string) string {
	if kind == "func" {
		switch language {
		case "javascript", "typescript", "tsx", "php", "dart", "c", "cpp", "shell":
			return "function"
		case "python":
			return "def"
		case "kotlin":
			return "fun"
		case "rust":
			return "fn"
		}
	}
	if language == "rust" && kind == "interface" {
		return "trait"
	}
	return kind
}
func scopeSelectorKindMatches(language, actual, requested string) bool {
	return actual == requested || requested == "func" && actual == nativeScopeKind(language, "func")
}
func incompleteGoFunctionPrefix(node ViewNode, language string) bool {
	if language != "go" || node.raw == nil || node.raw.raw == nil || node.Kind() != "function_declaration" && node.Kind() != "method_declaration" {
		return false
	}
	next := node.raw.raw.NextNamedSibling()
	return next != nil && next.IsError() && next.StartPosition().Row == node.raw.raw.EndPosition().Row
}
func memberFunctionDeclaration(language, nodeKind string) bool {
	switch language {
	case "kotlin", "swift", "dart", "javascript", "typescript", "tsx":
		return nodeKind == "function_declaration" || nodeKind == "protocol_function_declaration"
	case "python", "php", "c", "cpp":
		return nodeKind == "function_definition"
	case "rust":
		return nodeKind == "function_item"
	}
	return false
}
func navigableScopeKind(language, kind string) bool {
	switch kind {
	case "func", "method", "class", "struct", "interface", "enum", "snippet":
		return true
	case "block":
		return language == "hcl"
	case "rule":
		return language == "css"
	}
	return false
}
