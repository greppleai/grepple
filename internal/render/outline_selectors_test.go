package render

import (
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/parser"
)

func TestOutlineWithSelectorsKeepsNativeKindsAndOmitsUnverifiedRanges(t *testing.T) {
	content := "function Run() {}\ntype Name = string\n"
	outline := parser.OutlineFile("sample.ts", content)
	got := OutlineWithSelectors(outline, content)
	if !strings.Contains(got, "Run @function:1-1\n") || !strings.Contains(got, "2-2\ttype\tName") {
		t.Fatalf("outline=%q", got)
	}
	if strings.Contains(got, "@type:") {
		t.Fatalf("invented selector: %q", got)
	}
}
func TestOutlineWithSelectorsDoesNotAttributeEnclosingInterfaceToMethod(t *testing.T) {
	content := "package p\ntype Widget interface { Run() }\n"
	outline := parser.OutlineFile("widget.go", content)
	functions := FilterOutline(outline, OutlineKinds{"functions": true})
	got := OutlineWithSelectors(functions, content)
	if !strings.Contains(got, "2-2\tmethod\tRun") || strings.Contains(got, "Run @interface:") {
		t.Fatalf("inline interface member inherited wrong selector: %q", got)
	}
}
