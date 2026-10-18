package analysis

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/greppleai/grepple/internal/workaccount"
)

func TestReadSourcesAndUniverseCountDifferentActualGates(t *testing.T) {
	dir := t.TempDir()
	paths := []string{filepath.Join(dir, "a.go"), filepath.Join(dir, "b.go"), filepath.Join(dir, "c.txt"), filepath.Join(dir, "missing.go")}
	data := []byte("package sample\nfunc One() {}\n")
	for _, path := range paths[:2] {
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(paths[2], []byte("plain\n"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, work := workaccount.With(context.Background())
	sources := ReadSourcesContext(ctx, paths)
	before := work.Snapshot()
	if before.Attempted != 4 || before.Read != 3 || before.ReadFailed != 1 || before.BytesRead != int64(len(data)*2+6) || before.ParseObserved {
		t.Fatalf("reads counted as parsing: %+v", before)
	}
	universe, err := NewUniverseContext(ctx, sources, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer universe.Close()
	after := work.Snapshot()
	if !after.ParseObserved || after.ParseAttempted != 1 || after.Parsed != 1 || after.ParseFailed != 0 || after.Read != 3 || after.Attempted != 4 {
		t.Fatalf("max-files gate applied to reads or fake parses: %+v", after)
	}
	if universe.Summary().Discovered != 4 || universe.Summary().Selected != 1 {
		t.Fatalf("universe summary: %+v", universe.Summary())
	}
}
