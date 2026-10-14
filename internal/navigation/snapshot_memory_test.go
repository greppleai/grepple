package navigation

import (
	"fmt"
	"os"
	"reflect"
	"runtime"
	"runtime/pprof"
	"strings"
	"testing"
	"time"

	"github.com/greppleai/grepple/internal/parser"
)

func snapshotMemoryFixture() parser.NavigationGraph {
	graph := parser.NavigationGraph{}
	for n := 0; n < 10000; n++ {
		id := fmt.Sprintf("repo/pkg/%d.go:function:%d", n, n)
		graph.Declarations = append(graph.Declarations, parser.NavigationDeclaration{ID: id, Name: fmt.Sprintf("method%d", n), Path: fmt.Sprintf("pkg/%d.go", n%200), Language: "go", Start: 1, End: 4, Signature: "func method(context.Context) (string,error)"})
	}
	for n := 0; n < 100000; n++ {
		caller := graph.Declarations[n%10000]
		target := graph.Declarations[(n+1)%10000]
		graph.Calls = append(graph.Calls, parser.NavigationCall{ID: fmt.Sprintf("%s:call:%d", caller.ID, n), CallerID: caller.ID, TargetID: target.ID, Name: target.Name, Display: target.Name + "(context)", Language: "go", Path: caller.Path, Line: n%30 + 1, Confidence: "context-resolved", ReceiverRootType: "Service", ReceiverType: "Service", ReceiverRootImport: "repo/pkg/service", ReceiverFactory: "NewService", ReceiverMembers: []string{"service", "handler"}, ImportPath: "repo/pkg/service", Qualifier: "service", ResolvedName: target.Name})
	}
	return graph
}

// Separate processes avoid attributing retained fixture memory to the snapshot.
// Opt-in only; the profile path is supplied explicitly and contains no queries.
func TestSnapshotHeapMeasurement(t *testing.T) {
	mode := os.Getenv("GREPPLE_SNAPSHOT_MEASURE")
	if mode == "" {
		t.Skip("opt-in isolated heap measurement")
	}
	runtime.GC()
	var baseline runtime.MemStats
	runtime.ReadMemStats(&baseline)
	began := time.Now()
	graph := snapshotMeasuredFixture(t)
	declarations, calls := len(graph.Declarations), len(graph.Calls)
	buildTime := time.Since(began)
	var holder any = graph
	if mode == "compact" || mode == "readat" {
		began = time.Now()
		snapshot, err := NewGraphOperations().PrepareQuery(graph)
		if err != nil {
			t.Fatal(err)
		}
		if mode == "readat" {
			snapshotTestDiskDetails(t, snapshot.(*querySnapshot))
		}
		holder = snapshot
		graph = parser.NavigationGraph{}
		buildTime = time.Since(began)
	} else if mode != "legacy" {
		t.Fatal("invalid measurement mode")
	}
	runtime.GC()
	var measured runtime.MemStats
	runtime.ReadMemStats(&measured)
	t.Logf("mode=%s declarations=%d calls=%d retained_heap=%d conversion_or_fixture=%s", mode, declarations, calls, measured.HeapAlloc-baseline.HeapAlloc, buildTime)
	if path := os.Getenv("GREPPLE_SNAPSHOT_PROFILE"); path != "" {
		file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
		if err != nil {
			t.Fatal(err)
		}
		if err := pprof.WriteHeapProfile(file); err != nil {
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
	}
	runtime.KeepAlive(holder)
}

func TestSnapshotDetachedFromInputAndCorruptionFails(t *testing.T) {
	graph := navigationQueryFixture()
	expected, err := queryNavigationGraph(graph, []string{"root"}, NavigationQueryCallees, 1)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := NewGraphOperations().PrepareQuery(graph)
	if err != nil {
		t.Fatal(err)
	}
	graph.Calls[0].TargetID = "tampered"
	graph.Declarations[0].Name = "tampered"
	actual, err := snapshot.Query([]string{"root"}, NavigationQueryCallees, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(actual, expected) {
		t.Fatal("snapshot borrowed mutable input")
	}
	internal := snapshot.(*querySnapshot)
	internal.details.calls.blocks[0][len(internal.details.calls.blocks[0])-1] ^= 1
	if _, err := snapshot.Query([]string{"root"}, NavigationQueryCallees, 1); err == nil {
		t.Fatal("corrupt details returned a partial graph")
	}
}

func TestSnapshotContainsNoDecodedBuilderModel(t *testing.T) {
	typ := reflect.TypeOf(querySnapshot{})
	for n := 0; n < typ.NumField(); n++ {
		field := typ.Field(n)
		if strings.Contains(field.Type.String(), "NavigationGraph") || strings.Contains(field.Type.String(), "navigationIndex") {
			t.Fatalf("retained builder field %s", field.Name)
		}
	}
}
