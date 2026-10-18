package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/greppleai/grepple/internal/daemon"
	"github.com/greppleai/grepple/internal/parser"
	spanpb "go.opentelemetry.io/proto/otlp/trace/v1"
)

func assertNavigationPhases(t *testing.T, spans []*spanpb.Span, command, cacheState string, expectRender bool) {
	t.Helper()
	assertCLIPhaseContract(t, spans)
	byName := make(map[string]*spanpb.Span)
	counts := make(map[string]int)
	for _, span := range spans {
		counts[span.Name]++
		byName[span.Name] = span
		if len(span.Events) != 0 {
			t.Fatalf("unexpected events on %s", span.Name)
		}
		// The shared contract already checks each span's exact attributes.
	}
	wantBuild := command == "graph.resolve" && cacheState != "hit" && expectRender
	wantRead := cacheState != "hit"
	if counts["cli.invocation"] != 1 || counts["cli.output_spill"] != 1 || counts["cli.command"] != 1 || counts["cli.sources.resolve"] != 1 || counts["cli.navigation.execute"] != 1 || counts["cli.render"] > 1 || counts["cli.navigation.cache_lookup"] > 1 || (counts["cli.navigation.read"] == 1) != wantRead || (counts["cli.navigation.build"] == 1) != wantBuild || len(spans) != 5+counts["cli.navigation.cache_lookup"]+counts["cli.render"]+counts["cli.navigation.read"]+counts["cli.navigation.build"] || len(spans) > 9 {
		t.Fatalf("unbounded or missing navigation phases: %+v", counts)
	}
	if wantRead && (!bytes.Equal(byName["cli.navigation.read"].ParentSpanId, byName["cli.navigation.execute"].SpanId) || byName["cli.navigation.read"].StartTimeUnixNano < byName["cli.navigation.execute"].StartTimeUnixNano || byName["cli.navigation.read"].EndTimeUnixNano > byName["cli.navigation.execute"].EndTimeUnixNano) {
		t.Fatal("source read batch not bounded by navigation execution")
	}
	if wantBuild && (!bytes.Equal(byName["cli.navigation.build"].ParentSpanId, byName["cli.navigation.execute"].SpanId) || byName["cli.navigation.build"].StartTimeUnixNano < byName["cli.navigation.execute"].StartTimeUnixNano || byName["cli.navigation.build"].EndTimeUnixNano > byName["cli.navigation.execute"].EndTimeUnixNano) {
		t.Fatal("graph resolve build stage not bounded by navigation execution")
	}
	if wantBuild && byName["cli.navigation.read"].EndTimeUnixNano > byName["cli.navigation.build"].StartTimeUnixNano {
		t.Fatal("graph resolve build started before source reading completed")
	}
	if expectRender != (counts["cli.render"] == 1) || expectRender && (!bytes.Equal(byName["cli.render"].ParentSpanId, byName["cli.navigation.execute"].SpanId) || spanAttributes(byName["cli.render"])["cli.render.format"].GetStringValue() != "json") {
		t.Fatalf("wrong graph render phase: %+v", counts)
	}
	if spanAttributes(byName["cli.command"])["cli.command.name"].GetStringValue() != command || !bytes.Equal(byName["cli.navigation.execute"].ParentSpanId, byName["cli.command"].SpanId) || !bytes.Equal(byName["cli.sources.resolve"].ParentSpanId, byName["cli.command"].SpanId) || !bytes.Equal(byName["cli.command"].TraceId, byName["cli.invocation"].TraceId) {
		t.Fatalf("wrong graph category or parents: %+v", counts)
	}
	if cacheState == "" {
		if counts["cli.navigation.cache_lookup"] != 0 {
			t.Fatal("daemon span on non-daemon invocation")
		}
	} else if counts["cli.navigation.cache_lookup"] != 1 || !bytes.Equal(byName["cli.navigation.cache_lookup"].ParentSpanId, byName["cli.navigation.execute"].SpanId) || spanAttributes(byName["cli.navigation.cache_lookup"])["cli.cache.kind"].GetStringValue() != "daemon" || spanAttributes(byName["cli.navigation.cache_lookup"])["cli.cache.state"].GetStringValue() != cacheState {
		t.Fatalf("wrong daemon cache outcome: want %s got %s kind %s, counts %+v", cacheState, spanAttributes(byName["cli.navigation.cache_lookup"])["cli.cache.state"].GetStringValue(), spanAttributes(byName["cli.navigation.cache_lookup"])["cli.cache.kind"].GetStringValue(), counts)
	}
}

