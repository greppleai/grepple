package navigation

import (
	"os"
	"runtime"
	"sort"
	"testing"
	"time"
)

// Isolates persisted-reader cost from CLI source discovery/root selection fences.
func TestPagedStoredReadMeasurement(t *testing.T) {
	path, key, root, mode := os.Getenv("GREPPLE_PAGED_MEASURE_PATH"), os.Getenv("GREPPLE_PAGED_MEASURE_KEY"), os.Getenv("GREPPLE_PAGED_MEASURE_ROOT"), os.Getenv("GREPPLE_PAGED_MEASURE_MODE")
	if path == "" || key == "" || root == "" {
		t.Skip("opt-in persisted-reader measurement")
	}
	times := []time.Duration{}
	var bytes, allocs uint64
	var stats PagedReadStats
	for range 20 {
		runtime.GC()
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		start := time.Now()
		stats = measurePagedStoredReader(t, path, key, root, mode)
		times = append(times, time.Since(start))
		runtime.ReadMemStats(&after)
		bytes += after.TotalAlloc - before.TotalAlloc
		allocs += after.Mallocs - before.Mallocs
	}
	sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })
	t.Logf("mode=%s median=%s allocated_bytes=%d allocations=%d directory_bytes=%d core_bytes_read=%d core_pages=%d", mode, times[10], bytes/20, allocs/20, stats.DirectoryBytes, stats.CoreBytesRead, stats.CorePagesRead)
}
func measurePagedStoredReader(t *testing.T, path, key, root, mode string) PagedReadStats {
	t.Helper()
	store := NewGraphOperations().SnapshotStore()
	var lease SnapshotLease
	var err error
	if mode == "paged" || mode == "paged-full" {
		lease, _, err = store.OpenPaged(path, key)
	} else {
		lease, _, err = store.Open(path, key)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	if mode == "paged-full" || mode == "eager-full" {
		if _, err := lease.Graph(); err != nil {
			t.Fatal(err)
		}
	} else {
		if _, err := lease.Query([]string{root}, NavigationQueryCallees, 2); err != nil {
			t.Fatal(err)
		}
	}
	if paged, ok := lease.(PagedSnapshot); ok {
		return paged.ReadStats()
	}
	return PagedReadStats{}
}
