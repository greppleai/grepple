package parser

import "strings"

type rustNavigationUse struct {
	alias, path, imported string
	line                  int
}

func rustNavigationSourceFacts(root *syntaxNode, _ string, _ *navigationAdapterConfig) (map[string]navigationImport, string, map[string]map[string]navigationBinding) {
	imports, packageName, fields := emptyNavigationSourceFacts()
	pathAttribute := false
	for _, node := range root.NamedChildren() {
		switch node.Kind() {
		case "attribute_item", "inner_attribute_item":
			pathAttribute = pathAttribute || rustNavigationPathAttribute(node.Text())
			continue
		case "mod_item":
			if node.ChildByFieldName("body") == nil && !pathAttribute {
				name := navigationFieldText(node, "name", "")
				addScopedNavigationImport(imports, name, "self::"+name, "*", node.StartLine())
			}
		case "use_declaration":
			for _, item := range rustNavigationUses(node) {
				addScopedNavigationImport(imports, item.alias, item.path, item.imported, item.line)
			}
		}
		pathAttribute = false
	}
	return imports, packageName, fields
}

func rustNavigationExports(root *syntaxNode, _ string, language, path string) []NavigationExport {
	exports := []NavigationExport{}
	for _, node := range root.NamedChildren() {
		if !rustNavigationExported(node) {
			continue
		}
		if node.Kind() == "use_declaration" {
			for _, item := range rustNavigationUses(node) {
				exports = append(exports, NavigationExport{Name: item.alias, LocalName: item.alias, ImportPath: item.path, ImportedName: item.imported, Language: language, Path: path, Line: item.line})
			}
			continue
		}
		if !rustNavigationExportKind(node.Kind()) {
			continue
		}
		name := node.ChildByFieldName("name")
		if name == nil || strings.TrimSpace(name.Text()) == "" {
			continue
		}
		exports = append(exports, NavigationExport{Name: strings.TrimSpace(name.Text()), LocalName: strings.TrimSpace(name.Text()), Language: language, Path: path, Line: node.StartLine()})
	}
	return exports
}

func rustNavigationExported(node *syntaxNode) bool {
	for _, child := range node.NamedChildren() {
		if child.Kind() != "visibility_modifier" {
			continue
		}
		visibility := strings.Join(strings.Fields(child.Text()), "")
		return visibility == "pub" || visibility == "pub(crate)"
	}
	return false
}

func rustNavigationExportKind(kind string) bool {
	switch kind {
	case "function_item", "function_signature_item", "struct_item", "enum_item", "trait_item", "type_item", "const_item", "static_item", "union_item":
		return true
	}
	return false
}

func rustNavigationPathAttribute(value string) bool {
	compact := strings.Join(strings.Fields(value), "")
	return strings.Contains(compact, "path=")
}

func rustNavigationUses(node *syntaxNode) []rustNavigationUse {
	argument := node.ChildByFieldName("argument")
	if argument == nil {
		return nil
	}
	items := []rustNavigationUse{}
	collectRustNavigationUses(argument, "", node.StartLine(), &items)
	return items
}

func collectRustNavigationUses(node *syntaxNode, prefix string, line int, items *[]rustNavigationUse) {
	if node == nil {
		return
	}
	switch node.Kind() {
	case "scoped_use_list":
		path := rustNavigationJoinPath(prefix, navigationFieldText(node, "path", ""))
		collectRustNavigationUses(node.ChildByFieldName("list"), path, line, items)
	case "use_list":
		for _, child := range node.NamedChildren() {
			collectRustNavigationUses(child, prefix, line, items)
		}
	case "use_as_clause":
		pathNode := node.ChildByFieldName("path")
		aliasNode := node.ChildByFieldName("alias")
		if pathNode == nil || aliasNode == nil {
			return
		}
		target, imported := rustNavigationUseTarget(prefix, pathNode.Text())
		appendRustNavigationUse(items, aliasNode.Text(), target, imported, line)
	case "use_wildcard":
		value := strings.TrimSuffix(strings.TrimSpace(node.Text()), "::*")
		if value == "*" {
			value = ""
		}
		target := rustNavigationJoinPath(prefix, value)
		appendRustNavigationUse(items, "*", target, "*", line)
	case "self":
		target := strings.TrimSpace(prefix)
		appendRustNavigationUse(items, rustNavigationTerminal(target), target, rustNavigationTerminal(target), line)
	default:
		target := rustNavigationJoinPath(prefix, node.Text())
		name := rustNavigationTerminal(target)
		appendRustNavigationUse(items, name, target, name, line)
	}
}

func rustNavigationUseTarget(prefix, value string) (string, string) {
	value = strings.TrimSpace(value)
	if value == "self" {
		target := strings.TrimSpace(prefix)
		return target, rustNavigationTerminal(target)
	}
	target := rustNavigationJoinPath(prefix, value)
	return target, rustNavigationTerminal(target)
}

func appendRustNavigationUse(items *[]rustNavigationUse, alias, path, imported string, line int) {
	alias, path, imported = strings.TrimSpace(alias), rustNavigationCompactPath(path), strings.TrimSpace(imported)
	if alias == "" || path == "" || strings.ContainsAny(path, "<>()[]{};,") {
		return
	}
	*items = append(*items, rustNavigationUse{alias: alias, path: path, imported: imported, line: line})
}

func rustNavigationJoinPath(prefix, value string) string {
	prefix, value = rustNavigationCompactPath(prefix), rustNavigationCompactPath(value)
	if prefix == "" || value == "" || rustNavigationAbsolutePath(value) {
		if value == "" {
			return prefix
		}
		return value
	}
	return prefix + "::" + value
}

func rustNavigationAbsolutePath(value string) bool {
	first := value
	if index := strings.Index(value, "::"); index >= 0 {
		first = value[:index]
	}
	return first == "crate" || first == "self" || first == "super" || strings.HasPrefix(value, "::")
}

func rustNavigationCompactPath(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), "")
}

func rustNavigationTerminal(value string) string {
	value = strings.TrimSuffix(rustNavigationCompactPath(value), "::*")
	if index := strings.LastIndex(value, "::"); index >= 0 {
		return value[index+2:]
	}
	return value
}
