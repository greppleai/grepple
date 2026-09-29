package parser

import (
	"fmt"
	"sort"
	"strings"
)

type mdHeading struct {
	level int
	line  int
	title string
}

func collectMarkdownHeadings(node *syntaxNode, heads *[]mdHeading) {
	switch node.Kind() {
	case "atx_heading":
		level, title := 0, ""
		for _, child := range node.Children() {
			kind := child.Kind()
			switch {
			case len(kind) == len("atx_h1_marker") && strings.HasPrefix(kind, "atx_h") && strings.HasSuffix(kind, "_marker"):
				level = int(kind[5] - '0')
			case kind == "inline":
				title = mdATXTitle(child.Text())
			}
		}
		if level > 0 {
			*heads = append(*heads, mdHeading{level: level, line: node.StartLine(), title: title})
		}
		return
	case "setext_heading":
		level, title := 1, ""
		for _, child := range node.Children() {
			switch child.Kind() {
			case "setext_h2_underline":
				level = 2
			case "paragraph":
				title = mdSetextTitle(child.Text())
			}
		}
		*heads = append(*heads, mdHeading{level: level, line: node.StartLine(), title: title})
		return
	}
	// Only document and section containers hold structural headings; headings
	// nested in lists, block quotes, code, or HTML are excluded.
	if kind := node.Kind(); kind != "document" && kind != "section" {
		return
	}
	for _, child := range node.NamedChildren() {
		collectMarkdownHeadings(child, heads)
	}
}

// mdATXTitle strips an optional closing marker sequence (whitespace followed by
// trailing '#'s, or '#'s making up the whole content) and normalizes whitespace.
func mdATXTitle(text string) string {
	text = strings.TrimSpace(text)
	trimmed := strings.TrimRight(text, "#")
	if len(trimmed) < len(text) {
		if trimmed == "" {
			text = ""
		} else if last := trimmed[len(trimmed)-1]; last == ' ' || last == '\t' {
			text = strings.TrimRight(trimmed, " \t")
		}
	}
	return strings.Join(strings.Fields(text), " ")
}

// mdSetextTitle joins every content line of a Setext heading.
func mdSetextTitle(text string) string {
	return strings.Join(strings.Fields(text), " ")
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

// buildMarkdownSegments renders match context the way the AST path does for code:
// each matched line is shown, preceded by its enclosing heading chain as
// "summary" segments (breadcrumb), with everything else collapsed. So a hit deep
// in a document reads as `# Doc` › `## Section` › `### Subsection` › <line>.
func buildMarkdownSegments(root *syntaxNode, content string, hits map[int]bool) []Segment {
	lines := splitLines(content)
	var heads []mdHeading
	collectMarkdownHeadings(root, &heads)
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
	return mergeSegments(segments, len(lines))
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
