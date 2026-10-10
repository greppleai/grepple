package analysis

import (
	"reflect"
	"sync"
	"testing"

	"github.com/greppleai/grepple/internal/parser"
)

func TestLanguageGenerationConcurrentPublication(t *testing.T) {
	t.Setenv(parser.NavigationCacheDirectoryEnv, t.TempDir())
	sources := mixedLanguageSources()
	request := GraphQuery{Symbol: "pyLeaf", Direction: "callers", Depth: 2}
	universe, err := NewUniverse(sources, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer universe.Close()
	want, err := BuildGraph(universe, &request)
	if err != nil {
		t.Fatal(err)
	}
	var group sync.WaitGroup
	for range 8 {
		group.Go(func() {
			got, err := BuildStoredGraph(sources, 0, request)
			if err != nil {
				t.Error(err)
				return
			}
			if !reflect.DeepEqual(want, got) {
				t.Error("concurrent language generation changed projection")
			}
		})
	}
	group.Wait()
	assertLanguageGraphParity(t, sources, universe, request)
}
