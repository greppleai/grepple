package api

import (
	"encoding/json"
	"github.com/greppleai/grepple/internal/navigation"
	"github.com/greppleai/grepple/internal/parser"
	"github.com/greppleai/grepple/internal/search"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

type codexScopedControl struct {
	Key, Path, File string
	Languages       []string
}

// Opt-in: reads a copied indexed artifact; never mutates live volumes/checkouts.
func TestIndexedCodexScopedProjection(t *testing.T) {
	dir, mode := os.Getenv("GREPPLE_CODEX_SCOPED_DIR"), os.Getenv("GREPPLE_CODEX_SCOPED_MODE")
	if dir == "" || mode == "" {
		t.Skip("opt-in indexed Codex comparison")
	}
	controlPath := filepath.Join(dir, "control.json")
	if mode == "build" {
		buildCodexScopedProjection(t, dir, controlPath)
		return
	}
	data, err := os.ReadFile(controlPath)
	if err != nil {
		t.Fatal(err)
	}
	var control codexScopedControl
	if err := json.Unmarshal(data, &control); err != nil {
		t.Fatal(err)
	}
	if mode == "parity" {
		compareCodexScopedProjection(t, dir, control)
		return
	}
	measureCodexScopedProjection(t, dir, mode, control)
}
func compareCodexScopedProjection(t *testing.T, dir string, control codexScopedControl) {
	t.Helper()
	legacy := readCodexProjection(t, filepath.Join(dir, "indexed-codex.gnav"), control.Key, control.File, false)
	paged := readCodexProjection(t, control.Path, control.Key, control.File, true)
	keepCodexContext(&legacy, paged)
	if !reflect.DeepEqual(legacy, paged) {
		left, right := reflect.ValueOf(legacy), reflect.ValueOf(paged)
		for i := 0; i < left.NumField(); i++ {
			if !reflect.DeepEqual(left.Field(i).Interface(), right.Field(i).Interface()) {
				t.Errorf("indexed Codex differs: field=%s", left.Type().Field(i).Name)
			}
		}
	}
	compareCodexEnrichment(t, dir, control)
	t.Logf("indexed Codex parity: declarations=%d calls=%d types=%d", len(paged.Declarations), len(paged.Calls), len(paged.TypeDeclarations))
}
func compareCodexEnrichment(t *testing.T, dir string, control codexScopedControl) {
	t.Helper()
	source, err := os.ReadFile(filepath.Join(dir, "exec.rs"))
	if err != nil {
		t.Fatal(err)
	}
	lines := map[int]bool{}
	for line := 1; line <= len(search.SplitLines(string(source))); line++ {
		lines[line] = true
	}
	batch := searchBatch{matches: []search.FileMatch{{File: snapshotCodexFile(control.File), Language: "rust", Content: string(source), MatchLines: lines}}}
	plan := NewSearchPlan(SearchPlanOptions{Root: "/workspace/openai/codex", Query: ".", Related: true, RelatedRepositoryContext: true, FollowRelated: 2})
	store := NewNavigationSnapshotStore()
	legacy, _, err := store.Open(filepath.Join(dir, "indexed-codex.gnav"), control.Key)
	if err != nil {
		t.Fatal(err)
	}
	defer legacy.Close()
	paged, _, err := store.OpenPaged(control.Path, control.Key)
	if err != nil {
		t.Fatal(err)
	}
	defer paged.Close()
	left, err := legacy.Attach(plan, batch, "/workspace/openai/codex", "/workspace")
	if err != nil {
		t.Fatal(err)
	}
	right, err := paged.Attach(plan, batch, "/workspace/openai/codex", "/workspace")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(left.Results(0, 0, true), right.Results(0, 0, true)) {
		t.Fatal("indexed Codex enriched result differs")
	}
	t.Log("indexed Codex enriched result parity (selected file content; external previews unavailable in isolated fixture)")
}
func snapshotCodexFile(file string) string {
	if filepath.IsAbs(file) {
		return file
	}
	return filepath.Join("/workspace", file)
}
func measureCodexScopedProjection(t *testing.T, dir, mode string, control codexScopedControl) {
	t.Helper()
	for sample := 0; sample < 5; sample++ {
		runtime.GC()
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		start := time.Now()
		path := filepath.Join(dir, "indexed-codex.gnav")
		if mode == "paged" {
			path = control.Path
		}
		graph := readCodexProjection(t, path, control.Key, control.File, mode == "paged")
		runtime.ReadMemStats(&after)
		t.Logf("mode=%s sample=%d elapsed=%s allocated=%d declarations=%d calls=%d types=%d", mode, sample, time.Since(start), after.TotalAlloc-before.TotalAlloc, len(graph.Declarations), len(graph.Calls), len(graph.TypeDeclarations))
		runtime.KeepAlive(graph)
	}
}
func buildCodexScopedProjection(t *testing.T, dir, controlPath string) {
	t.Helper()
	var identity struct{ Key string }
	data, err := os.ReadFile(filepath.Join(dir, "identity.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &identity); err != nil {
		t.Fatal(err)
	}
	artifact, _, err := NewNavigationSnapshotStore().OpenArtifact(filepath.Join(dir, "indexed-codex.gnav"), identity.Key)
	if err != nil {
		t.Fatal(err)
	}
	parts, err := PartitionNavigationArtifact(artifact)
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range parts {
		graph := part.Artifact.(indexedNavigation).graph
		t.Logf("partition=%v declarations=%d calls=%d", part.Languages, len(graph.Declarations), len(graph.Calls))
		if containsCodexLanguage(part.Languages, "rust") {
			publishCodexPartition(t, dir, controlPath, identity.Key, part)
		}
	}
}
func containsCodexLanguage(languages []string, wanted string) bool {
	for _, language := range languages {
		if language == wanted {
			return true
		}
	}
	return false
}
func publishCodexPartition(t *testing.T, dir, controlPath, key string, part NavigationPartition) {
	t.Helper()
	control := codexScopedControl{Key: key, Path: filepath.Join(dir, "rust-paged.gnav"), Languages: part.Languages}
	graph := part.Artifact.(indexedNavigation).graph
	for _, declaration := range graph.Declarations {
		if strings.HasSuffix(filepath.ToSlash(declaration.Path), "codex-rs/core/src/exec.rs") {
			control.File = declaration.Path
			break
		}
	}
	if control.File == "" {
		t.Fatal("indexed Codex exec.rs absent")
	}
	start := time.Now()
	if err := NewNavigationSnapshotStore().SavePaged(control.Path, key, part.Artifact, nil); err != nil {
		t.Fatal(err)
	}
	t.Logf("paged Rust publish=%s selected=%s", time.Since(start), control.File)
	data, err := json.Marshal(control)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(controlPath, data, 0600); err != nil {
		t.Fatal(err)
	}
}
func readCodexProjection(t *testing.T, path, key, file string, paged bool) parser.NavigationGraph {
	t.Helper()
	store := navigation.NewGraphOperations().SnapshotStore()
	var lease navigation.StoredSnapshot
	var err error
	if paged {
		lease, _, err = store.OpenPaged(path, key)
	} else {
		lease, _, err = store.Open(path, key)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	graph, err := lease.RelatedProjection([]string{file}, "/workspace", 2)
	if err != nil {
		t.Fatal(err)
	}
	return graph
}
func keepCodexContext(legacy *parser.NavigationGraph, paged parser.NavigationGraph) {
	ids := map[string]bool{}
	for _, d := range paged.Declarations {
		ids[d.ID] = true
	}
	declarations := legacy.Declarations[:0]
	for _, d := range legacy.Declarations {
		if ids[d.ID] {
			declarations = append(declarations, d)
		}
	}
	legacy.Declarations = declarations
	ids = map[string]bool{}
	for _, d := range paged.TypeDeclarations {
		encoded, _ := json.Marshal(d)
		ids[string(encoded)] = true
	}
	types := legacy.TypeDeclarations[:0]
	for _, d := range legacy.TypeDeclarations {
		encoded, _ := json.Marshal(d)
		if ids[string(encoded)] {
			types = append(types, d)
		}
	}
	legacy.TypeDeclarations = types
}
