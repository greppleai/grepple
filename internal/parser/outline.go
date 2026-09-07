package parser

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
	yaml "go.yaml.in/yaml/v3"
)

// OutlineFile parses content according to the file's language and returns its
// structural outline. Unsupported languages yield an outline with no symbols
// (Markdown and JSON/YAML are handled with lightweight scanners rather than
// tree-sitter).
func OutlineFile(path, content string) FileOutline {
	return OutlineFileDepth(path, content, 0)
}

// OutlineFileDepth is OutlineFile with a nesting cap for the structured-data
// formats (JSON/YAML): maxDepth <= 0 means unlimited, maxDepth N shows at most N
// levels of keys. The cap is ignored for code and Markdown outlines.
func OutlineFileDepth(path, content string, maxDepth int) FileOutline {
	if isMarkdownPath(path) {
		return FileOutline{Path: path, Language: "markdown", Symbols: nonNil(outlineMarkdown(content))}
	}
	if lang := structuredLang(path); lang != "" {
		return FileOutline{Path: path, Language: lang, Symbols: nonNil(outlineStructured(content, maxDepth))}
	}
	lang := LanguageFor(path)
	out := FileOutline{Path: path, Language: lang, Symbols: []Symbol{}}
	config := configForLanguage(lang)
	if config == nil || config.grammar == nil {
		return out
	}
	tree, err := parseTree(config, content)
	if err != nil {
		return out
	}
	defer tree.Close()
	root := tree.RootNode()
	switch lang {
	case "go":
		out.Symbols = nonNil(outlineGo(root, content, config))
	case "java":
		out.Symbols = nonNil(outlineJava(root, content, config))
	case "kotlin":
		out.Symbols = nonNil(outlineKotlin(root, content, config))
	default: // typescript, tsx, javascript
		out.Symbols = nonNil(outlineTSJS(root, content, config))
	}
	return out
}

func nonNil(s []Symbol) []Symbol {
	if s == nil {
		return []Symbol{}
	}
	return s
}

func isMarkdownPath(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md", ".markdown", ".mdown", ".mkd":
		return true
	}
	return false
}

// symbolFrom builds a Symbol for a node, capturing its line bounds and a compact
// one-line signature.
func symbolFrom(kind, name string, node *sitter.Node, content string) Symbol {
	return Symbol{
		Kind:      kind,
		Name:      name,
		Signature: declSignature(node, content),
		Start:     nodeStart(node),
		End:       nodeEnd(node),
	}
}

// declSignature returns the declaration line of a node with its body stripped:
// everything up to the first "{" (or first newline for bodyless declarations),
// whitespace-collapsed onto a single line.
func declSignature(node *sitter.Node, content string) string {
	text := nodeText(node, content)
	if i := strings.IndexByte(text, '{'); i >= 0 {
		text = text[:i]
	} else if i := strings.IndexByte(text, '\n'); i >= 0 {
		text = text[:i]
	}
	return strings.Join(strings.Fields(text), " ")
}

// --- Go ---

func outlineGo(root *sitter.Node, content string, config *languageConfig) []Symbol {
	var out []Symbol
	for _, child := range namedChildren(root) {
		switch child.Kind() {
		case "function_declaration":
			out = append(out, symbolFrom("func", extractNodeName(child, content, config), child, content))
		case "method_declaration":
			name := goReceiver(child, content) + extractNodeName(child, content, config)
			out = append(out, symbolFrom("method", name, child, content))
		case "type_declaration":
			out = append(out, goTypes(child, content, config)...)
		case "const_declaration":
			out = append(out, goValues(child, content, "const")...)
		case "var_declaration":
			out = append(out, goValues(child, content, "var")...)
		}
	}
	return out
}

// goReceiver renders a method's receiver type as a "(*Type)." prefix so methods
// read as members of their type even though Go declares them at file scope.
func goReceiver(method *sitter.Node, content string) string {
	recv := method.ChildByFieldName("receiver")
	if recv == nil {
		return ""
	}
	for _, pd := range namedChildren(recv) {
		if t := pd.ChildByFieldName("type"); t != nil {
			return "(" + nodeText(t, content) + ")."
		}
	}
	return ""
}

func goTypes(decl *sitter.Node, content string, config *languageConfig) []Symbol {
	var out []Symbol
	for _, spec := range namedChildren(decl) {
		if spec.Kind() != "type_spec" && spec.Kind() != "type_alias" {
			continue
		}
		name := extractNodeName(spec, content, config)
		kind := "type"
		var children []Symbol
		if t := spec.ChildByFieldName("type"); t != nil {
			switch t.Kind() {
			case "struct_type":
				kind = "struct"
			case "interface_type":
				kind = "interface"
				children = goInterfaceMembers(t, content, config)
			}
		}
		sym := symbolFrom(kind, name, spec, content)
		sym.Children = children
		out = append(out, sym)
	}
	return out
}

