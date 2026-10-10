package navigation

import (
	"github.com/greppleai/grepple/internal/parser"
	"os"
	"testing"
)

func TestSnapshotArtifactDifferentialParity(t *testing.T) {
	if os.Getenv("GREPPLE_SNAPSHOT_ARTIFACT") == "" {
		t.Skip("opt-in real artifact parity")
	}
	graph := snapshotMeasuredFixture(t)
	snapshot, err := NewGraphOperations().PrepareQuery(graph)
	if err != nil {
		t.Fatal(err)
	}
	if os.Getenv("GREPPLE_SNAPSHOT_DISK_DETAILS") == "1" {
		snapshotTestDiskDetails(t, snapshot.(*querySnapshot))
	}
	if len(graph.Declarations) == 0 {
		t.Fatal("artifact has no roots")
	}
	for _, direction := range []NavigationQueryDirection{NavigationQueryCallers, NavigationQueryCallees, NavigationQueryDependencies, NavigationQueryDependents, NavigationQueryImpact} {
		for _, row := range []int{0, len(graph.Declarations) / 2, len(graph.Declarations) - 1} {
			assertSnapshotQueryParity(t, graph, snapshot, []string{graph.Declarations[row].ID}, direction, 1)
		}
	}
	t.Logf("artifact parity declarations=%d calls=%d", len(graph.Declarations), len(graph.Calls))
}

func snapshotMeasuredFixture(t *testing.T) parser.NavigationGraph {
	t.Helper()
	path := os.Getenv("GREPPLE_SNAPSHOT_ARTIFACT")
	if path == "" {
		return snapshotMemoryFixture()
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := parser.UnmarshalNavigationFactArtifact(data)
	if err != nil {
		t.Fatal(err)
	}
	return artifact.Graph
}
