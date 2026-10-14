package search

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/greppleai/grepple/internal/hashline"
)

func TestAtScopeCLIExactCleanSourceAndShorthand(t *testing.T) {
	root := chdirTemp(t)
	path := "quoted name.go"
	content := "package sample\nfunc outer() {\n if true {\n  println(1)\n }\n}\n"
	writeGraphSource(t, root, path, content)
	t.Setenv("GREPPLE_SETTINGS", filepath.Join(t.TempDir(), "missing.json"))
	for _, tc := range []struct {
		name       string
		args       []string
		start, end int
	}{
		{"exact default", []string{"--at", path, "@if:3-5"}, 3, 5},
		{"shorthand default", []string{"--at", path, "@if:4"}, 3, 5},
		{"function exact", []string{"--at", path, "@func:2-6"}, 2, 6},
		{"line only", []string{"--line-only", "--at", path, "@if:3-5"}, 3, 5},
		{"line only opt out", []string{"--line-only", "--no-annotate-scopes", "--at", path, "@if:3-5"}, 3, 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			output := captureStdout(t, func() {
				if err := runTestSearch(tc.args); err != nil {
					t.Fatal(err)
				}
			})
			if strings.Contains(output, " @if:") || strings.Contains(output, " @func:") || strings.Contains(output, "Related:") {
				t.Fatalf("decorated/expanded --at: %q", output)
			}
			hashes := hashline.Lines(content)
			for line := 1; line <= 6; line++ {
				needle := hashes[line-1] + "│" + strconv.Itoa(line) + "│"
				if strings.Contains(output, needle) != (line >= tc.start && line <= tc.end) {
					t.Fatalf("line %d selection [%d,%d] mismatch: %q", line, tc.start, tc.end, output)
				}
			}
		})
	}
	annotated := captureStdout(t, func() {
		if err := runTestSearch([]string{"--line-only", "-F", "println(1)", path}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(annotated, "@if:3-5") || !strings.Contains(annotated, "@func:2-6") {
		t.Fatalf("default search annotations changed: %q", annotated)
	}
}

func TestAtScopeCLIJSONKeepsLiteralLines(t *testing.T) {
	root := chdirTemp(t)
	writeGraphSource(t, root, "example.go", "package p\nfunc run() {\n if true {\n  println(1)\n }\n}\n")
	for _, args := range [][]string{{"--json", "--at", "example.go", "@if:3-5"}, {"--json", "--line-only", "--at", "example.go", "@if:3-5"}} {
		output := captureStdout(t, func() {
			if err := runTestSearch(args); err != nil {
				t.Fatal(err)
			}
		})
		type hit struct {
			Line    int    `json:"line"`
			Text    string `json:"text"`
			Nearest any    `json:"nearest"`
			Scope   any    `json:"scope"`
		}
		var body struct {
			Results []struct {
				Matches []hit `json:"matches"`
			} `json:"results"`
			Matches []hit `json:"matches"`
		}
		if err := json.Unmarshal([]byte(output), &body); err != nil {
			t.Fatal(err)
		}
		hits := body.Matches
		if len(body.Results) == 1 {
			hits = body.Results[0].Matches
		}
		if len(hits) != 3 {
			t.Fatalf("expected 3 exact JSON lines: %s", output)
		}
		for i, match := range hits {
			if match.Line != i+3 || strings.Contains(match.Text, "@if:") || match.Nearest != nil || match.Scope != nil {
				t.Fatalf("JSON source/metadata changed: %+v", match)
			}
		}
	}
}

func TestAtScopeCLIRejectsBeforeIOOrRemoteAndPreservesLegacy(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--at", "does-not-exist.go", "@if:0"}, "positive"},
		{[]string{"--at", "does-not-exist.go", "@if:2-1"}, "ascending"},
		{[]string{"--at", "does-not-exist.go", "@if:3-"}, "positive"},
		{[]string{"--at", "does-not-exist.go", "@if:3-4-5"}, "positive"},
		{[]string{"--at", "does-not-exist.go", "@func:+2"}, "positive"},
		{[]string{"--at", "does-not-exist.go", "@if:99999999999999999999999999999"}, "positive"},
		{[]string{"--at", "does-not-exist.go", "@identifier:2"}, "unknown scope kind"},
		{[]string{"--at", "does-not-exist.go", "@if"}, "expected @KIND"},
		{[]string{"--at", "does-not-exist.go", "@if:2", "extra.go"}, "cannot be combined"},
		{[]string{"--at", "does-not-exist.go:2", "@if:2"}, "cannot be combined"},
		{[]string{"--at", "does-not-exist.go", "needle"}, "cannot be combined"},
		{[]string{"--at", "does-not-exist.go", "@if:2", "--files"}, "requires default"},
	} {
		var runErr error
		output := captureStdout(t, func() { runErr = runTestSearch(tc.args) })
		if runErr == nil || !strings.Contains(runErr.Error(), tc.want) || output != "" {
			t.Fatalf("args=%q stdout=%q error=%v; want %q", tc.args, output, runErr, tc.want)
		}
	}
	for _, huge := range []string{"@if:" + strings.Repeat("9", 4096), "@if:" + strings.Repeat("1-", 2048)} {
		var runErr error
		output := captureStdout(t, func() { runErr = runTestSearch([]string{"--at", "does-not-exist.go", huge}) })
		if runErr == nil || !strings.Contains(runErr.Error(), "positive") || output != "" {
			t.Fatalf("huge selector length=%d error=%v output=%q", len(huge), runErr, output)
		}
	}
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { requests.Add(1); w.WriteHeader(http.StatusTeapot) }))
	defer server.Close()
	for _, args := range [][]string{
		{"--server", server.URL, "--at", "missing.go", "@func:2"},
		{"--remote", "--at", "missing.go", "@func:2"},
		{"--remote-only", "--at", "missing.go", "@func:2"},
		{"--cursor", "bad", "--at", "missing.go", "@func:2"},
	} {
		var runErr error
		output := captureStdout(t, func() { runErr = runTestSearch(args) })
		if runErr == nil || !strings.Contains(runErr.Error(), "require local source") || output != "" || requests.Load() != 0 {
			t.Fatalf("args=%q stdout=%q error=%v requests=%d", args, output, runErr, requests.Load())
		}
	}
	legacy, serverURL, remote, err := parseTestSearchArgs([]string{"--server", server.URL, "--repo", "acme/project", "--at", "legacy.go:2-3"})
	if err != nil || serverURL != server.URL || !remote || legacy.Params.AtScope != nil || searchRequestFromParams(legacy.Params).At != "legacy.go:2-3" {
		t.Fatalf("legacy remote options=%+v server=%q remote=%v error=%v", legacy, serverURL, remote, err)
	}
	root := chdirTemp(t)
	writeGraphSource(t, root, "local.go", "package p\nfunc run() {}\n")
	output := captureStdout(t, func() {
		if err := runTestSearch([]string{"--local", "--server", server.URL, "--at", "local.go", "@func:2-2"}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(output, hashline.Lines("package p\nfunc run() {}\n")[1]+"│2│func run() {}") || requests.Load() != 0 {
		t.Fatalf("local override output=%q remote requests=%d", output, requests.Load())
	}
	local, _, _, err := parseTestSearchArgs([]string{"--local", "--server", server.URL, "--at", "local.go", "@func:2-2"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := SearchRemote(testCommandContext(), local, server.URL); err == nil || !strings.Contains(err.Error(), "require local source") || requests.Load() != 0 {
		t.Fatalf("direct remote selector error=%v requests=%d", err, requests.Load())
	}
}

func TestAtScopeCLINoPartialStdoutOnChangedOrAmbiguousSource(t *testing.T) {
	root := chdirTemp(t)
	writeGraphSource(t, root, "change.go", "package p\nfunc f() {\n if true {\n  println(1)\n }\n}\n")
	first := captureStdout(t, func() {
		if err := runTestSearch([]string{"--at", "change.go", "@if:3-5"}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(first, "│5│") {
		t.Fatalf("initial selection=%q", first)
	}
	writeGraphSource(t, root, "change.go", "package p\nfunc f() {\n println(1)\n}\n")
	var selectionErr error
	output := captureStdout(t, func() { selectionErr = runTestSearch([]string{"--at", "change.go", "@if:3-5"}) })
	if selectionErr == nil || !strings.Contains(selectionErr.Error(), "stale") || output != "" {
		t.Fatalf("changed source error=%v stdout=%q", selectionErr, output)
	}
	writeGraphSource(t, root, "nested.json", "{\n \"a\": {\n  \"b\": 1\n }\n}\n")
	output = captureStdout(t, func() { selectionErr = runTestSearch([]string{"--json", "--at", "nested.json", "@object:3"}) })
	if selectionErr == nil || !strings.Contains(selectionErr.Error(), "ambiguous") || output != "" {
		t.Fatalf("ambiguous scope error=%v stdout=%q", selectionErr, output)
	}
}

func TestAtScopeCLISingleLineAndPatternSeparation(t *testing.T) {
	root := chdirTemp(t)
	content := "package p\nfunc run() { println(1) }\n"
	writeGraphSource(t, root, "single.go", content)
	output := captureStdout(t, func() {
		if err := runTestSearch([]string{"--at", "single.go", "@func:2-2"}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(output, hashline.Lines(content)[1]+"│2│func run() { println(1) }") || strings.Contains(output, "│1│") || strings.Contains(output, "@func:") {
		t.Fatalf("single line scope=%q", output)
	}
	options, _, _, err := parseTestSearchArgs([]string{"-F", "@if:2", "single.go"})
	if err != nil || options.Params.Query != "@if:2" || options.Params.AtScope != nil {
		t.Fatalf("annotation-looking search pattern=%+v err=%v", options, err)
	}
}

func TestAtScopeCLIRejectsIncompleteGoAndLiteralSpoofing(t *testing.T) {
	root := chdirTemp(t)
	path := "colon:name.go"
	incomplete := "func run() {\n if true {\n"
	writeGraphSource(t, root, path, incomplete)
	annotated := captureStdout(t, func() {
		if err := runTestSearch([]string{"--line-only", "-F", "func run", path}); err != nil {
			t.Fatal(err)
		}
	})
	if strings.Contains(annotated, "@func:") {
		t.Fatalf("recovered prefix was annotated: %q", annotated)
	}
	for _, token := range []string{"@func:1-1", "@func:1"} {
		var err error
		output := captureStdout(t, func() { err = runTestSearch([]string{"--at", path, token}) })
		if err == nil || !strings.Contains(err.Error(), "no verified") || output != "" {
			t.Fatalf("incomplete %s: %q / %v", token, output, err)
		}
	}
	content := "package p\nfunc f() {\n work(\"λ\") // @if:2-2  \t\n}\n"
	writeGraphSource(t, root, path, content)
	var err error
	output := captureStdout(t, func() { err = runTestSearch([]string{"--at", path, "@if:2-2"}) })
	if err == nil || !strings.Contains(err.Error(), "no verified") || output != "" {
		t.Fatalf("forged suffix: %q / %v", output, err)
	}
	output = captureStdout(t, func() { err = runTestSearch([]string{"--at", path, "@func:2-4"}) })
	if err != nil || !strings.Contains(output, hashline.Lines(content)[2]+"│3│ work(\"λ\") // @if:2-2  \t") || strings.Contains(output, "│1│") {
		t.Fatalf("clean Unicode/trailing whitespace source lost: %q / %v", output, err)
	}
	// A copied old bound is not proof even when its token looks plausible.
	writeGraphSource(t, root, path, "package p\nfunc f() {}\n")
	output = captureStdout(t, func() { err = runTestSearch([]string{"--at", path, "@func:2-4"}) })
	if err == nil || !strings.Contains(err.Error(), "stale") || output != "" {
		t.Fatalf("changed source: %q / %v", output, err)
	}
}
