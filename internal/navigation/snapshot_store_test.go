package navigation

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestSnapshotStoreReopenLifetimeAndParity(t *testing.T) {
	graph := navigationQueryFixture()
	prepared, err := newQuerySnapshot(graph)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "graph.gnav")
	key := strings.Repeat("a", 64)
	store := NewGraphOperations().SnapshotStore()
	if err := store.Save(path, key, prepared, []byte("metadata")); err != nil {
		t.Fatal(err)
	}
	opened, metadata, err := store.Open(path, key)
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Close()
	if string(metadata) != "metadata" {
		t.Fatal("metadata changed")
	}
	actual, err := opened.Graph()
	if err != nil || !reflect.DeepEqual(graph, actual) {
		t.Fatalf("full graph changed: %v", err)
	}
	if _, _, err := store.Open(path, strings.Repeat("b", 64)); err == nil {
		t.Fatal("wrong revision accepted")
	}
	if err := store.Save(path, key, prepared, nil); err != nil {
		t.Fatal(err)
	}
	assertStoredSnapshotConcurrentQueries(t, opened)
	if err := opened.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := opened.Graph(); err == nil {
		t.Fatal("closed generation accepted")
	}
	assertStoredSnapshotCorruption(t, store, path, key)

}

func assertStoredSnapshotConcurrentQueries(t *testing.T, opened QuerySnapshot) {
	t.Helper()
	var group sync.WaitGroup
	for i := 0; i < 4; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			for j := 0; j < 4; j++ {
				if _, err := opened.Query([]string{"root"}, NavigationQueryCallees, 2); err != nil {
					t.Error(err)
				}
			}
		}()
	}
	group.Wait()
}

func assertStoredSnapshotCorruption(t *testing.T, store SnapshotStore, path, key string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data[len(data)-1] ^= 1
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	corrupt, _, err := store.Open(path, key)
	if err != nil {
		t.Fatal(err)
	}
	defer corrupt.Close()
	if _, err := corrupt.Graph(); err == nil {
		t.Fatal("corrupted details accepted")
	}
}