func goInterfaceMembers(iface *sitter.Node, content string, config *languageConfig) []Symbol {
	var out []Symbol
	for _, m := range namedChildren(iface) {
		if m.Kind() == "method_elem" || m.Kind() == "method_spec" {
			out = append(out, symbolFrom("method", extractNodeName(m, content, config), m, content))
		}
	}
	return out
}

func goValues(decl *sitter.Node, content string, label string) []Symbol {
	var out []Symbol
	for _, spec := range namedChildren(decl) {
		if spec.Kind() != "const_spec" && spec.Kind() != "var_spec" {
			continue
		}
		for _, id := range namedChildren(spec) {
			if id.Kind() == "identifier" {
				out = append(out, symbolFrom(label, nodeText(id, content), spec, content))
			}
		}
	}
	return out
}

// --- TypeScript / JavaScript ---

func outlineTSJS(root *sitter.Node, content string, config *languageConfig) []Symbol {
	var out []Symbol
	for _, child := range tsTopLevel(root) {
		out = append(out, tsSymbolsFor(child, content, config)...)
	}
	return out
}

// tsTopLevel unwraps `export`/`export default` statements so the inner
// declaration is treated as a top-level definition.
func tsTopLevel(root *sitter.Node) []*sitter.Node {
	var out []*sitter.Node
	for _, child := range namedChildren(root) {
		if child.Kind() == "export_statement" {
			for _, inner := range namedChildren(child) {
				if tsIsDecl(inner.Kind()) {
					out = append(out, inner)
				}
			}
			continue
		}
		out = append(out, child)
	}
	return out
}

func tsIsDecl(kind string) bool {
	switch kind {
	case "class_declaration", "abstract_class_declaration", "interface_declaration",
		"function_declaration", "generator_function_declaration", "type_alias_declaration",
		"enum_declaration", "lexical_declaration", "variable_declaration":
		return true
	}
	return false
}

func tsSymbolsFor(node *sitter.Node, content string, config *languageConfig) []Symbol {
	name := extractNodeName(node, content, config)
	switch node.Kind() {
	case "class_declaration", "abstract_class_declaration":
		sym := symbolFrom("class", name, node, content)
		sym.Children = tsBodyMembers(node, content, config)
		return []Symbol{sym}
	case "interface_declaration":
		sym := symbolFrom("interface", name, node, content)
		sym.Children = tsBodyMembers(node, content, config)
		return []Symbol{sym}
	case "enum_declaration":
		return []Symbol{symbolFrom("enum", name, node, content)}
	case "function_declaration", "generator_function_declaration":
		return []Symbol{symbolFrom("function", name, node, content)}
	case "type_alias_declaration":
		return []Symbol{symbolFrom("type", name, node, content)}
	case "lexical_declaration", "variable_declaration":
		return tsVariables(node, content, config)
	}
	return nil
}

func tsVariables(decl *sitter.Node, content string, config *languageConfig) []Symbol {
	var out []Symbol
	for _, d := range namedChildren(decl) {
		if d.Kind() != "variable_declarator" {
			continue
		}
		name := ""
		if n := d.ChildByFieldName("name"); n != nil {
			name = nodeText(n, content)
		}
		kind := "const"
		if v := d.ChildByFieldName("value"); v != nil && config.functionLikeTypes.contains(v.Kind()) {
			kind = "function"
		}
		out = append(out, symbolFrom(kind, name, d, content))
	}
	return out
}

// tsBodyMembers lists the members of a class or interface body.
func tsBodyMembers(node *sitter.Node, content string, config *languageConfig) []Symbol {
	body := node.ChildByFieldName("body")
	if body == nil {
		return nil
	}
	var out []Symbol
	for _, m := range namedChildren(body) {
		switch m.Kind() {
		case "method_definition", "method_signature":
			out = append(out, symbolFrom("method", extractNodeName(m, content, config), m, content))
		case "public_field_definition", "field_definition", "property_signature":
			out = append(out, symbolFrom("property", extractNodeName(m, content, config), m, content))
		}
	}
	return out
}

// --- Java ---

func outlineJava(root *sitter.Node, content string, config *languageConfig) []Symbol {
	var out []Symbol
	for _, child := range namedChildren(root) {
		if config.classDeclarationTypes.contains(child.Kind()) {
			out = append(out, javaType(child, content, config))
		}
	}
	return out
}

