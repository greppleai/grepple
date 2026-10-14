package navigation

import (
	"github.com/greppleai/grepple/internal/parser"
	"os"
	"sort"
	"testing"
	"time"
)

func TestSnapshotLatencyMeasurement(t *testing.T) {
	if os.Getenv("GREPPLE_SNAPSHOT_LATENCY_MEASURE") == "" {
		t.Skip("opt-in latency measurement")
	}
	graph := snapshotNeighborhoodFixture(10000)
	snapshot, err := NewGraphOperations().PrepareQuery(graph)
	if err != nil {
		t.Fatal(err)
	}
	reference := func() (parser.NavigationGraph, error) {
		return queryNavigationGraph(graph, []string{"0"}, NavigationQueryCallees, 2)
	}
	compact := func() (parser.NavigationGraph, error) {
		return snapshot.Query([]string{"0"}, NavigationQueryCallees, 2)
	}
	diskSnapshot, err := NewGraphOperations().PrepareQuery(graph)
	if err != nil {
		t.Fatal(err)
	}
	diskReader := snapshotTestDiskDetails(t, diskSnapshot.(*querySnapshot))
	disk := func() (parser.NavigationGraph, error) {
		return diskSnapshot.Query([]string{"0"}, NavigationQueryCallees, 2)
	}
	for _, measurement := range []struct {
		name  string
		query func() (parser.NavigationGraph, error)
	}{{"legacy", reference}, {"compact", compact}, {"readat", disk}} {
		sampleSnapshotLatency(t, measurement.name, measurement.query)
	}
	t.Logf("disk_latency_detail_reads=%d disk_latency_detail_bytes=%d", diskReader.reads.Load(), diskReader.bytes.Load())
}

func sampleSnapshotLatency(t *testing.T, name string, query func() (parser.NavigationGraph, error)) {
	t.Helper()
	samples := make([]time.Duration, 200)
	for i := range samples {
		start := time.Now()
		if _, err := query(); err != nil {
			t.Fatal(err)
		}
		samples[i] = time.Since(start)
	}
	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
	t.Logf("mode=%s samples=%d p50=%s p95=%s", name, len(samples), samples[len(samples)/2], samples[190])
}
