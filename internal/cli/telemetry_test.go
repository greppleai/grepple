package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestConfiguredCLITracesFlushOnSuccessAndCommandError(t *testing.T) {
	var exports atomic.Int64
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/traces" || request.Method != http.MethodPost || request.Header.Get("Authorization") != "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		exports.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer collector.Close()
	path := filepath.Join(t.TempDir(), "settings.json")
	t.Setenv("GREPPLE_SETTINGS", path)
	writeSettings := func(enabled bool) {
		t.Helper()
		body, err := json.Marshal(map[string]any{"telemetry": map[string]any{"enabled": enabled, "trace_endpoint": collector.URL + "/v1/traces"}})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, body, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeSettings(false)
	captureStdout(t, func() {
		if err := Run([]string{"--no-repo-config", "version"}); err != nil {
			t.Fatal(err)
		}
	})
	if exports.Load() != 0 {
		t.Fatal("disabled telemetry contacted collector")
	}
	writeSettings(true)
	captureStdout(t, func() {
		if err := Run([]string{"--no-repo-config", "version"}); err != nil {
			t.Fatal(err)
		}
	})
	if exports.Load() != 1 {
		t.Fatalf("successful invocation exports: %d", exports.Load())
	}
	if err := Run([]string{"ask"}); err == nil {
		t.Fatal("expected a command error")
	}
	if exports.Load() != 2 {
		t.Fatalf("failed invocation exports: %d", exports.Load())
	}
}

func TestCLICollectorOutageDoesNotChangeCommandResult(t *testing.T) {
	collector := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	endpoint := collector.URL + "/v1/traces"
	collector.Close()
	path := filepath.Join(t.TempDir(), "settings.json")
	t.Setenv("GREPPLE_SETTINGS", path)
	body, err := json.Marshal(map[string]any{"telemetry": map[string]any{"enabled": true, "trace_endpoint": endpoint}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	output := captureStdout(t, func() {
		if err := Run([]string{"--no-repo-config", "version"}); err != nil {
			t.Fatalf("collector failure changed CLI exit: %v", err)
		}
	})
	if !strings.Contains(output, "grepple ") {
		t.Fatalf("collector failure changed output: %q", output)
	}
}

func TestCLIBackendRequestAndExporterShareOneInvocationTrace(t *testing.T) {
	var payload []byte
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		var err error
		payload, err = io.ReadAll(io.LimitReader(request.Body, 1<<20))
		if err != nil {
			t.Error(err)
		}
		if request.Header.Get("traceparent") != "" {
			t.Error("exporter inherited backend traceparent")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer collector.Close()
	var parent string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		parent = request.Header.Get("traceparent")
		if request.Header.Get("tracestate") != "" || request.Header.Get("baggage") != "" {
			t.Error("backend received baggage or tracestate")
		}
		fmt.Fprint(w, `{"ok":true,"repos":[{"repo":"example/repo"}]}`)
	}))
	defer backend.Close()
	path := filepath.Join(t.TempDir(), "settings.json")
	t.Setenv("GREPPLE_SETTINGS", path)
	t.Setenv("GREPPLE_TOKEN", "")
	writeSettings := func(enabled bool) {
		t.Helper()
		body, err := json.Marshal(map[string]any{"telemetry": map[string]any{"enabled": enabled, "trace_endpoint": collector.URL + "/v1/traces"}})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, body, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeSettings(false)
	captureStdout(t, func() {
		if err := Run([]string{"--no-repo-config", "repos", "--server", backend.URL}); err != nil {
			t.Fatal(err)
		}
	})
	if parent != "" || len(payload) != 0 {
		t.Fatalf("disabled trace emitted headers or export: %q", parent)
	}
	writeSettings(true)
	captureStdout(t, func() {
		if err := Run([]string{"--no-repo-config", "repos", "--server", backend.URL}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.HasPrefix(parent, "00-") || !strings.HasSuffix(parent, "-01") || !bytes.Contains(payload, []byte("cli.http.repos")) || bytes.Contains(payload, []byte(backend.URL)) {
		t.Fatalf("missing or unsafe backend trace: parent=%q exported=%d", parent, len(payload))
	}
}

func TestCLISlowCollectorCannotDelayExitIndefinitely(t *testing.T) {
	collector := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(3 * time.Second):
		}
	}))
	defer collector.Close()
	path := filepath.Join(t.TempDir(), "settings.json")
	t.Setenv("GREPPLE_SETTINGS", path)
	body, err := json.Marshal(map[string]any{"telemetry": map[string]any{"enabled": true, "trace_endpoint": collector.URL + "/v1/traces"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	output := captureStdout(t, func() {
		if err := Run([]string{"--no-repo-config", "version"}); err != nil {
			t.Fatalf("slow collector changed exit: %v", err)
		}
	})
	if time.Since(started) > 3*time.Second || !strings.Contains(output, "grepple ") {
		t.Fatal("slow collector delayed or changed CLI result")
	}
}