func javaType(node *sitter.Node, content string, config *languageConfig) Symbol {
	sym := symbolFrom(javaTypeKind(node.Kind()), extractNodeName(node, content, config), node, content)
	sym.Children = javaMembers(node, content, config)
	return sym
}

func javaTypeKind(kind string) string {
	switch kind {
	case "interface_declaration":
		return "interface"
	case "enum_declaration":
		return "enum"
	case "record_declaration":
		return "record"
	default:
		return "class"
	}
}

func javaMembers(node *sitter.Node, content string, config *languageConfig) []Symbol {
	body := node.ChildByFieldName("body")
	if body == nil {
		return nil
	}
	var out []Symbol
	for _, m := range namedChildren(body) {
		switch m.Kind() {
		case "method_declaration":
			out = append(out, symbolFrom("method", extractNodeName(m, content, config), m, content))
		case "constructor_declaration":
			out = append(out, symbolFrom("constructor", extractNodeName(m, content, config), m, content))
		case "field_declaration":
			for _, d := range namedChildren(m) {
				if d.Kind() == "variable_declarator" {
					out = append(out, symbolFrom("field", extractNodeName(d, content, config), m, content))
				}
			}
		case "enum_constant":
			out = append(out, symbolFrom("const", extractNodeName(m, content, config), m, content))
		case "class_declaration", "interface_declaration", "enum_declaration", "record_declaration":
			out = append(out, javaType(m, content, config))
		}
	}
	return out
}

// --- Kotlin ---

func outlineKotlin(root *sitter.Node, content string, config *languageConfig) []Symbol {
	var out []Symbol
	for _, child := range namedChildren(root) {
		switch child.Kind() {
		case "class_declaration":
			kind := "class"
			if sig := declSignature(child, content); strings.Contains(sig, "interface ") {
				kind = "interface"
			}
			sym := symbolFrom(kind, extractNodeName(child, content, config), child, content)
			sym.Children = kotlinMembers(child, content, config)
			out = append(out, sym)
		case "object_declaration":
			sym := symbolFrom("object", extractNodeName(child, content, config), child, content)
			sym.Children = kotlinMembers(child, content, config)
			out = append(out, sym)
		case "function_declaration":
			out = append(out, symbolFrom("fun", extractNodeName(child, content, config), child, content))
		case "property_declaration":
			out = append(out, symbolFrom("val", kotlinPropertyName(child, content, config), child, content))
		}
	}
	return out
}

func kotlinMembers(node *sitter.Node, content string, config *languageConfig) []Symbol {
	var body *sitter.Node
	for _, child := range namedChildren(node) {
		if child.Kind() == "class_body" || child.Kind() == "enum_class_body" {
			body = child
			break
		}
	}
	if body == nil {
		return nil
	}
	var out []Symbol
	for _, m := range namedChildren(body) {
		switch m.Kind() {
		case "function_declaration":
			out = append(out, symbolFrom("fun", extractNodeName(m, content, config), m, content))
		case "property_declaration":
			out = append(out, symbolFrom("val", kotlinPropertyName(m, content, config), m, content))
		}
	}
	return out
}

// kotlinPropertyName digs the variable name out of a property_declaration, whose
// name sits inside a variable_declaration child rather than a "name" field.
func kotlinPropertyName(node *sitter.Node, content string, config *languageConfig) string {
	if name := extractNodeName(node, content, config); name != "" {
		return name
	}
	for _, child := range namedChildren(node) {
		if child.Kind() == "variable_declaration" {
			return extractNodeName(child, content, config)
		}
	}
	return ""
}

// --- Markdown ---

type mdHeading struct {
	level int
	line  int
	title string
}

// scanMarkdownHeadings returns the ATX headings (# .. ######) in document order,
// skipping any that appear inside fenced code blocks (``` or ~~~).
func scanMarkdownHeadings(content string) []mdHeading {
	var heads []mdHeading
	var fence fenceTracker
	for i, raw := range strings.Split(content, "\n") {
		trimmed := strings.TrimLeft(raw, " ")
		if fence.skip(trimmed) {
			continue
		}
		if heading, ok := parseATXHeading(trimmed, i+1); ok {
			heads = append(heads, heading)
		}
	}
	return heads
}

// fenceTracker tracks fenced code blocks (``` or ~~~) while scanning markdown.
type fenceTracker struct {
	inside bool
	marker byte
}

