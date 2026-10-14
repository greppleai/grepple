package navigation

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestPagedBudgetSentinelAndNoCoreReadsOnOpen(t *testing.T) {
	graph := navigationQueryFixture()
	snapshot, err := newQuerySnapshot(graph)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "paged.gnav")
	key := strings.Repeat("a", 64)
	store := NewGraphOperations().SnapshotStore()
	if err := store.SavePaged(path, key, snapshot, nil); err != nil {
		t.Fatal(err)
	}
	opened, _, err := store.OpenPaged(path, key)
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Close()
	stats := opened.ReadStats()
	if stats.CoreBytesRead != 0 || stats.CorePagesRead != 0 || stats.DirectoryBytes == 0 {
		t.Fatal("open hydrated numeric pages")
	}
	reader := newPagedQueryReader(opened.(*pagedStoredSnapshot))
	reader.read = pagedReadBudget
	reader.number("calls", 0)
	if !errors.Is(reader.err, ErrSnapshotBudget) {
		t.Fatalf("budget error lost: %v", reader.err)
	}
	if _, err := opened.Query([]string{"root"}, NavigationQueryCallees, 2); err != nil {
		t.Fatal(err)
	}
	if opened.ReadStats().CoreBytesRead == 0 {
		t.Fatal("query read metrics missing")
	}
}
