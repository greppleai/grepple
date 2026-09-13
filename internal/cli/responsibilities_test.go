package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResponsibilitiesReportsRepeatedWorkflowsAndUsesCache(t *testing.T) {
	dir := chdirTemp(t)
	writeGraphSource(t, dir, "main.go", `package sample

type Foo struct{ State int }
func (Foo) Parse(){}
func (Foo) Validate(){}
func One(value Foo){ _ = value.State; value.Parse(); value.Validate() }
func Two(value Foo){ _ = value.State; value.Parse(); value.Validate() }
`)
	first := captureStdout(t, func() {
		if err := Run([]string{"responsibilities", "Foo", "."}); err != nil {
			t.Fatal(err)
		}
	})
	for _, expected := range []string{
		"Foo\n", "consumers: 2 functions / 1 files / 1 packages", "external method surface: 2/2 methods",
		"Parse + Validate", "Parse -> Validate", "Parse() + State(read) + Validate()", "2 occurrences / 1 files / 1 packages",
	} {
		if !strings.Contains(first, expected) {
			t.Fatalf("responsibility output missing %q:\n%s", expected, first)
		}
	}
	second := captureStdout(t, func() {
		if err := Run([]string{"responsibilities", "Foo", "."}); err != nil {
			t.Fatal(err)
		}
	})
	if second != first {
		t.Fatalf("cache changed deterministic output:\nfirst=%s\nsecond=%s", first, second)
	}
	if _, cache, err := buildCachedResponsibilityGraph([]string{"."}, 0, true); err != nil || cache != "hit" {
		t.Fatalf("cache state=%q err=%v", cache, err)
	}
	cacheEntries, err := os.ReadDir(filepath.Join(".grepple", "cache", "responsibilities"))
	if err != nil || len(cacheEntries) != 1 {
		t.Fatalf("cache entries=%d err=%v", len(cacheEntries), err)
	}
}

func TestResponsibilitiesJSONIsCompleteAndNoCacheWritesFiles(t *testing.T) {
	dir := chdirTemp(t)
	writeGraphSource(t, dir, "main.go", "package sample\ntype Foo struct{}\nfunc (Foo) Save(){}\nfunc Run(value Foo){ value.Save() }\n")
	text := captureStdout(t, func() {
		if err := Run([]string{"responsibilities", "--json", "--no-cache", "--min-occurrences", "1", "Foo", "."}); err != nil {
			t.Fatal(err)
		}
	})
	var output responsibilitiesOutput
	if err := json.Unmarshal([]byte(text), &output); err != nil {
		t.Fatal(err)
	}
	if output.Schema != "grepple-responsibilities-v1" || output.Type != "Foo" || output.Consumers.Functions != 1 {
		t.Fatalf("output=%#v", output)
	}
	if _, err := os.Stat(filepath.Join(".grepple", "cache")); !os.IsNotExist(err) {
		t.Fatalf("--no-cache created cache: %v", err)
	}
}

func TestResponsibilitiesCacheInvalidatesWhenSourceChanges(t *testing.T) {
	dir := chdirTemp(t)
	path := writeGraphSource(t, dir, "main.go", "package sample\ntype Foo struct{}\nfunc (Foo) Save(){}\nfunc Run(value Foo){ value.Save() }\n")
	captureStdout(t, func() {
		if err := Run([]string{"responsibilities", "--min-occurrences", "1", "Foo", "."}); err != nil {
			t.Fatal(err)
		}
	})
	if err := os.WriteFile(path, []byte("package sample\ntype Foo struct{}\nfunc (Foo) Save(){}\nfunc Run(value Foo){ value.Save() }\nfunc Again(value Foo){ value.Save() }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, cache, err := buildCachedResponsibilityGraph([]string{"."}, 0, true)
	if err != nil || cache != "miss" {
		t.Fatalf("cache state=%q err=%v", cache, err)
	}
	output := captureStdout(t, func() {
		if err := Run([]string{"responsibilities", "--min-occurrences", "1", "Foo", "."}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(output, "consumers: 2 functions") {
		t.Fatalf("stale cache used:\n%s", output)
	}
}
