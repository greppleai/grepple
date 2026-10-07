package api_test

import (
	"context"
	"errors"
	"github.com/greppleai/grepple/api"
	"os"
	"path/filepath"
	"testing"
)

func TestSearchFilesContextCancellationAndCompatibility(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "sample.go")
	if err := os.WriteFile(file, []byte("package sample\n// needle\n"), 0600); err != nil {
		t.Fatal(err)
	}
	plan := api.NewSearchPlan(api.SearchPlanOptions{Query: "needle", Root: root, SkipSegments: true})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := api.SearchFilesContext(ctx, plan, []string{file}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	old, err := api.SearchFiles(plan, []string{file})
	if err != nil {
		t.Fatal(err)
	}
	current, err := api.SearchFilesContext(context.Background(), plan, []string{file})
	if err != nil || old.Len() != 1 || current.Len() != old.Len() {
		t.Fatalf("compatibility: %v", err)
	}
}
