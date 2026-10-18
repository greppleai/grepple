package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	spanpb "go.opentelemetry.io/proto/otlp/trace/v1"
)

func commandSpan(t *testing.T, spans []*spanpb.Span) *spanpb.Span {
	t.Helper()
	for _, span := range spans {
		if span.Name == "cli.command" {
			return span
		}
	}
	t.Fatal("missing command span")
	return nil
}

func TestParsedCommandOptionTaxonomy(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want map[string]any
	}{
		{"implicit search", []string{"-F", "--line-only", "private-query", "private-path"}, map[string]any{"cli.command.name": "search", "cli.command.source_mode": "local", "cli.command.output_format": "text", "cli.command.option.fixed_strings": true, "cli.command.option.line_only": true}},
		{"explicit search", []string{"search", "-F", "--line-only", "private-query", "private-path"}, map[string]any{"cli.command.name": "search", "cli.command.source_mode": "local", "cli.command.output_format": "text", "cli.command.option.fixed_strings": true, "cli.command.option.line_only": true}},
		{"listing selectors", []string{"--no-spill", "search", "--files", "--purpose", "secret-domain", "private-path"}, map[string]any{"cli.command.name": "search", "cli.command.source_mode": "local", "cli.command.output_format": "text", "cli.command.option.files": true, "cli.command.option.metadata_filter_requested": true, "cli.command.option.no_spill": true}},
		{"outline json", []string{"search", "--outline", "--json", "private-path"}, map[string]any{"cli.command.name": "search", "cli.command.source_mode": "local", "cli.command.output_format": "json", "cli.command.option.outline": true}},
		{"summary alias", []string{"search", "--count-by-repo", "private-query"}, map[string]any{"cli.command.name": "search", "cli.command.source_mode": "local", "cli.command.output_format": "compact", "cli.command.option.count_summary": true}},
		{"remote only", []string{"search", "--remote-only", "--server", "https://private.example", "-F", "private-query"}, map[string]any{"cli.command.name": "search", "cli.command.source_mode": "remote", "cli.command.output_format": "text", "cli.command.option.fixed_strings": true}},
		{"cursor requested", []string{"search", "--cursor", "private-cursor", "private-query"}, map[string]any{"cli.command.name": "search", "cli.command.source_mode": "remote", "cli.command.output_format": "text", "cli.command.option.cursor_requested": true}},
		{"json matches no related", []string{"search", "--json-matches", "--no-related", "private-query"}, map[string]any{"cli.command.name": "search", "cli.command.source_mode": "local", "cli.command.output_format": "json", "cli.command.option.json_matches": true, "cli.command.option.no_related_requested": true}},
		{"mixed", []string{"search", "--remote", "private-query"}, map[string]any{"cli.command.name": "search", "cli.command.source_mode": "mixed", "cli.command.output_format": "text"}},
		{"local overrides server", []string{"search", "--local", "--server", "https://private.example", "private-query"}, map[string]any{"cli.command.name": "search", "cli.command.source_mode": "local", "cli.command.output_format": "text"}},
		{"graph remote at", []string{"graph", "callers", "--repo", "private/repo", "--at", "private.go:9", "--json"}, map[string]any{"cli.command.name": "graph.callers", "cli.command.source_mode": "remote", "cli.command.output_format": "json", "cli.command.option.at": true}},
		{"graph local daemon", []string{"--daemon", "graph", "callees", "--symbol", "Private"}, map[string]any{"cli.command.name": "graph.callees", "cli.command.source_mode": "local", "cli.command.output_format": "compact", "cli.command.option.daemon_requested": true}},
		{"resolve", []string{"graph", "resolve", "--symbol", "Private", "--json"}, map[string]any{"cli.command.name": "graph.resolve", "cli.command.source_mode": "local", "cli.command.output_format": "json"}},
		{"tree", []string{"tree", "--purpose", "secret-domain", "--json", "private-path"}, map[string]any{"cli.command.name": "tree", "cli.command.output_format": "json", "cli.command.option.metadata_filter_requested": true}},
		{"tree remote", []string{"tree", "--repo", "private/repo"}, map[string]any{"cli.command.name": "tree", "cli.command.source_mode": "remote", "cli.command.output_format": "text"}},
		{"overview", []string{"overview", "--json"}, map[string]any{"cli.command.name": "overview"}},
		{"version wins", []string{"--version"}, map[string]any{"cli.command.name": "version"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			parsed, help, err := parseApplicationArgs(tc.args)
			if err != nil || help {
				t.Fatalf("parse: help=%v err=%v", help, err)
			}
			got := map[string]any{}
			for _, item := range parsed.traceCommandAttributes() {
				if _, exists := got[string(item.Key)]; exists {
					t.Fatalf("duplicate key %s", item.Key)
				}
				got[string(item.Key)] = item.Value.AsInterface()
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("attributes: got %#v want %#v", got, tc.want)
			}
			for _, arg := range tc.args {
				if strings.Contains(arg, "private") {
					for _, item := range parsed.traceCommandAttributes() {
						if strings.Contains(item.Value.Emit(), arg) {
							t.Fatalf("private argument escaped in %s", item.Key)
						}
					}
				}
			}
		})
	}
}

