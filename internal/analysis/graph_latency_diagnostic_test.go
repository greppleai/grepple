package analysis

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime/pprof"
	"testing"
	"time"

	"github.com/greppleai/grepple/internal/navigation"
)

func TestNavigationStoredLatencyDiagnostic(t *testing.T) {
	root := os.Getenv("GREPPLE_NAVIGATION_DIAGNOSTIC_ROOT")
	if root == "" {
		t.Skip("opt-in local corpus diagnostic")
	}
	t.Setenv("GREPPLE_NAVIGATION_MONOLITHIC", "1") // This historical diagnostic measures the complete core.
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(previous)
	output := os.Getenv("GREPPLE_NAVIGATION_DIAGNOSTIC_OUTPUT")
	data, err := os.ReadFile(filepath.Join(output, "paths.json"))
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	if err = json.Unmarshal(data, &paths); err != nil {
		t.Fatal(err)
	}
	sources := ReadSources(paths)
	request := GraphQuery{Direction: "callers", Depth: 2, At: os.Getenv("GREPPLE_NAVIGATION_DIAGNOSTIC_AT")}
	if _, err := BuildStoredGraph(sources, 0, request); err != nil {
		t.Fatal(err)
	}
	profile, err := os.Create(filepath.Join(output, "cpu.pprof"))
	if err != nil {
		t.Fatal(err)
	}
	defer profile.Close()
	if err = pprof.StartCPUProfile(profile); err != nil {
		t.Fatal(err)
	}
	defer pprof.StopCPUProfile()
	compareStoredTraversal(t, sources, request)
	for sample := 0; sample < 3; sample++ {
		measureStoredGraphStages(t, sources, request, sample)
	}
}
func measureStoredGraphStages(t *testing.T, sources []Source, request GraphQuery, sample int) {
	t.Helper()
	filter, err := navigation.NewGraphOperations().NormalizeFilter(navigation.NavigationGraphFilter{})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	key, path := storedGraphKey(sources, 0, filter)
	t.Logf("sample=%d stage=source_context_fence duration=%s", sample, time.Since(start))
	start = time.Now()
	snapshot, _, err := navigation.NewGraphOperations().SnapshotStore().Open(path, key)
	if err != nil {
		t.Fatal(err)
	}
	defer snapshot.Close()
	t.Logf("sample=%d stage=open_core_manifest duration=%s", sample, time.Since(start))
	start = time.Now()
	declarations, err := snapshot.Declarations()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("sample=%d stage=declaration_hydration duration=%s declarations=%d", sample, time.Since(start), len(declarations))
	start = time.Now()
	roots, err := selectGraphRoots(declarations, request)
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, len(roots))
	for _, root := range roots {
		ids = append(ids, root.ID)
	}
	t.Logf("sample=%d stage=root_selection duration=%s", sample, time.Since(start))
	start = time.Now()
	graph, err := snapshot.Query(ids, navigation.NavigationQueryDirection(request.Direction), request.Depth)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("sample=%d stage=traverse_hydrate duration=%s", sample, time.Since(start))
	start = time.Now()
	report := graphReport(graph, &Universe{})
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("sample=%d stage=report_json duration=%s bytes=%d", sample, time.Since(start), len(data))
}
