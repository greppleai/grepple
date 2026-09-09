package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/greppleai/grepple/parser"
)

// TestOutlineArgsNeedNoPattern verifies --outline turns positionals into globs
// (like --files) and does not require a search pattern.
func TestOutlineArgsNeedNoPattern(t *testing.T) {
	options, _, _, err := parseSearchArgs([]string{"--outline", "**/*.go"})
	if err != nil {
		t.Fatal(err)
	}
	if !options.Outline {
		t.Fatal("Outline should be set")
	}
	if options.Params.Query != "" {
		t.Fatalf("query should be empty, got %q", options.Params.Query)
	}
	if len(options.Params.Globs) != 1 || options.Params.Globs[0] != "**/*.go" {
		t.Fatalf("positional should become a glob, got %#v", options.Params.Globs)
	}
}

func TestOutlineRejectsCountCombo(t *testing.T) {
	if _, _, _, err := parseSearchArgs([]string{"--outline", "--count", "x"}); err == nil {
		t.Fatal("expected --outline + --count to be rejected")
	}
}

// TestOutlineLocalRenders writes a Go file and checks the rendered outline.
func TestOutlineLocalRenders(t *testing.T) {
	dir := chdirTemp(t)
	src := "package p\n\ntype Rule struct {\n\tID string\n}\n\nfunc New() *Rule { return nil }\n"
	if err := os.WriteFile(filepath.Join(dir, "rule.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	out := captureStdout(t, func() {
		if err := runSearch([]string{"--local", "--outline", "rule.go"}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, "rule.go\tgo") {
		t.Fatalf("missing header: %q", out)
	}
	if !strings.Contains(out, "struct\tRule") || !strings.Contains(out, "func\tNew") {
		t.Fatalf("missing symbols: %q", out)
	}
}

func TestOutlineLocalJSON(t *testing.T) {
	dir := chdirTemp(t)
	src := "package p\n\nfunc Only() {}\n"
	if err := os.WriteFile(filepath.Join(dir, "only.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	out := captureStdout(t, func() {
		if err := runSearch([]string{"--local", "--outline", "--json", "only.go"}); err != nil {
			t.Fatal(err)
		}
	})
	var payload struct {
		Files []parser.FileOutline `json:"files"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("invalid json %q: %v", out, err)
	}
	if len(payload.Files) != 1 || len(payload.Files[0].Symbols) != 1 ||
		payload.Files[0].Symbols[0].Name != "Only" {
		t.Fatalf("unexpected json payload: %#v", payload)
	}
}

// TestGetOutlineRemote verifies `grepple get ... --outline` fetches raw content and
// outlines it client-side.
func TestGetOutlineRemote(t *testing.T) {
	isolateCLIAuth(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Must fetch raw (no format=json) so the client can outline it.
		if r.URL.Query().Get("format") == "json" {
			http.Error(w, "should not request json", http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte("package p\n\ntype Widget struct{}\n\nfunc Build() {}\n"))
	}))
	defer server.Close()

	out := captureStdout(t, func() {
		if err := runGet([]string{"--server", server.URL, "--outline", "owner/repo", "widget.go"}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, "struct\tWidget") || !strings.Contains(out, "func\tBuild") {
		t.Fatalf("unexpected get --outline output: %q", out)
	}
}
