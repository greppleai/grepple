package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	spanpb "go.opentelemetry.io/proto/otlp/trace/v1"
)

// Compare traced and untraced runs at both ends of a large candidate set.
// The phase budget and output accounting must not grow per file or match.
func TestCLIHighFileCountTraceBoundAndParity(t *testing.T) {
	capture, settings, endpoint := startPhaseCapture(t)
	root := t.TempDir()
	t.Setenv("GREPPLE_ARTIFACT_DIR", filepath.Join(t.TempDir(), "artifacts"))
	secret := "private-high-cardinality-query"
	paths := make([]string, 96)
	for i := range paths {
		paths[i] = filepath.Join(root, fmt.Sprintf("private-%03d.txt", i))
		if err := os.WriteFile(paths[i], []byte(secret+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var expectedSpans int
	for _, size := range []int{4, len(paths)} {
		args := append([]string{"--no-repo-config", "--no-spill", "search", "--no-related", "--line-only", "--limit", "0", "-F", secret}, paths[:size]...)
		setTelemetry := func(enabled bool) {
			body, err := json.Marshal(map[string]any{"telemetry": map[string]any{"enabled": enabled, "trace_endpoint": endpoint}})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(settings, body, 0o600); err != nil {
				t.Fatal(err)
			}
		}
		setTelemetry(false)
		capture.reset()
		var baselineErr error
		baseline := captureStdout(t, func() { baselineErr = Run(args) })
		if capture.requestCount() != 0 {
			t.Fatal("disabled command exported telemetry")
		}
		setTelemetry(true)
		var tracedErr error
		output := captureStdout(t, func() { tracedErr = Run(args) })
		if baselineErr != nil || tracedErr != nil || baseline != output || strings.Count(output, secret) != size {
			t.Fatalf("result/parity mismatch size=%d baseline=%v traced=%v", size, baselineErr, tracedErr)
		}
		spans, payload := capture.snapshot()
		assertCLIPhaseContract(t, spans)
		assertSearchPhase(t, spans, "search", "content", true, true)
		if bytes.Contains(payload, []byte(secret)) || bytes.Contains(payload, []byte(root)) || bytes.Contains(payload, []byte(endpoint)) || bytes.Contains(payload, []byte("private-000.txt")) {
			t.Fatal("private source, selector or collector URL in exported telemetry")
		}
		if expectedSpans == 0 {
			expectedSpans = len(spans)
		} else if len(spans) != expectedSpans {
			t.Fatalf("span cardinality grew from %d to %d", expectedSpans, len(spans))
		}
		find := func(name string) *spanpb.Span {
			for _, span := range spans {
				if span.Name == name {
					return span
				}
			}
			t.Fatalf("missing %s", name)
			return nil
		}
		work := spanAttributes(find("cli.search.local"))
		if work["cli.files.attempted"].GetIntValue() != int64(size) || work["cli.files.read"].GetIntValue() != int64(size) || work["cli.files.bytes_read"].GetIntValue() != int64(size*(len(secret)+1)) {
			t.Fatalf("source accounting size=%d: %+v", size, work)
		}
		final := spanAttributes(find("cli.output_spill"))
		if final["cli.output.mode"].GetStringValue() != "inline" || final["cli.output.logical_bytes"].GetIntValue() != int64(len(output)) || final["cli.output.stdout_bytes"].GetIntValue() != int64(len(output)) {
			t.Fatalf("delivered byte accounting size=%d: %+v", size, final)
		}
		if final["cli.output.artifact_bytes"] != nil {
			t.Fatal("inline output invented artifact bytes")
		}
		capture.reset()
	}
}

func TestCLITraceStageAndOutputMatrix(t *testing.T) {
	capture, settings, endpoint := startPhaseCapture(t)
	root := t.TempDir()
	isolated := t.TempDir()
	t.Setenv("GREPPLE_ARTIFACT_DIR", filepath.Join(t.TempDir(), "artifacts"))
	path := filepath.Join(root, "private-stage-fixture.go")
	secret := "private-stage-query"
	if err := os.WriteFile(path, []byte("package sample\nfunc Root(){ Target() }\nfunc Target() {}\n// "+secret+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name                                 string
		args                                 []string
		command                              string
		local, discover, scan, parse, render int
		attempted                            int64
		checkRead                            bool
	}{
		{name: "listing", args: []string{"search", "--files", root}, command: "search", local: 1, discover: 1, render: 1, attempted: 0, checkRead: true},
		{name: "outline", args: []string{"search", "--outline", path}, command: "search", local: 1, discover: 1, scan: 1, render: 1, attempted: 1, checkRead: true},
		{name: "at", args: []string{"search", "--at", path + ":1"}, command: "search", local: 1, render: 1, attempted: 1, checkRead: true},
		{name: "count summary", args: []string{"search", "--count-summary", "-F", secret, root}, command: "search", local: 1, discover: 1, scan: 1, render: 1, attempted: 1, checkRead: true},
		{name: "fast empty", args: []string{"search", "--files", isolated}, command: "search", local: 1, discover: 1, render: 1, attempted: 0, checkRead: true},
		{name: "tree", args: []string{"tree", "--depth", "1", root}, command: "tree"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"--no-repo-config", "--no-spill"}, tc.args...)
			set := func(enabled bool) {
				body, err := json.Marshal(map[string]any{"telemetry": map[string]any{"enabled": enabled, "trace_endpoint": endpoint}})
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(settings, body, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			set(false)
			capture.reset()
			var baselineErr error
			baseline := captureStdout(t, func() { baselineErr = Run(args) })
			if capture.requestCount() != 0 {
				t.Fatal("disabled trace exported")
			}
			set(true)
			var tracedErr error
			output := captureStdout(t, func() { tracedErr = Run(args) })
			if baseline != output || (baselineErr == nil) != (tracedErr == nil) {
				t.Fatalf("trace changed output or exit: baseline=%v traced=%v", baselineErr, tracedErr)
			}
			spans, payload := capture.snapshot()
			assertCLIPhaseContract(t, spans)
			if bytes.Contains(payload, []byte(secret)) || bytes.Contains(payload, []byte(root)) || bytes.Contains(payload, []byte(endpoint)) || bytes.Contains(payload, []byte("private-stage-fixture")) {
				t.Fatal("private data leaked")
			}
			counts := map[string]int{}
			byName := map[string]*spanpb.Span{}
			for _, span := range spans {
				counts[span.Name]++
				byName[span.Name] = span
			}
			for name, expected := range map[string]int{"cli.search.local": tc.local, "cli.sources.discover": tc.discover, "cli.search.scan": tc.scan, "cli.search.parse": tc.parse, "cli.render": tc.render} {
				if counts[name] != expected {
					t.Fatalf("%s count=%d want=%d", name, counts[name], expected)
				}
			}
			if spanAttributes(byName["cli.command"])["cli.command.name"].GetStringValue() != tc.command {
				t.Fatal("wrong category")
			}
			if tc.checkRead && spanAttributes(byName["cli.search.local"])["cli.files.attempted"].GetIntValue() != tc.attempted {
				t.Fatalf("read count mismatch: %s", tc.name)
			}
			final := spanAttributes(byName["cli.output_spill"])
			if final["cli.output.mode"].GetStringValue() != "inline" || final["cli.output.logical_bytes"].GetIntValue() != int64(len(output)) || final["cli.output.stdout_bytes"].GetIntValue() != int64(len(output)) {
				t.Fatalf("output bytes mismatch: %v", final)
			}
			capture.reset()
		})
	}
}

// This end-to-end microbenchmark includes config load, command dispatch and
// temp-output finalization. It is NOT a controlled enabled-vs-disabled ratio.
func BenchmarkCLIVersionTelemetryDisabled(b *testing.B) {
	root := b.TempDir()
	settings := filepath.Join(root, "settings.json")
	if err := os.WriteFile(settings, []byte(`{"telemetry":{"enabled":false}}`), 0o600); err != nil {
		b.Fatal(err)
	}
	b.Setenv("GREPPLE_SETTINGS", settings)
	b.Setenv("GREPPLE_ARTIFACT_DIR", filepath.Join(root, "artifacts"))
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		b.Fatal(err)
	}
	defer devNull.Close()
	previous := os.Stdout
	os.Stdout = devNull
	defer func() { os.Stdout = previous }()
	args := []string{"--no-repo-config", "version"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := Run(args); err != nil {
			b.Fatal(err)
		}
	}
}
