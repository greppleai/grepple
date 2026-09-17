package parser

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
	}, visibilityDetail: rustNavigationVisibilityDetail}
}
