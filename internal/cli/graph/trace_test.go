package graph

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/greppleai/grepple/internal/cliruntime"
)

func TestGraphCLIPathResolutionUsesInvocationCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sample.go")
	if err := os.WriteFile(path, []byte("package p\nfunc Target() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if paths, err := ResolveInputPaths([]string{path}, nil); err != nil || len(paths) != 1 {
		t.Fatalf("legacy no-context resolver changed: %v %v", paths, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := resolveCLIPaths(cliruntime.Environment{InvocationScope: ctx}, []string{path})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled graph source resolution: %v", err)
	}
}
