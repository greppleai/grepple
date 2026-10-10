package analysis

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/greppleai/grepple/internal/navigation"
)

// Filesystem-warm isolated reader measurement: excludes discovery/source reads
// and revision fencing, includes root selection, selective hydration and query.
func TestLanguagePartitionLoadMeasurement(t *testing.T) {
	path := os.Getenv("GREPPLE_PARTITION_MEASURE_PATH")
	mode := os.Getenv("GREPPLE_PARTITION_MEASURE_MODE")
	at := os.Getenv("GREPPLE_PARTITION_MEASURE_AT")
	if path == "" || mode == "" || at == "" {
		t.Skip("opt-in reader measurement")
	}
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir("/workspace/grepple"); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(previous)
	key := strings.TrimSuffix(filepath.Base(path), ".gnav")
	filter, _ := navigation.NewGraphOperations().NormalizeFilter(navigation.NavigationGraphFilter{})
	request := GraphQuery{Direction: "callees", Depth: 2, At: at}
	durations := []time.Duration{}
	var bytes, allocations uint64
	for range 20 {
		runtime.GC()
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		start := time.Now()
		measureLanguageReader(t, mode, path, key, request, filter)
		durations = append(durations, time.Since(start))
		runtime.ReadMemStats(&after)
		bytes += after.TotalAlloc - before.TotalAlloc
		allocations += after.Mallocs - before.Mallocs
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	t.Logf("mode=%s median=%s p95=%s mean_bytes=%d mean_allocations=%d", mode, durations[10], durations[18], bytes/20, allocations/20)
}