// skip reports whether the line is a fence delimiter or inside a fenced code
// block (neither can be a heading), toggling the state on delimiters.
func (f *fenceTracker) skip(trimmed string) bool {
	if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
		if !f.inside {
			f.inside, f.marker = true, trimmed[0]
		} else if trimmed[0] == f.marker {
			f.inside = false
		}
		return true
	}
	return f.inside
}

// parseATXHeading parses one line as an ATX heading; ok=false when it is not
// one (more than six hashes, or no space after the markers — "#hashtag").
func parseATXHeading(trimmed string, line int) (mdHeading, bool) {
	if !strings.HasPrefix(trimmed, "#") {
		return mdHeading{}, false
	}
	level := 0
	for level < len(trimmed) && trimmed[level] == '#' {
		level++
	}
	if level > 6 {
		return mdHeading{}, false
	}
	if level < len(trimmed) && trimmed[level] != ' ' && trimmed[level] != '\t' {
		return mdHeading{}, false // e.g. "#hashtag", not a heading
	}
	title := strings.TrimSpace(strings.TrimRight(strings.TrimSpace(trimmed[level:]), "#"))
	title = strings.TrimSpace(title)
	return mdHeading{level: level, line: line, title: title}, true
}

// markdownHeadingEnds computes the last line each heading's section spans: up to
// (but excluding) the next heading of the same or higher level, else EOF. This is
// what makes a heading's [line, end] range contain all of its nested content, so a
// match's enclosing heading chain is every heading whose range covers it.
func markdownHeadingEnds(heads []mdHeading, lineCount int) []int {
	ends := make([]int, len(heads))
	for k := range heads {
		ends[k] = lineCount
		for m := k + 1; m < len(heads); m++ {
			if heads[m].level <= heads[k].level {
				ends[k] = heads[m].line - 1
				break
			}
		}
	}
	return ends
}

func outlineMarkdown(content string) []Symbol {
	heads := scanMarkdownHeadings(content)
	if len(heads) == 0 {
		return nil
	}
	ends := markdownHeadingEnds(heads, strings.Count(content, "\n")+1)
	return mdTree(heads, ends)
}

// buildMarkdownSegments renders match context the way the AST path does for code:
// each matched line is shown, preceded by its enclosing heading chain as
// "summary" segments (breadcrumb), with everything else collapsed. So a hit deep
// in a document reads as `# Doc` › `## Section` › `### Subsection` › <line>.
func buildMarkdownSegments(content string, hits map[int]bool, maxSegments int) []Segment {
	lines := splitLines(content)
	heads := scanMarkdownHeadings(content)
	ends := markdownHeadingEnds(heads, len(lines))

	matched := make([]int, 0, len(hits))
	for line := range hits {
		matched = append(matched, line)
	}
	sort.Ints(matched)

	var segments []Segment
	seenHeading := map[int]bool{}
	for _, ml := range matched {
		for k, h := range heads {
			// Ancestor heading: its section covers the match, and it is not the
			// matched line itself (that is shown as a real line below).
			if h.line <= ml && ml <= ends[k] && h.line != ml && !seenHeading[h.line] {
				seenHeading[h.line] = true
				text := ""
				if h.line >= 1 && h.line <= len(lines) {
					text = strings.TrimRight(lines[h.line-1], " \t")
				}
				segments = append(segments, Segment{Kind: "summary", Start: h.line, End: h.line, Text: text})
			}
		}
		segments = append(segments, Segment{Kind: "lines", Start: ml, End: ml})
	}
	return limitSegments(mergeSegments(segments, len(lines)), maxSegments)
}

type mdNode struct {
	level int
	sym   Symbol
	kids  []*mdNode
}

func mdTree(heads []mdHeading, ends []int) []Symbol {
	root := &mdNode{}
	stack := []*mdNode{root}
	for k, h := range heads {
		node := &mdNode{level: h.level, sym: Symbol{
			Kind:  fmt.Sprintf("h%d", h.level),
			Name:  h.title,
			Start: h.line,
			End:   ends[k],
		}}
		for len(stack) > 1 && stack[len(stack)-1].level >= h.level {
			stack = stack[:len(stack)-1]
		}
		parent := stack[len(stack)-1]
		parent.kids = append(parent.kids, node)
		stack = append(stack, node)
	}
	return mdConvert(root.kids)
}

func mdConvert(nodes []*mdNode) []Symbol {
	out := make([]Symbol, 0, len(nodes))
	for _, n := range nodes {
		n.sym.Children = mdConvert(n.kids)
		out = append(out, n.sym)
	}
	return out
}

