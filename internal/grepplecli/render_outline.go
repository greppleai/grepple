package grepplecli

import (
	"fmt"
	"strings"

	"grepple/internal/parser"
)

// RenderOutline formats an outline as an indented, tab-separated tree.
func RenderOutline(outline parser.FileOutline) string {
	var output strings.Builder
	output.WriteString(outline.Path)
	output.WriteByte('\t')
	output.WriteString(outline.Language)
	output.WriteByte('\n')
	renderSymbols(&output, outline.Symbols, 0)
	return output.String()
}

// RenderOutlineOrContent returns the human-mode outline for a file, or the raw
// file content when the outline would not be smaller. Both forms share the same
// path/language header. JSON output is unaffected.
func RenderOutlineOrContent(outline parser.FileOutline, content string) string {
	rendered := RenderOutline(outline)
	full := outline.Path + "\t" + outline.Language + "\n" + content
	if !strings.HasSuffix(full, "\n") {
		full += "\n"
	}
	if len(full) < len(rendered) {
		return full
	}
	return rendered
}

func renderSymbols(output *strings.Builder, symbols []parser.Symbol, depth int) {
	indent := strings.Repeat("  ", depth)
	for _, symbol := range symbols {
		fmt.Fprintf(output, "%s%d-%d\t%s\t%s\n", indent, symbol.Start, symbol.End, symbol.Kind, symbol.Name)
		if len(symbol.Children) > 0 {
			renderSymbols(output, symbol.Children, depth+1)
		}
	}
}
