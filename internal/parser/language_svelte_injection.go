package parser

import "strings"

// Embedded trees borrow the original source and are owned/closed by the host tree.
// Parsing only each body keeps memory linear in the component size; offsets and
// first-line columns are projected back to the original UTF-8 byte coordinates.
func svelteInjectBodies(tree *syntaxTree) error {
	tree.embedded = make(map[uintptr]*syntaxTree)
	tree.injectedErrors = make(map[uintptr]bool)
	var failure error
	tree.RootNode().WalkNamed(func(node *syntaxNode) {
		if failure != nil || (node.Kind() != "script_element" && node.Kind() != "style_element") {
			return
		}
		body := svelteNamedChild(node, "raw_text")
		if body == nil || body.StartByte() == body.EndByte() {
			return
		}
		language, reason := svelteBodyLanguage(node)
		if reason != "" {
			tree.diagnostics = append(tree.diagnostics, svelteUnsupportedDiagnostic(body, reason))
			svelteMarkEmbeddedError(tree, body)
			return
		}
		if language == "" {
			return
		}
		embedded, err := adapterForLanguage(language).Parse(body.Text())
		if err != nil {
			failure = err
			return
		}
		embedded.source, embedded.offset, embedded.origin = tree.source, body.StartByte(), body.StartPosition()
		embedded.language, embedded.parent = language, body
		tree.embedded[body.ID()] = embedded
		if embedded.RootNode().HasError() {
			svelteMarkEmbeddedError(tree, body)
		}
	})
	return failure
}

func svelteBodyLanguage(node *syntaxNode) (string, string) {
	tag := svelteNamedChild(node, "start_tag")
	if _, found := svelteLiteralAttribute(tag, "src"); found {
		return "", ""
	}
	lang, _ := svelteLiteralAttribute(tag, "lang")
	mime, _ := svelteLiteralAttribute(tag, "type")
	if node.Kind() == "style_element" {
		if (lang == "" || lang == "css") && (mime == "" || mime == "text/css") {
			return "css", ""
		}
		return "", "unsupported embedded style language"
	}
	if mime != "" && mime != "module" && mime != "text/javascript" && mime != "application/javascript" {
		return "", "unsupported embedded script type"
	}
	switch lang {
	case "", "js", "javascript":
		return "javascript", ""
	case "ts", "typescript":
		return "typescript", ""
	default:
		return "", "unsupported embedded script language"
	}
}

func svelteLiteralAttribute(tag *syntaxNode, name string) (string, bool) {
	if tag == nil {
		return "", false
	}
	for _, attribute := range tag.NamedChildren() {
		if attribute.Kind() != "attribute" || svelteChildText(attribute, "attribute_name") != name {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(attribute.Text(), name))
		if value == "" {
			return "", true
		}
		value = strings.TrimSpace(strings.TrimPrefix(value, "="))
		return strings.Trim(value, "\"'"), true
	}
	return "", false
}

func svelteUnsupportedDiagnostic(body *syntaxNode, reason string) ParseDiagnostic {
	lineStarts := sourceLineStarts(body.source)
	return ParseDiagnostic{Message: reason, Kind: "unsupported-embedded-language", Range: Range{
		StartByte: int(body.StartByte()), EndByte: int(body.EndByte()),
		Start: sourcePosition(body.source, lineStarts, int(body.StartByte())),
		End:   sourcePosition(body.source, lineStarts, int(body.EndByte())),
	}}
}

func svelteEmbeddedOutline(root *syntaxNode, content string) []Symbol {
	if embedded := root.injectionRoot(); embedded != nil {
		return adapterForLanguage(embedded.tree.language).Outline(embedded, content)
	}
	return nil
}

func svelteEmbeddedSegments(root *syntaxNode, content string, hits map[int]bool, rules *structureRules) []Segment {
	remaining := make(map[int]bool, len(hits))
	for line, hit := range hits {
		remaining[line] = hit
	}
	var segments []Segment
	root.WalkNamed(func(node *syntaxNode) {
		embedded := node.injectionRoot()
		if embedded == nil {
			return
		}
		selected := svelteSelectBodyHits(node, hits, remaining)
		if len(selected) == 0 {
			return
		}
		adapter := adapterForLanguage(embedded.tree.language)
		segments = append(segments, buildASTSegments(embedded, content, newMatchLines(selected), adapter.Rules())...)
		for _, tag := range node.Parent().NamedChildren() {
			if tag.Kind() == "start_tag" || tag.Kind() == "end_tag" {
				segments = append(segments, Segment{Kind: "lines", Start: tag.StartLine(), End: tag.EndLine()})
			}
		}
	})
	segments = append(segments, buildASTSegments(root, content, newMatchLines(remaining), rules)...)
	return mergeSegments(segments, len(splitLines(content)))
}

func svelteMarkEmbeddedError(tree *syntaxTree, body *syntaxNode) {
	for node := body; node != nil; node = node.Parent() {
		tree.injectedErrors[node.ID()] = true
	}
}
func svelteSelectBodyHits(node *syntaxNode, hits, remaining map[int]bool) map[int]bool {
	selected := make(map[int]bool)
	for line, hit := range hits {
		if hit && line >= node.StartLine() && line <= node.EndLine() {
			selected[line] = true
			delete(remaining, line)
		}
	}
	return selected
}
