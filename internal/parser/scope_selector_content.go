package parser

// markdownScopeSelector reuses the same heading recognition and end-line rules
// as the line-only projection; the heading tree is parsed once per request.
func markdownScopeSelector(content string, selector ScopeSelector) (ScopeLineRange, ScopeSelectionStatus) {
	if selector.Kind != "section" {
		return ScopeLineRange{}, ScopeMissing
	}
	adapter := contentAdapters["markdown"]
	if adapter == nil {
		return ScopeLineRange{}, ScopeMissing
	}
	tree, err := adapter.Parse(content)
	if err != nil {
		return ScopeLineRange{}, ScopeMissing
	}
	defer tree.Close()
	root := tree.RootNode()
	if root == nil || root.HasError() {
		return ScopeLineRange{}, ScopeMissing
	}
	var heads []mdHeading
	collectMarkdownHeadings(root, &heads)
	ends := markdownHeadingEnds(heads, len(splitLines(content)))
	var chosen *ScopeLineRange
	for index, head := range heads {
		span := ScopeLineRange{Kind: "section", StartLine: head.line, EndLine: ends[index]}
		if selector.Exact {
			if span.StartLine != selector.StartLine || span.EndLine != selector.EndLine {
				continue
			}
		} else if selector.StartLine < span.StartLine || selector.StartLine > span.EndLine {
			continue
		}
		if chosen != nil && *chosen != span {
			if selector.Exact {
				return ScopeLineRange{}, ScopeAmbiguous
			}
			// Only the deepest heading is rendered for an interior hit.
			if chosen.StartLine > span.StartLine {
				continue
			}
		}
		selected := span
		chosen = &selected
	}
	if chosen == nil {
		return ScopeLineRange{}, ScopeMissing
	}
	return *chosen, ScopeVerified
}
