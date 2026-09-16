package parser

import (
	"path/filepath"
	"strconv"
	"strings"
)

type rustNavigationUse struct {
	alias, path, imported string
	line                  int
}

func rustNavigationSourceFacts(root *syntaxNode, _ string, _ *navigationAdapterConfig) (map[string]navigationImport, string, map[string]map[string]navigationBinding) {
	imports, packageName, fields := emptyNavigationSourceFacts()
	collectRustNavigationSourceFacts(root, "", imports)
	return imports, packageName, fields
}

func collectRustNavigationSourceFacts(root *syntaxNode, scope string, imports map[string]navigationImport) {
	pathHint, pathAttribute := "", false
	for _, node := range root.NamedChildren() {
		if node.Kind() == "attribute_item" || node.Kind() == "inner_attribute_item" {
			if hint, present := rustNavigationPathAttribute(node.Text()); present {
				pathHint, pathAttribute = hint, true
			}
			continue
		}
		collectRustNavigationSourceNode(node, scope, pathHint, pathAttribute, imports)
		pathHint, pathAttribute = "", false
	}
}

func collectRustNavigationSourceNode(node *syntaxNode, scope, pathHint string, pathAttribute bool, imports map[string]navigationImport) {
	switch node.Kind() {
	case "mod_item":
		collectRustNavigationModule(node, scope, pathHint, pathAttribute, imports)
	case "use_declaration":
		for _, item := range rustNavigationUses(node) {
			addRustNavigationImport(imports, navigationImport{alias: item.alias, path: rustNavigationScopedPath(item.path, scope), imported: item.imported, scope: scope, line: item.line})
		}
	}
}

func collectRustNavigationModule(node *syntaxNode, scope, pathHint string, pathAttribute bool, imports map[string]navigationImport) {
	name := navigationFieldText(node, "name", "")
	body := node.ChildByFieldName("body")
	if name != "" && (body != nil || !pathAttribute || pathHint != "") {
		addRustNavigationImport(imports, navigationImport{alias: name, path: rustNavigationScopedPath("self::"+name, scope), imported: "*", kind: "module", scope: scope, targetPathHint: pathHint, inline: body != nil, line: node.StartLine()})
	}
	if body != nil && name != "" {
		collectRustNavigationSourceFacts(body, rustNavigationJoinPath(scope, name), imports)
	}
}

func addRustNavigationImport(imports map[string]navigationImport, item navigationImport) {
	if item.alias == "" || item.path == "" {
		return
	}
	key := item.alias
	if item.scope != "" {
		key = item.scope + "\x01" + item.alias
	}
	if item.alias == "*" {
		imports[navigationImportUniqueKey(item)] = item
		return
	}
	if existing, ok := imports[key]; ok {
		delete(imports, key)
		imports[navigationImportUniqueKey(existing)] = existing
		imports[navigationImportUniqueKey(item)] = item
		return
	}
	prefix := item.scope + "\x00" + item.alias + "\x00"
	for existingKey := range imports {
		if strings.HasPrefix(existingKey, prefix) {
			imports[navigationImportUniqueKey(item)] = item
			return
		}
	}
	imports[key] = item
}

func rustNavigationExports(root *syntaxNode, _ string, language, path string) []NavigationExport {
	exports := []NavigationExport{}
	collectRustNavigationExports(root, "", language, path, &exports)
	return exports
}

func collectRustNavigationExports(root *syntaxNode, scope, language, path string, exports *[]NavigationExport) {
	for _, node := range root.NamedChildren() {
		collectRustNestedModuleExports(node, scope, language, path, exports)
		if rustNavigationExported(node) {
			*exports = append(*exports, rustNavigationNodeExports(node, scope, language, path)...)
		}
	}
}

func collectRustNestedModuleExports(node *syntaxNode, scope, language, path string, exports *[]NavigationExport) {
	if node.Kind() != "mod_item" {
		return
	}
	body := node.ChildByFieldName("body")
	name := navigationFieldText(node, "name", "")
	if body != nil && name != "" {
		collectRustNavigationExports(body, rustNavigationJoinPath(scope, name), language, path, exports)
	}
}

