package analysis

import (
	"reflect"
	"testing"
	"time"

	"github.com/greppleai/grepple/internal/navigation"
)

func compareStoredTraversal(t *testing.T, sources []Source, request GraphQuery) {
	t.Helper()
	operations := navigation.NewGraphOperations()
	filter, err := operations.NormalizeFilter(navigation.NavigationGraphFilter{})
	if err != nil {
		t.Fatal(err)
	}
	key, path := storedGraphKey(sources, 0, filter)
	snapshot, _, err := operations.SnapshotStore().Open(path, key)
	if err != nil {
		t.Fatal(err)
	}
	defer snapshot.Close()
	graph, err := snapshot.Graph()
	if err != nil {
		t.Fatal(err)
	}
	roots, err := selectGraphRoots(graph.Declarations, request)
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, len(roots))
	for _, root := range roots {
		ids = append(ids, root.ID)
	}
	for sample := 0; sample < 3; sample++ {
		start := time.Now()
		reference, err := operations.Query(graph, ids, navigation.NavigationQueryDirection(request.Direction), request.Depth)
		if err != nil {
			t.Fatal(err)
		}
		referenceTime := time.Since(start)
		start = time.Now()
		actual, err := snapshot.Query(ids, navigation.NavigationQueryDirection(request.Direction), request.Depth)
		if err != nil {
			t.Fatal(err)
		}
		compactTime := time.Since(start)
		if !reflect.DeepEqual(reference, actual) {
			t.Fatal("traversal parity failure")
		}
		t.Logf("sample=%d query_only_reference=%s query_only_stored=%s declarations=%d calls=%d", sample, referenceTime, compactTime, len(actual.Declarations), len(actual.Calls))
	}
}
