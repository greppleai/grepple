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
