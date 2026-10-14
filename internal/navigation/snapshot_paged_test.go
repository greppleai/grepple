package navigation

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/parser"
)

func assertPagedSnapshotParity(t *testing.T, graph parser.NavigationGraph) {
	t.Helper()
	prepared, err := newQuerySnapshot(graph)
	if err != nil {
		t.Fatal(err)
	}
	store := NewGraphOperations().SnapshotStore()
	path := filepath.Join(t.TempDir(), "paged.gnav")
	key := strings.Repeat("c", 64)
	if err := store.SavePaged(path, key, prepared, []byte("metadata")); err != nil {
		t.Fatal(err)
	}
	opened, metadata, err := store.OpenPaged(path, key)
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Close()
	if string(metadata) != "metadata" {
		t.Fatal("metadata changed")
	}
	got, err := opened.Graph()
	if err != nil || !reflect.DeepEqual(graph, got) {
		t.Fatalf("paged graph changed: %v", err)
	}
	for _, d := range []NavigationQueryDirection{NavigationQueryCallers, NavigationQueryCallees, NavigationQueryDependencies, NavigationQueryDependents, NavigationQueryImpact} {
		for depth := 1; depth <= 4; depth++ {
			for _, root := range graph.Declarations {
				assertSnapshotQueryParity(t, graph, opened, []string{root.ID, graph.Declarations[0].ID}, d, depth)
			}
		}
	}
}
func TestPagedSnapshotRichAndNilShapes(t *testing.T) {
	graph := navigationQueryFixture()
	graph.Declarations = append(graph.Declarations, graph.Declarations[0])
	duplicate := graph.Calls[0]
	duplicate.CallerID = "missing"
	duplicate.TargetID = "unknown"
	graph.Calls = append(graph.Calls, duplicate, parser.NavigationCall{ID: "direct-candidates", CallerID: "root", TargetID: "a", CandidateTargetIDs: []string{"extra", "extra"}}, parser.NavigationCall{ID: "empty", CallerID: "root", TargetID: "a", CandidateTargetIDs: []string{}})
	assertPagedSnapshotParity(t, graph)
	assertPagedSnapshotParity(t, parser.NavigationGraph{})
	assertPagedSnapshotParity(t, parser.NavigationGraph{Declarations: []parser.NavigationDeclaration{}, Calls: []parser.NavigationCall{}})
}
func TestPagedSnapshotSelectiveReadsCorruptionAndPinnedInode(t *testing.T) {
	graph := snapshotNeighborhoodFixture(20000)
	prepared, err := newQuerySnapshot(graph)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "paged.gnav")
	key := strings.Repeat("c", 64)
	store := NewGraphOperations().SnapshotStore()
	if err := store.SavePaged(path, key, prepared, nil); err != nil {
		t.Fatal(err)
	}
	opened, _, err := store.OpenPaged(path, key)
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Close()
	s := opened.(*pagedStoredSnapshot)
	r := newPagedQueryReader(s)
	nodes := r.keyRows("lookup", "0")
	if len(nodes) != 1 {
		t.Fatal("missing locator")
	}
	rows := r.rows("out", nodes[0])
	if len(rows) != 1 || r.err != nil {
		t.Fatal(r.err)
	}
	if r.read > 2<<20 {
		t.Fatalf("selective reads=%d", r.read)
	}
	t.Logf("selective numeric/locator bytes=%d of file", r.read)
	section := s.manifest.Sections["calls"]
	unused := section.Blocks[len(section.Blocks)-1]
	file, err := os.OpenFile(path, os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteAt([]byte{255}, s.offset+int64(unused.Offset)); err != nil {
		t.Fatal(err)
	}
	file.Close()
	assertSnapshotQueryParity(t, graph, opened, []string{"0"}, NavigationQueryCallees, 2)
	if _, err := opened.Graph(); err == nil {
		t.Fatal("corrupt page accepted during full scan")
	}
	if err := store.SavePaged(path, key, prepared, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := opened.Graph(); err == nil {
		t.Fatal("replacement changed pinned generation")
	}
	fresh, _, err := store.OpenPaged(path, key)
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Close()
	assertSnapshotQueryParity(t, graph, fresh, []string{"0"}, NavigationQueryCallees, 2)
	if _, _, err := store.OpenPaged(path, strings.Repeat("b", 64)); err == nil {
		t.Fatal("wrong revision accepted")
	}
	fresh.Close()
	if _, err := fresh.Query([]string{"0"}, NavigationQueryCallees, 2); err == nil {
		t.Fatal("closed lease accepted")
	}
}
func TestPagedSnapshotConcurrentQueries(t *testing.T) {
	graph := navigationQueryFixture()
	prepared, err := newQuerySnapshot(graph)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "paged.gnav")
	key := strings.Repeat("c", 64)
	store := NewGraphOperations().SnapshotStore()
	if err := store.SavePaged(path, key, prepared, nil); err != nil {
		t.Fatal(err)
	}
	opened, _, err := store.OpenPaged(path, key)
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Close()
	assertStoredSnapshotConcurrentQueries(t, opened)
}
