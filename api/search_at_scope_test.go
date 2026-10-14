package api

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/greppleai/grepple/internal/parser"
)

func TestSearchPlanScopeSelectorIsClonedAndLocal(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "example.go")
	if err := os.WriteFile(path, []byte("package p\nfunc run() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	selector := parser.ScopeSelector{Kind: "func", StartLine: 2, EndLine: 2, Exact: true}
	plan := NewSearchPlan(SearchPlanOptions{At: path, Root: root, AtScope: &selector})
	selector.Kind = "if"
	options := plan.Options()
	if options.AtScope == nil || options.AtScope.Kind != "func" {
		t.Fatalf("plan selector=%+v", options.AtScope)
	}
	options.AtScope.Kind = "class"
	batch, err := SearchAt(plan)
	if err != nil || batch.Len() != 1 {
		t.Fatalf("batch=%+v error=%v", batch, err)
	}
}
