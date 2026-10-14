package search

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestScopeAnnotationsCLIOnlyMatchingBodyAndCleanAt(t *testing.T) {
	dir := chdirTemp(t)
	writeGraphSource(t, dir, "call.go", "package p\nfunc runtimeStart() {\n runtime.Start()\n runtimeStart()\n}\n")
	body := captureStdout(t, func() {
		if err := runTestSearch([]string{"--line-only", "-F", "runtime.Start", "call.go"}); err != nil {
			t.Fatal(err)
		}
	})
	if strings.Contains(body, "│2│") || strings.Contains(body, "func runtimeStart() {") || strings.Count(body, "│3│") != 1 || !strings.Contains(body, "│3│ runtime.Start() @func:2-5\n") || strings.Count(body, "@func:2-5") != 1 {
		t.Fatalf("body-only anchored search: %q", body)
	}
	both := captureStdout(t, func() {
		if err := runTestSearch([]string{"--line-only", "-F", "runtimeStart", "call.go"}); err != nil {
			t.Fatal(err)
		}
	})
	for _, line := range []string{"│2│func runtimeStart() { @func:2-5\n", "│4│ runtimeStart() @func:2-5\n"} {
		if !strings.Contains(both, line) {
			t.Errorf("missing declaration/body %q: %q", line, both)
		}
	}
	if strings.Count(both, "@func:2-5") != 2 {
		t.Errorf("duplicate same-span labels: %q", both)
	}
	clean := captureStdout(t, func() {
		if err := runTestSearch([]string{"--line-only", "--at", "call.go:3"}); err != nil {
			t.Fatal(err)
		}
	})
	if strings.Contains(clean, "@func") || !strings.Contains(clean, "│3│ runtime.Start()") {
		t.Errorf("--at source contaminated: %q", clean)
	}
	plain := captureStdout(t, func() {
		if err := runTestSearch([]string{"--line-only", "--no-annotate-scopes", "-F", "runtime.Start", "call.go"}); err != nil {
			t.Fatal(err)
		}
	})
	if strings.Contains(plain, "@func") || !strings.Contains(plain, "│3│ runtime.Start()") {
		t.Errorf("--no-annotate-scopes did not restore plain output: %q", plain)
	}
}

func TestScopeAnnotationsCLIBothJSONModesKeepLiteralText(t *testing.T) {
	dir := chdirTemp(t)
	writeGraphSource(t, dir, "call.go", "package p\nfunc run() {\n runtime.Start()\n}\n")
	for _, flag := range []string{"--json", "--json-matches"} {
		t.Run(flag, func(t *testing.T) {
			output := captureStdout(t, func() {
				if err := runTestSearch([]string{"--line-only", flag, "-F", "runtime.Start", "call.go"}); err != nil {
					t.Fatal(err)
				}
			})
			var response struct {
				Matches []struct {
					Line    int    `json:"line"`
					Text    string `json:"text"`
					Nearest *struct {
						Kind      string `json:"kind"`
						StartLine int    `json:"startLine"`
						EndLine   int    `json:"endLine"`
					} `json:"nearest"`
					Scope *struct {
						Kind      string `json:"kind"`
						StartLine int    `json:"startLine"`
						EndLine   int    `json:"endLine"`
					} `json:"scope"`
				} `json:"matches"`
			}
			if err := json.Unmarshal([]byte(output), &response); err != nil {
				t.Fatal(err)
			}
			if len(response.Matches) != 1 {
				t.Fatalf("missing flat JSON match: %s", output)
			}
			hit := response.Matches[0]
			if hit.Line != 3 || hit.Text != " runtime.Start()" || hit.Nearest == nil || hit.Scope == nil || hit.Nearest.Kind != "func" || hit.Nearest.StartLine != 2 || hit.Scope.EndLine != 4 {
				t.Fatalf("incorrect typed JSON: %s", output)
			}
			if strings.Contains(hit.Text, "@func") || strings.Contains(hit.Text, "⟦source-bytes:") {
				t.Fatal(fmt.Sprintf("decorated JSON text: %s", output))
			}
		})
	}
}
