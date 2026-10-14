package parser

import "sort"

// markdownLineScopeAnnotations uses the same heading recognition and section
// boundaries as Markdown outline/segments. Heading titles are never labels.
func markdownLineScopeAnnotations(content string, hits map[int]bool) map[int]LineScopeAnnotation {
	adapter := contentAdapters["markdown"]
	if adapter == nil {
		return nil
	}
	tree, err := adapter.Parse(content)
	if err != nil {
		return nil
	}
	defer tree.Close()
	root := tree.RootNode()
	if root == nil || root.HasError() {
		return nil
	}
	var heads []mdHeading
	collectMarkdownHeadings(root, &heads)
	if len(heads) == 0 {
		return nil
	}
	lineCount := len(splitLines(content))
	ends := markdownHeadingEnds(heads, lineCount)
	matched := make([]int, 0, len(hits))
	for line := range hits {
		if line > 0 && line <= lineCount {
			matched = append(matched, line)
		}
	}
	sort.Ints(matched)
	result := make(map[int]LineScopeAnnotation, len(matched))
	// The deepest containing heading is a section. An enclosing heading is
	// also a section, so both roles identify the same smallest section.
	for _, line := range matched {
		for index := len(heads) - 1; index >= 0; index-- {
			if heads[index].line <= line && line <= ends[index] {
				span := ScopeLineRange{Kind: "section", StartLine: heads[index].line, EndLine: ends[index]}
				result[line] = LineScopeAnnotation{Nearest: &span, Scope: &span}
				break
			}
		}
	}
	return result
}