func TestCommandOptionsOTLPAndOutputParity(t *testing.T) {
	capture, settings, endpoint := startPhaseCapture(t)
	root := t.TempDir()
	secret := "private-secret-query-and-source"
	path := filepath.Join(root, "private-secret-file.txt")
	if err := os.WriteFile(path, []byte(secret+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	args := []string{"--no-repo-config", "--no-spill", "search", "--no-related", "-F", "-i", "--files-with-matches", secret, path}
	settingsBody, _ := json.Marshal(map[string]any{"telemetry": map[string]any{"enabled": false, "trace_endpoint": endpoint}})
	if err := os.WriteFile(settings, settingsBody, 0o600); err != nil {
		t.Fatal(err)
	}
	baseline := captureStdout(t, func() {
		if err := Run(args); err != nil {
			t.Fatal(err)
		}
	})
	if capture.requestCount() != 0 {
		t.Fatal("disabled invocation exported a trace")
	}
	settingsBody, _ = json.Marshal(map[string]any{"telemetry": map[string]any{"enabled": true, "trace_endpoint": endpoint}})
	if err := os.WriteFile(settings, settingsBody, 0o600); err != nil {
		t.Fatal(err)
	}
	captured := captureStdout(t, func() {
		if err := Run(args); err != nil {
			t.Fatal(err)
		}
	})
	if captured != baseline || !strings.Contains(captured, "private-secret-file.txt") {
		t.Fatal("output changed when tracing was enabled")
	}
	spans, payload := capture.snapshot()
	assertCLIPhaseContract(t, spans)
	attrs := spanAttributes(commandSpan(t, spans))
	for key, expected := range map[string]bool{"cli.command.option.no_repo_config": true, "cli.command.option.no_spill": true, "cli.command.option.fixed_strings": true, "cli.command.option.files_with_matches": true, "cli.command.option.ignore_case": true, "cli.command.option.no_related_requested": true} {
		if attrs[key] == nil || attrs[key].GetBoolValue() != expected {
			t.Fatalf("missing typed selected option: %s", key)
		}
	}
	if attrs["cli.command.source_mode"].GetStringValue() != "local" || attrs["cli.command.output_format"].GetStringValue() != "text" {
		t.Fatalf("wrong command mode: %v", attrs)
	}
	if bytes.Contains(payload, []byte(secret)) || bytes.Contains(payload, []byte(root)) || bytes.Contains(payload, []byte("private-secret-file.txt")) {
		t.Fatal("private query, file or path escaped to trace")
	}
}

func TestCommandOptionsGraphTreeAndErrorCapture(t *testing.T) {
	capture, settings, endpoint := startPhaseCapture(t)
	path := filepath.Join(t.TempDir(), "private-graph-fixture.go")
	if err := os.WriteFile(path, []byte("package sample\nfunc Root(){ Target() }\nfunc Target() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		args                  []string
		command, mode, format string
		option                string
		wantError             bool
	}{
		{args: []string{"--no-repo-config", "graph", "callers", "--symbol", "Target", "--json", path}, command: "graph.callers", mode: "local", format: "json"},
		{args: []string{"--no-repo-config", "tree", "--json", "."}, command: "tree", format: "json"},
		{args: []string{"--no-repo-config", "search", "--at", "private-missing-file.go:1"}, command: "search", mode: "local", format: "text", option: "cli.command.option.at", wantError: true},
	} {
		settingsBody, _ := json.Marshal(map[string]any{"telemetry": map[string]any{"enabled": false, "trace_endpoint": endpoint}})
		if err := os.WriteFile(settings, settingsBody, 0o600); err != nil {
			t.Fatal(err)
		}
		capture.reset()
		var baselineErr error
		baseline := captureStdout(t, func() { baselineErr = Run(tc.args) })
		if capture.requestCount() != 0 {
			t.Fatal("disabled invocation exported a trace")
		}
		settingsBody, _ = json.Marshal(map[string]any{"telemetry": map[string]any{"enabled": true, "trace_endpoint": endpoint}})
		if err := os.WriteFile(settings, settingsBody, 0o600); err != nil {
			t.Fatal(err)
		}
		var tracedErr error
		captured := captureStdout(t, func() { tracedErr = Run(tc.args) })
		if baseline != captured || (baselineErr == nil) != (tracedErr == nil) || (tracedErr != nil) != tc.wantError {
			t.Fatalf("tracing changed output/error: baseline=%q (%v), traced=%q (%v)", baseline, baselineErr, captured, tracedErr)
		}
		spans, payload := capture.snapshot()
		assertCLIPhaseContract(t, spans)
		attrs := spanAttributes(commandSpan(t, spans))
		if attrs["cli.command.name"].GetStringValue() != tc.command || attrs["cli.command.output_format"].GetStringValue() != tc.format {
			t.Fatalf("command mode mismatch: %v", attrs)
		}
		if tc.mode == "" {
			if attrs["cli.command.source_mode"] != nil {
				t.Fatal("tree legacy target is not proven local")
			}
		} else if attrs["cli.command.source_mode"].GetStringValue() != tc.mode {
			t.Fatalf("source mode mismatch: %v", attrs)
		}
		if tc.option != "" && (attrs[tc.option] == nil || !attrs[tc.option].GetBoolValue()) {
			t.Fatalf("option missing: %s", tc.option)
		}
		if bytes.Contains(payload, []byte(path)) || bytes.Contains(payload, []byte("private-missing-file")) {
			t.Fatal("private file escaped to trace")
		}
	}
}
