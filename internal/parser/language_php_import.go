package parser

import "strings"

// phpNavigationImport records explicit namespace imports; trait use and dynamic
// require/include expressions are not file-wide namespace bindings.
func phpNavigationImport(imports map[string]navigationImport, node *syntaxNode) {
	var visit func(*syntaxNode, string)
	visit = func(item *syntaxNode, prefix string) {
		if item.Kind() == "namespace_use_group" {
			for _, child := range item.NamedChildren() {
				visit(child, prefix)
			}
			return
		}
		if item.Kind() != "namespace_use_clause" {
			return
		}
		var name string
		for _, child := range item.NamedChildren() {
			if child.Kind() == "qualified_name" || child.Kind() == "name" {
				name = child.Text()
				break
			}
		}
		name = strings.TrimPrefix(name, `\`)
		if prefix != "" {
			name = prefix + `\` + name
		}
		if name == "" {
			return
		}
		parts := strings.Split(name, `\`)
		alias := parts[len(parts)-1]
		if named := item.ChildByFieldName("alias"); named != nil {
			alias = named.Text()
		}
		if len(parts) < 2 {
			return
		} // no resolvable namespace
		addScopedNavigationImport(imports, alias, strings.Join(parts, "."), parts[len(parts)-1], item.StartLine())
	}
	prefix := ""
	for _, item := range node.NamedChildren() {
		if item.Kind() == "namespace_name" || item.Kind() == "qualified_name" {
			prefix = item.Text()
		}
	}
	for _, item := range node.NamedChildren() {
		visit(item, prefix)
	}
}
