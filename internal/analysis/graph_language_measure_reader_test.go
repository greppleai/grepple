package analysis

import (
	"github.com/greppleai/grepple/internal/navigation"
	"testing"
)

func measureLanguageReader(t *testing.T, mode, path, key string, request GraphQuery, filter navigation.NavigationGraphFilter) {
	t.Helper()
	if mode == "split" {
		manifest, err := readLanguageManifest(path, key)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := queryLanguageManifest(manifest, path, key, request, filter); err != nil {
			t.Fatal(err)
		}
		return
	}
	snapshot, metadata, err := navigation.NewGraphOperations().SnapshotStore().Open(path, key)
	if err != nil {
		t.Fatal(err)
	}
	defer snapshot.Close()
	if _, err := queryStoredGraph(snapshot, metadata, request, filter); err != nil {
		t.Fatal(err)
	}
}
