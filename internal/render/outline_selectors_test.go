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
