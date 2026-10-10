package graph

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/greppleai/grepple/internal/analysis"
	"github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/search"
)

func TestNavigationDiscoveryLatencyDiagnostic(t *testing.T) {
	root := os.Getenv("GREPPLE_NAVIGATION_DIAGNOSTIC_ROOT")
	if root == "" {
		t.Skip("opt-in local corpus diagnostic")
	}
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(previous)
	start := time.Now()
	paths, err := ResolveInputPaths([]string{"."}, search.SourcePolicyConfigurer(cliruntime.Environment{}.Repository()))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("stage=discovery duration=%s files=%d", time.Since(start), len(paths))
	start = time.Now()
	sources := analysis.ReadSources(paths)
	t.Logf("stage=source_read duration=%s", time.Since(start))
	data, err := json.Marshal(paths)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(os.Getenv("GREPPLE_NAVIGATION_DIAGNOSTIC_OUTPUT"), "paths.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	_ = sources
}