// --- JSON / YAML (structured data) ---
//
// These formats get a key/shape skeleton with values omitted: every mapping key
// and its value's type, nested, with line ranges — the win for large config files.
// Parsing is via yaml.v3 (JSON is valid YAML), whose Node preserves line numbers,
// so no tree-sitter grammar is added. YAML multi-document streams (--- separated)
// are supported: each document becomes a `document [i]` node.

func structuredLang(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		return "json"
	case ".yaml", ".yml":
		return "yaml"
	}
	return ""
}

func outlineStructured(content string, maxDepth int) []Symbol {
	dec := yaml.NewDecoder(strings.NewReader(content))
	var docs []*yaml.Node
	for {
		var doc yaml.Node
		if err := dec.Decode(&doc); err != nil {
			break // EOF or parse error: return whatever parsed cleanly so far
		}
		if root := documentRoot(&doc); root != nil {
			docs = append(docs, root)
		}
	}
	if len(docs) == 0 {
		return nil
	}
	// Single document: list its top-level members directly. Multiple documents:
	// wrap each in a `document [i]` node so the stream structure is visible.
	if len(docs) == 1 {
		return structuredMembers(docs[0], 1, maxDepth)
	}
	syms := make([]Symbol, 0, len(docs))
	for i, root := range docs {
		syms = append(syms, Symbol{
			Kind:     "document",
			Name:     fmt.Sprintf("[%d]", i),
			Start:    root.Line,
			End:      nodeEndLine(root),
			Children: structuredMembers(root, 1, maxDepth),
		})
	}
	return syms
}

func documentRoot(doc *yaml.Node) *yaml.Node {
	if doc.Kind == yaml.DocumentNode {
		if len(doc.Content) > 0 {
			return doc.Content[0]
		}
		return nil
	}
	if doc.Kind == 0 {
		return nil // empty document (e.g. trailing --- )
	}
	return doc
}

// structuredMembers returns the symbols for the members of a container node: one
// per mapping key, or one per container element of a sequence (scalar elements are
// skipped — with values hidden they carry no information). depth is the level of
// the members being produced (top-level = 1).
func structuredMembers(node *yaml.Node, depth, maxDepth int) []Symbol {
	switch node.Kind {
	case yaml.MappingNode:
		syms := make([]Symbol, 0, len(node.Content)/2)
		for i := 0; i+1 < len(node.Content); i += 2 {
			key, val := node.Content[i], node.Content[i+1]
			syms = append(syms, structuredEntry(key.Value, key.Line, val, depth, maxDepth))
		}
		return syms
	case yaml.SequenceNode:
		var syms []Symbol
		for i, el := range node.Content {
			if !yamlIsContainer(el) {
				continue // scalar element: nothing to show once the value is hidden
			}
			syms = append(syms, structuredEntry(fmt.Sprintf("[%d]", i), el.Line, el, depth, maxDepth))
		}
		return syms
	}
	return nil
}

// structuredEntry builds the Symbol for one key (or sequence element): its type,
// line span, an array-length suffix for sequences, and — unless the depth cap is
// reached — its nested members.
func structuredEntry(name string, startLine int, val *yaml.Node, depth, maxDepth int) Symbol {
	sym := Symbol{
		Kind:  yamlKind(val),
		Name:  name,
		Start: startLine,
		End:   maxInt(startLine, nodeEndLine(val)),
	}
	if val.Kind == yaml.SequenceNode {
		sym.Name = fmt.Sprintf("%s [%d]", name, len(val.Content))
	}
	if yamlIsContainer(val) && (maxDepth <= 0 || depth < maxDepth) {
		sym.Children = structuredMembers(val, depth+1, maxDepth)
	}
	return sym
}

func yamlIsContainer(n *yaml.Node) bool {
	return n.Kind == yaml.MappingNode || n.Kind == yaml.SequenceNode
}

// yamlKind maps a node to the outline's type vocabulary using yaml.v3's resolved
// tag (so `8787` is number, `true` is bool, `~` is null, etc., for JSON and YAML).
func yamlKind(n *yaml.Node) string {
	switch n.Kind {
	case yaml.MappingNode:
		return "object"
	case yaml.SequenceNode:
		return "array"
	case yaml.AliasNode:
		return "alias"
	}
	switch n.ShortTag() {
	case "!!int", "!!float":
		return "number"
	case "!!bool":
		return "bool"
	case "!!null":
		return "null"
	}
	return "string"
}

// nodeEndLine is the last line a node spans: the max line over the node and all of
// its descendants (yaml.v3 records only start lines).
func nodeEndLine(n *yaml.Node) int {
	end := n.Line
	for _, c := range n.Content {
		if e := nodeEndLine(c); e > end {
			end = e
		}
	}
	return end
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
