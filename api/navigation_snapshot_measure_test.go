package api

import (
	"os"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestNavigationSnapshotArtifactReloadMeasurement(t *testing.T) {
	input := os.Getenv("GREPPLE_SNAPSHOT_INPUT")
	output := os.Getenv("GREPPLE_SNAPSHOT_OUTPUT")
	mode := os.Getenv("GREPPLE_SNAPSHOT_RELOAD_MODE")
	if input == "" || output == "" || mode == "" {
		t.Skip("opt-in preserved artifact measurement")
	}
	key := strings.Repeat("a", 64)
	store := NewNavigationSnapshotStore()
	if mode == "build" {
		content, err := os.ReadFile(input)
		if err != nil {
			t.Fatal(err)
		}
		graph, err := DecodeNavigationArtifact(content)
		if err != nil {
			t.Fatal(err)
		}
		started := time.Now()
		if err := store.Save(output, key, graph, nil); err != nil {
			t.Fatal(err)
		}
		t.Logf("snapshot_publish=%s", time.Since(started))
		return
	}
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	started := time.Now()
	var snapshot NavigationSnapshot
	var err error
	if mode == "declarations" {
		snapshot, _, err = store.OpenDeclarations(output, key)
	} else {
		snapshot, _, err = store.Open(output, key)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer snapshot.Close()
	opened := time.Since(started)
	index, err := snapshot.DeclarationIndex()
	if err != nil {
		t.Fatal(err)
	}
	runtime.ReadMemStats(&after)
	declarations, types := 0, 0
	index.VisitDeclarations(func(ArtifactDeclaration) { declarations++ })
	index.VisitTypeDeclarations(func(ArtifactTypeDeclaration) { types++ })
	t.Logf("reload=%s open=%s allocated=%d declarations=%d types=%d", time.Since(started), opened, after.TotalAlloc-before.TotalAlloc, declarations, types)
	runtime.KeepAlive(index)
}
