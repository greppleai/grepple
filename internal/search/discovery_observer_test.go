package search

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCandidateDiscoveryObserverIsAggregateAndExplicit(t *testing.T) {
	root := t.TempDir()
	var candidates []string
	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, []byte("needle\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		candidates = append(candidates, path)
	}
	calls, count := 0, 0
	ctx := WithCandidateDiscoveryObserver(context.Background(), func(mode string) func(int, error) {
		if mode != "content" {
			t.Errorf("unexpected discovery mode %s", mode)
		}
		calls++
		return func(value int, err error) {
			if err != nil {
				t.Error(err)
			}
			count = value
		}
	})
	params := Params{Query: "needle", Globs: candidates}
	if _, err := FilesContext(ctx, params, candidates); err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatal("supplied candidates unexpectedly triggered discovery")
	}
	if _, err := FilesContext(ctx, params, nil); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || count != len(candidates) {
		t.Fatalf("discovery calls=%d, candidates=%d", calls, count)
	}
}

func TestCandidateDiscoveryObserverReportsCancellation(t *testing.T) {
	root := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls, finishes := 0, 0
	ctx = WithCandidateDiscoveryObserver(ctx, func(mode string) func(int, error) {
		if mode != "listing" {
			t.Errorf("unexpected listing mode %s", mode)
		}
		calls++
		cancel()
		return func(_ int, err error) {
			finishes++
			if !errors.Is(err, context.Canceled) {
				t.Errorf("discovery cancellation: %v", err)
			}
		}
	})
	if _, err := ListFilePathsContext(ctx, Params{Files: true, Globs: []string{root}}, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("listing cancellation: %v", err)
	}
	if calls != 1 || finishes != 1 {
		t.Fatalf("observer callbacks: start=%d, finish=%d", calls, finishes)
	}
}
