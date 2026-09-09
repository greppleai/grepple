package parser

import (
	"fmt"
	"path/filepath"
	"strings"

	yaml "go.yaml.in/yaml/v3"
)

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