func TestCLINavigationColdWarmAndBoundedInput(t *testing.T) {
	capture, _, _ := startPhaseCapture(t)
	root := t.TempDir()
	navigationCache := filepath.Join(t.TempDir(), "navigation")
	t.Setenv(parser.NavigationCacheDirectoryEnv, navigationCache)
	t.Setenv("GREPPLE_CACHE_DIR", filepath.Join(t.TempDir(), "daemon-cache"))
	privatePath := filepath.Join(root, "private-graph-fixture.go")
	if err := os.WriteFile(privatePath, []byte("package sample\nfunc Root(){ Target() }\nfunc Target() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 18; i++ {
		path := filepath.Join(root, "extra-"+string(rune('a'+i))+".go")
		if err := os.WriteFile(path, []byte("package sample\nfunc Extra"+string(rune('A'+i))+"() {}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var cold string
	for _, paths := range []string{privatePath, privatePath, root} {
		capture.reset()
		text := captureStdout(t, func() {
			if err := Run([]string{"--no-repo-config", "graph", "callers", "--json", "--symbol", "Target", paths}); err != nil {
				t.Fatal(err)
			}
		})
		if paths == privatePath {
			if cold != "" && text != cold {
				t.Fatal("warm stored graph changed output")
			}
			if cold == "" {
				entries, err := os.ReadDir(filepath.Join(navigationCache, "query-ready"))
				if err != nil {
					t.Fatal("cold graph did not publish a query-ready snapshot: ", err)
				}
				published := false
				for _, entry := range entries {
					published = published || strings.HasSuffix(entry.Name(), ".gnav.languages.json")
				}
				if !published {
					t.Fatal("cold graph did not publish a language manifest")
				}
			}
			cold = text
		}
		spans, payload := capture.snapshot()
		assertNavigationPhases(t, spans, "graph.callers", "", true)
		if bytes.Contains(payload, []byte(privatePath)) || bytes.Contains(payload, []byte("Target")) || bytes.Contains(payload, []byte(root)) {
			t.Fatal("private source/selector appeared in exported graph spans")
		}
	}
	// Traversal depth may multiply visited declarations but not CLI phases.
	for _, depth := range []string{"1", "4", "10"} {
		capture.reset()
		captureStdout(t, func() {
			if err := Run([]string{"--no-repo-config", "graph", "callers", "--json", "--symbol", "Target", "--depth", depth, root}); err != nil {
				t.Fatal(err)
			}
		})
		depthSpans, _ := capture.snapshot()
		assertNavigationPhases(t, depthSpans, "graph.callers", "", true)
	}
	capture.reset()
	captureStdout(t, func() {
		if err := Run([]string{"--no-repo-config", "graph", "resolve", "--json", "--symbol", "Target", privatePath}); err != nil {
			t.Fatal(err)
		}
	})
	spans, _ := capture.snapshot()
	assertNavigationPhases(t, spans, "graph.resolve", "", true)

	capture.reset()
	if err := Run([]string{"--no-repo-config", "graph", "callers", "--symbol", "Target", "--confidence", "invalid-confidence", privatePath}); err == nil {
		t.Fatal("invalid graph filter unexpectedly succeeded")
	}
	spans, _ = capture.snapshot()
	assertNavigationPhases(t, spans, "graph.callers", "", false)
	for _, span := range spans {
		if span.Name == "cli.navigation.execute" && spanAttributes(span)["cli.phase.outcome"].GetStringValue() != "error" {
			t.Fatal("negative path did not record a fixed error outcome")
		}
	}
}

func TestCLIDaemonLookupMissThenHit(t *testing.T) {
	capture, _, _ := startPhaseCapture(t)
	root := t.TempDir()
	cache := filepath.Join(t.TempDir(), "daemon-cache")
	if err := os.MkdirAll(cache, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GREPPLE_CACHE_DIR", cache)
	t.Setenv(parser.NavigationCacheDirectoryEnv, filepath.Join(t.TempDir(), "navigation"))
	path := filepath.Join(root, "private-daemon-graph.go")
	if err := os.WriteFile(path, []byte("package sample\nfunc Target() {}\nfunc Caller(){ Target() }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })
	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan error, 1)
	go func() { finished <- daemon.Serve(ctx) }()
	defer func() {
		cancel()
		if err := <-finished; err != nil && !errors.Is(err, context.Canceled) {
			t.Error(err)
		}
	}()
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		if _, err := os.Stat(filepath.Join(cache, "daemon.json")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("private daemon failed to start")
		}
	}
	var cold string
	for _, state := range []string{"miss", "hit"} {
		capture.reset()
		output := captureStdout(t, func() {
			if err := Run([]string{"--no-repo-config", "--daemon", "graph", "callees", "--json", "--symbol", "Caller", filepath.Base(path)}); err != nil {
				t.Fatal(err)
			}
		})
		if state == "hit" && output != cold {
			t.Fatal("daemon cache hit changed graph result")
		}
		cold = output
		spans, payload := capture.snapshot()
		assertNavigationPhases(t, spans, "graph.callees", state, true)
		if bytes.Contains(payload, []byte(path)) || strings.Contains(string(payload), "Caller") {
			t.Fatal("daemon lookup exported graph input")
		}
	}
	cold = ""
	for _, state := range []string{"miss", "hit"} {
		capture.reset()
		output := captureStdout(t, func() {
			if err := Run([]string{"--no-repo-config", "--daemon", "graph", "resolve", "--json", "--symbol", "Caller", filepath.Base(path)}); err != nil {
				t.Fatal(err)
			}
		})
		if state == "hit" && output != cold {
			t.Fatal("daemon resolve cache hit changed result")
		}
		cold = output
		spans, payload := capture.snapshot()
		assertNavigationPhases(t, spans, "graph.resolve", state, true)
		if bytes.Contains(payload, []byte(path)) || bytes.Contains(payload, []byte("Caller")) {
			t.Fatal("daemon resolve exported graph input")
		}
	}
}
