package search

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/parser"
)

// TestOutlineArgsNeedNoPattern verifies --outline turns positionals into globs
// (like --files) and does not require a search pattern.
func TestOutlineArgsNeedNoPattern(t *testing.T) {
	options, _, _, err := parseTestSearchArgs([]string{"--outline", "**/*.go"})
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
	if _, _, _, err := parseTestSearchArgs([]string{"--outline", "--count", "x"}); err == nil {
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
		if err := runTestSearch([]string{"--local", "--outline", "rule.go"}); err != nil {
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

func TestOutlineHumanOutputIsBounded(t *testing.T) {
	dir := chdirTemp(t)
	src := "package p\n\n" + strings.Repeat("func Example() {}\n", 100)
	if err := os.WriteFile(filepath.Join(dir, "many.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	out := captureStdout(t, func() {
		if err := runTestSearch([]string{"--local", "--outline", "--max-output-bytes", "256", "many.go"}); err != nil {
			t.Fatal(err)
		}
	})
	if len(out) > 256 || !strings.Contains(out, "grepple output truncated") {
		t.Fatalf("outline output was not cleanly bounded (%d bytes): %q", len(out), out)
	}
}

func TestOutlineLocalJSON(t *testing.T) {
	dir := chdirTemp(t)
	src := "package p\n\nfunc Only() {}\n"
	if err := os.WriteFile(filepath.Join(dir, "only.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	out := captureStdout(t, func() {
		if err := runTestSearch([]string{"--local", "--outline", "--json", "only.go"}); err != nil {
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

func TestOutlineSelectorsAreOptInAndRemainLocalHumanOutput(t *testing.T) {
	dir := chdirTemp(t)
	src := "// doc\nfunc Run() {}\ntype Name string\n"
	if err := os.WriteFile(filepath.Join(dir, "sample.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	out := captureStdout(t, func() {
		if err := runTestSearch([]string{"--outline", "--outline-selectors", "sample.go"}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, "Run @func:2-2\n") || !strings.Contains(out, "3-3\ttype\tName") || strings.Contains(out, "@type:") {
		t.Fatalf("selector outline=%q", out)
	}
	plain := captureStdout(t, func() {
		if err := runTestSearch([]string{"--outline", "sample.go"}); err != nil {
			t.Fatal(err)
		}
	})
	if strings.Contains(plain, "@func:") {
		t.Fatalf("default outline changed: %q", plain)
	}
	for _, args := range [][]string{{"--outline-selectors", "sample.go"}, {"--outline", "--outline-selectors", "--json", "sample.go"}} {
		if _, _, _, err := parseTestSearchArgs(args); err == nil {
			t.Fatalf("expected rejection for %v", args)
		}
	}
}
