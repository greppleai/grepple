package search

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/greppleai/grepple/internal/workaccount"
)

func TestObservedScanCountsActualAttemptsNotWindowCandidates(t *testing.T) {
	root := t.TempDir()
	total := WorkerCount()*16 + 1
	for i := range total {
		path := filepath.Join(root, "file-"+strconv.Itoa(i)+".txt")
		if err := os.WriteFile(path, []byte("needle\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	ctx, work := workaccount.With(context.Background())
	matches, err := FilesContext(ctx, Params{Root: root, Query: "needle", SkipSegments: true, Limit: 1}, nil)
	if err != nil || len(matches) != 1 {
		t.Fatalf("window: %d, %v", len(matches), err)
	}
	got := work.Snapshot()
	if !got.CandidatesObserved || got.Candidates != int64(total) || got.Attempted == 0 || got.Attempted >= int64(total) || got.Read != got.Attempted || got.BytesRead != got.Read*7 || got.ParseObserved {
		t.Fatalf("window counts: %+v", got)
	}
}

func TestObservedReadsIncludeFailuresDuplicatesAndMetadataSelection(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "one.go")
	if err := os.WriteFile(file, []byte("needle\n"), 0600); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(root, "missing.go")
	ctx, work := workaccount.With(context.Background())
	results, err := FilesContext(ctx, Params{Root: root, Query: "needle", SkipSegments: true, MetadataCandidates: []string{file, missing, file}}, nil)
	if err != nil || len(results) != 2 {
		t.Fatalf("results: %d %v", len(results), err)
	}
	got := work.Snapshot()
	if got.CandidatesObserved || !got.SelectedObserved || got.Selected != 3 || got.Attempted != 3 || got.Read != 2 || got.ReadFailed != 1 || got.BytesRead != 14 {
		t.Fatalf("selected/actual counts: %+v", got)
	}
	ctx, listed := workaccount.With(context.Background())
	_, err = ListFilePathsContext(ctx, Params{Root: root, Limit: 1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	got = listed.Snapshot()
	if !got.CandidatesObserved || !got.SelectedObserved || got.Attempted != 0 || got.Read != 0 || got.BytesRead != 0 {
		t.Fatalf("listing read contents: %+v", got)
	}
}

func TestCancelledSearchDoesNotScheduleOrClaimWork(t *testing.T) {
	ctx, work := workaccount.With(context.Background())
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	_, err := FilesContext(ctx, Params{Root: t.TempDir(), Query: "needle"}, nil)
	if err != context.Canceled {
		t.Fatalf("expected cancellation, got %v", err)
	}
	got := work.Snapshot()
	if got.Attempted != 0 || got.CandidatesObserved || got.ParseObserved {
		t.Fatalf("cancelled before discovery but reported work: %+v", got)
	}
}
