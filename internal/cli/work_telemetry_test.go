package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/greppleai/grepple/internal/parser"
	spanpb "go.opentelemetry.io/proto/otlp/trace/v1"
)

func workSpan(t *testing.T, spans []*spanpb.Span, name string) *spanpb.Span {
	t.Helper()
	for _, span := range spans {
		if span.Name == name {
			return span
		}
	}
	t.Fatalf("missing %s", name)
	return nil
}

func TestCLIWorkAccountingOffline(t *testing.T) {
	capture, settings, endpoint := startPhaseCapture(t)
	root := t.TempDir()
	source := []byte("package sample\nfunc Root(){ Target() }\nfunc Target() {}\n")
	gofile := filepath.Join(root, "private-work.go")
	if err := os.WriteFile(gofile, source, 0600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.txt", "b.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("needle\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	cache := filepath.Join(t.TempDir(), "navigation")
	t.Setenv(parser.NavigationCacheDirectoryEnv, cache)
	for _, tc := range []struct {
		name                                                         string
		args                                                         []string
		span                                                         string
		attempted, read, failed, bytes, candidates, selected, parsed int64
		wantCandidates, wantSelected, wantParsed, wantError          bool
	}{
		{name: "listing", args: []string{"search", "--files", "--limit", "1", root}, span: "cli.search.local", attempted: 0, read: 0, bytes: 0, candidates: 3, selected: 3, wantCandidates: true, wantSelected: true},
		{name: "content", args: []string{"search", "--no-related", "--limit", "0", "-F", "needle", root}, span: "cli.search.local", attempted: 3, read: 3, bytes: int64(len(source) + 14), candidates: 3, parsed: 0, wantCandidates: true, wantParsed: true},
		{name: "content grammar parse", args: []string{"search", "--no-related", "-F", "Target", gofile}, span: "cli.search.local", attempted: 1, read: 1, bytes: int64(len(source)), candidates: 1, parsed: 1, wantCandidates: true, wantParsed: true},
		{name: "files with matches", args: []string{"search", "--files-with-matches", "-F", "needle", root}, span: "cli.search.local", attempted: 3, read: 3, bytes: int64(len(source) + 14), candidates: 3, wantCandidates: true},
		{name: "count summary", args: []string{"search", "--count-summary", "-F", "needle", root}, span: "cli.search.local", attempted: 3, read: 3, bytes: int64(len(source) + 14), candidates: 3, wantCandidates: true},
		{name: "at", args: []string{"search", "--at", gofile + ":1"}, span: "cli.search.local", attempted: 1, read: 1, bytes: int64(len(source))},
		{name: "failed at", args: []string{"search", "--at", filepath.Join(root, "private-missing.go") + ":1"}, span: "cli.search.local", attempted: 1, read: 0, failed: 1, wantError: true},
		{name: "outline", args: []string{"search", "--outline", gofile}, span: "cli.search.local", attempted: 1, read: 1, bytes: int64(len(source)), candidates: 1, selected: 1, parsed: 1, wantCandidates: true, wantSelected: true, wantParsed: true},
		{name: "graph cold query", args: []string{"graph", "callers", "--symbol", "Target", gofile}, span: "cli.navigation.execute", attempted: 1, read: 1, bytes: int64(len(source)), parsed: 1, wantParsed: true},
		{name: "graph warm query", args: []string{"graph", "callers", "--symbol", "Target", gofile}, span: "cli.navigation.execute", attempted: 1, read: 1, bytes: int64(len(source)), parsed: 0, wantParsed: true},
		{name: "graph resolve", args: []string{"graph", "resolve", "--symbol", "Target", gofile}, span: "cli.navigation.execute", attempted: 1, read: 1, bytes: int64(len(source)), parsed: 1, wantParsed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"--no-repo-config"}, tc.args...)
			disabled, _ := json.Marshal(map[string]any{"telemetry": map[string]any{"enabled": false, "trace_endpoint": endpoint}})
			if err := os.WriteFile(settings, disabled, 0600); err != nil {
				t.Fatal(err)
			}
			capture.reset()
			var baselineErr error
			baseline := captureStdout(t, func() { baselineErr = Run(args) })
			if capture.requestCount() != 0 {
				t.Fatal("disabled telemetry exported work")
			}
			if tc.name == "graph cold query" {
				if err := os.RemoveAll(cache); err != nil {
					t.Fatal(err)
				}
			}
			enabled, _ := json.Marshal(map[string]any{"telemetry": map[string]any{"enabled": true, "trace_endpoint": endpoint}})
			if err := os.WriteFile(settings, enabled, 0600); err != nil {
				t.Fatal(err)
			}
			var tracedErr error
			observed := captureStdout(t, func() { tracedErr = Run(args) })
			if baseline != observed || (baselineErr == nil) != (tracedErr == nil) || (tracedErr != nil) != tc.wantError {
				t.Fatalf("output/exit changed: baseline=%q (%v), traced=%q (%v)", baseline, baselineErr, observed, tracedErr)
			}
			spans, payload := capture.snapshot()
			if tc.name == "content grammar parse" {
				parse := workSpan(t, spans, "cli.search.parse")
				scan := workSpan(t, spans, "cli.search.scan")
				if parse.EndTimeUnixNano < parse.StartTimeUnixNano || scan.EndTimeUnixNano > parse.StartTimeUnixNano {
					t.Fatal("parser batch is not bounded after the scan")
				}
			}
			assertCLIPhaseContract(t, spans)
			attrs := spanAttributes(workSpan(t, spans, tc.span))
			for key, want := range map[string]int64{"cli.files.attempted": tc.attempted, "cli.files.read": tc.read, "cli.files.read_failed": tc.failed, "cli.files.bytes_read": tc.bytes} {
				if attrs[key] == nil || attrs[key].GetIntValue() != want {
					t.Fatalf("%s: expected %d, got %v", key, want, attrs[key])
				}
			}
			for _, field := range []struct {
				key        string
				applicable bool
				want       int64
			}{
				{"cli.files.candidates", tc.wantCandidates, tc.candidates}, {"cli.files.selected", tc.wantSelected, tc.selected},
				{"cli.files.parse_attempted", tc.wantParsed, tc.parsed}, {"cli.files.parsed", tc.wantParsed, tc.parsed}, {"cli.files.parse_failed", tc.wantParsed, 0},
			} {
				if field.applicable && (attrs[field.key] == nil || attrs[field.key].GetIntValue() != field.want) || !field.applicable && attrs[field.key] != nil {
					t.Fatalf("%s: applicable %v want %d got %v", field.key, field.applicable, field.want, attrs[field.key])
				}
			}
			if bytes.Contains(payload, []byte(root)) || bytes.Contains(payload, []byte(gofile)) || bytes.Contains(payload, []byte("private-work")) {
				t.Fatal("private source path leaked")
			}
			capture.reset()
		})
	}
}