func rustNavigationNodeExports(node *syntaxNode, scope, language, path string) []NavigationExport {
	if node.Kind() == "use_declaration" {
		exports := []NavigationExport{}
		for _, item := range rustNavigationUses(node) {
			exports = append(exports, NavigationExport{Name: item.alias, LocalName: item.alias, ImportPath: rustNavigationScopedPath(item.path, scope), ImportedName: item.imported, Scope: scope, Language: language, Path: path, Line: item.line})
		}
		return exports
	}
	if !rustNavigationExportKind(node.Kind()) {
		return nil
	}
	name := node.ChildByFieldName("name")
	if name == nil || strings.TrimSpace(name.Text()) == "" {
		return nil
	}
	return []NavigationExport{{Name: strings.TrimSpace(name.Text()), LocalName: strings.TrimSpace(name.Text()), Scope: scope, Language: language, Path: path, Line: node.StartLine()}}
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

func rustNavigationPathAttribute(value string) (string, bool) {
	trimmed := strings.TrimSpace(value)
	inner := strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(trimmed, "#["), "#!["), "]")
	if strings.HasPrefix(inner, "path") {
		remainder := strings.TrimSpace(inner[len("path"):])
		if strings.HasPrefix(remainder, "=") {
			path, ok := rustNavigationStringLiteral(strings.TrimSpace(strings.TrimPrefix(remainder, "=")))
			if !ok || strings.TrimSpace(path) == "" {
				return "", true
			}
			return filepath.ToSlash(strings.TrimSpace(path)), true
		}
	}
	compact := strings.Join(strings.Fields(value), "")
	return "", strings.Contains(compact, "path=")
}

func rustNavigationStringLiteral(value string) (string, bool) {
	if strings.HasPrefix(value, `"`) {
		path, err := strconv.Unquote(value)
		return path, err == nil
	}
	if !strings.HasPrefix(value, "r") {
		return "", false
	}
	hashes := 0
	for hashes+1 < len(value) && value[hashes+1] == '#' {
		hashes++
	}
	openingQuote := hashes + 1
	if openingQuote >= len(value) || value[openingQuote] != '"' {
		return "", false
	}
	suffix := `"` + strings.Repeat("#", hashes)
	if len(value) < openingQuote+1+len(suffix) || !strings.HasSuffix(value, suffix) {
		return "", false
	}
	content := value[openingQuote+1 : len(value)-len(suffix)]
	if strings.Contains(content, suffix) {
		return "", false
	}
	return content, true
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

func rustNavigationScopedPath(value, scope string) string {
	value, scope = rustNavigationCompactPath(value), rustNavigationCompactPath(scope)
	if scope == "" || value == "" || strings.HasPrefix(value, "crate::") || strings.HasPrefix(value, "::") {
		return value
	}
	scopeParts := strings.Split(scope, "::")
	parts := strings.Split(value, "::")
	switch parts[0] {
	case "self":
		return rustNavigationJoinPath("self::"+scope, strings.Join(parts[1:], "::"))
	case "super":
		count := 0
		for count < len(parts) && parts[count] == "super" {
			count++
		}
		if count <= len(scopeParts) {
			prefix := "self"
			if remaining := scopeParts[:len(scopeParts)-count]; len(remaining) > 0 {
				prefix += "::" + strings.Join(remaining, "::")
			}
			return rustNavigationJoinPath(prefix, strings.Join(parts[count:], "::"))
		}
		remaining := append([]string(nil), parts[len(scopeParts):]...)
		return strings.Join(remaining, "::")
	default:
		return value
	}
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

// RustScopedPath normalizes self/super paths from an inline-module scope while
// preserving crate-qualified and external paths.
func RustScopedPath(value, scope string) string {
	return rustNavigationScopedPath(value, scope)
}

func rustNavigationTerminal(value string) string {
	value = strings.TrimSuffix(rustNavigationCompactPath(value), "::*")
	if index := strings.LastIndex(value, "::"); index >= 0 {
		return value[index+2:]
	}
	return value
}
