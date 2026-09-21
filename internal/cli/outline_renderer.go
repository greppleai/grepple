package cli

import (
	rendercommand "github.com/greppleai/grepple/internal/cli/render"
	"github.com/greppleai/grepple/parser"
)

// RenderOutline formats an outline as an indented, tab-separated tree.
func RenderOutline(outline parser.FileOutline) string { return rendercommand.Outline(outline) }

// RenderOutlineOrContent returns the compact outline or smaller raw content.
func RenderOutlineOrContent(outline parser.FileOutline, content string) string {
	return rendercommand.OutlineOrContent(outline, content)
}
