package navigation

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSnapshotDeclarationLeaseDoesNotLoadCore(t *testing.T) {
	graph := navigationQueryFixture()
	prepared, err := newQuerySnapshot(graph)
	if err != nil {
		t.Fatal(err)
	}
	store := NewGraphOperations().SnapshotStore()
	path := filepath.Join(t.TempDir(), "query.gnav")
	key := strings.Repeat("a", 64)
	if err = store.Save(path, key, prepared, nil); err != nil {
		t.Fatal(err)
	}
	lease, _, err := store.OpenDeclarations(path, key)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	if lease.(*storedSnapshot).snapshot.ids != nil {
		t.Fatal("declaration lease decoded adjacency core")
	}
	declarations, err := lease.Declarations()
	if err != nil || !reflect.DeepEqual(declarations, graph.Declarations) {
		t.Fatalf("declarations changed: %v", err)
	}
	if _, err := lease.Graph(); err == nil {
		t.Fatal("declaration lease permitted full graph")
	}
	if _, err := lease.RelatedProjection(nil, "", 1); err == nil {
		t.Fatal("declaration lease permitted traversal")
	}
}
