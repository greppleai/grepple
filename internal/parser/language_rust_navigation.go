package parser

import (
	"path/filepath"
	"strings"
)

func rustNavigationAdapter(rules *structureRules) navigationAdapter {
	return &navigationAdapterConfig{rules: rules, callTypes: newStringSet("call_expression"), parameterTypes: newStringSet("parameter"), fieldContainerTypes: newStringSet("struct_item"), sourceFacts: rustNavigationSourceFacts, exports: rustNavigationExports, typeReferenceFacts: true, nestedModulePath: func(node *syntaxNode, current string) string {
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
	}, visibilityDetail: rustNavigationVisibilityDetail, entrypoint: rustNavigationEntrypoint, fieldFacts: true, memberAccessFacts: true}
}

func rustNavigationEntrypoint(context navigationEntrypointContext) string {
	if context.node.Kind() != "function_item" || context.name != "main" || context.container != "" || context.scope != "" {
		return ""
	}
	path := filepath.Clean(context.path)
	if !rustExecutableCratePath(path) {
		return ""
	}
	return "process"
}

// rustExecutableCratePath recognizes a process entrypoint, not a crate library.
func rustExecutableCratePath(path string) bool {
	if strings.ToLower(filepath.Ext(path)) != ".rs" {
		return false
	}
	base := filepath.Base(path)
	if base == "lib.rs" {
		return false
	}
	parent := filepath.Base(filepath.Dir(path))
	if parent == "bin" && filepath.Base(filepath.Dir(filepath.Dir(path))) == "src" {
		return true
	}
	if base != "main.rs" {
		return false
	}
	if parent == "src" {
		return true
	}
	return filepath.Base(filepath.Dir(filepath.Dir(path))) == "bin" && filepath.Base(filepath.Dir(filepath.Dir(filepath.Dir(path)))) == "src"
}
