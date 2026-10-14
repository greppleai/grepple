package api_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/greppleai/grepple/api"
)

func TestPublicLocalScopeSelector(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "sample.go")
	if err := os.WriteFile(path, []byte("package p\nfunc f() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	plan := api.NewSearchPlan(api.SearchPlanOptions{
		At: path, Root: root,
		AtScope: &api.ScopeSelector{Kind: "func", StartLine: 2, EndLine: 2, Exact: true},
	})
	batch, err := api.SearchAt(plan)
	if err != nil || batch.Len() != 1 {
		t.Fatalf("external API scope result=%v error=%v", batch, err)
	}
}
