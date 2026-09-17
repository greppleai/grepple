package parser

import "path/filepath"

func rustNavigationAdapter(rules *structureRules) navigationAdapter {
	return &navigationAdapterConfig{rules: rules, callTypes: newStringSet("call_expression"), sourceFacts: rustNavigationSourceFacts, exports: rustNavigationExports, nestedModulePath: func(node *syntaxNode, current string) string {
		if node.Kind() != "mod_item" || node.ChildByFieldName("body") == nil {
			return ""
		}
		return rustNavigationJoinPath(current, navigationFieldText(node, "name", ""))
	}, containerName: func(node *syntaxNode, content string, envelope *navigationEnvelope) string {
		if node.Kind() == "impl_item" {
			if target := node.ChildByFieldName("type"); target != nil {
				return target.Text()
			}
		}
		return defaultNavigationDeclarationName(node, content, rules, envelope)
	}, visibility: func(node *syntaxNode, _ string, content string) NavigationVisibility {
		return visibilityFromRequiredModifier(navigationDeclarationHeader(node, content), "pub")
	}, visibilityDetail: rustNavigationVisibilityDetail, entrypoint: rustNavigationEntrypoint, memberAccessFacts: true}
}

func rustNavigationEntrypoint(context navigationEntrypointContext) string {
	if context.node.Kind() != "function_item" || context.name != "main" || context.container != "" || context.scope != "" {
		return ""
	}
	path := filepath.Clean(context.path)
	if !rustCrateRootPath(path) || filepath.Base(path) == "lib.rs" {
		return ""
	}
	return "process"
}
