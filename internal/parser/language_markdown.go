package parser

import (
	"strings"

	markdown "github.com/Bitspark/tree-sitter-markdown/bindings/go"
)

// markdownLanguage parses the Markdown block grammar. It is used for heading,
// outline, and segment structure and is not a code-navigation language.
type markdownLanguage struct {
	grammar syntaxLanguage
	rules   structureRules
}

func newMarkdownLanguage() languageAdapter {
	return &markdownLanguage{
		grammar: newSyntaxLanguage(markdown.Language()),
		rules: structureRules{
			structuralTypes:       newStringSet("atx_heading", "setext_heading"),
			contextTypes:          newStringSet("section"),
			containerTypes:        newStringSet("section"),
			classDeclarationTypes: newStringSet(),
			classBodyTypes:        newStringSet(),
			exportTypes:           newStringSet(),
			functionLikeTypes:     newStringSet(),
			nameFieldCandidates:   newStringSet("inline", "paragraph"),
		},
	}
}

func (*markdownLanguage) ID() string                       { return "markdown" }
func (language *markdownLanguage) Grammar() syntaxLanguage { return language.grammar }
func (language *markdownLanguage) Parse(content string) (*syntaxTree, error) {
	return parseSyntaxTree(language.grammar, content)
}
func (language *markdownLanguage) Rules() *structureRules { return &language.rules }
func (*markdownLanguage) BuildSegments(root *syntaxNode, content string, hits map[int]bool) []Segment {
	return buildMarkdownSegments(root, content, hits)
}
func (*markdownLanguage) Navigation() navigationAdapter { return nil }
func (language *markdownLanguage) Outline(root *syntaxNode, content string) []Symbol {
	var heads []mdHeading
	collectMarkdownHeadings(root, &heads)
	if len(heads) == 0 {
		return nil
	}
	return mdTree(heads, markdownHeadingEnds(heads, strings.Count(content, "\n")+1))
}
