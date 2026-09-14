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
		if err := Run([]string{"responsibilities", "--type", "Foo", "."}); err != nil {
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
		if err := Run([]string{"responsibilities", "--type", "Foo", "."}); err != nil {
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

func TestResponsibilitiesDirectoryDiscoversAndRanksTypes(t *testing.T) {
	dir := chdirTemp(t)
	writeGraphSource(t, dir, "main.go", `package sample
type Busy struct{}
func (Busy) Save(){}
func One(value Busy){ value.Save() }
func Two(value Busy){ value.Save() }
type Quiet struct{}
func (Quiet) Read(){}
func Once(value Quiet){ value.Read() }
`)
	output := captureStdout(t, func() {
		if err := Run([]string{"responsibilities", "--no-cache", "."}); err != nil {
			t.Fatal(err)
		}
	})
	for _, expected := range []string{"responsibilities paths=. files=1 types=2 shown=2", "Busy\n", "consumers: 2 functions", "Quiet\n", "consumers: 1 functions"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("directory output missing %q:\n%s", expected, output)
		}
	}
	if strings.Index(output, "Busy\n") > strings.Index(output, "Quiet\n") {
		t.Fatalf("reports are not ranked by evidence:\n%s", output)
	}
	limited := captureStdout(t, func() {
		if err := Run([]string{"responsibilities", "--no-cache", "--limit", "1", "."}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(limited, "types=2 shown=1") || !strings.Contains(limited, "omitted 1 lower-ranked types") || strings.Contains(limited, "Quiet\n") {
		t.Fatalf("limited directory output is incomplete or misleading:\n%s", limited)
	}
	jsonText := captureStdout(t, func() {
		if err := Run([]string{"responsibilities", "--json", "--no-cache", "--limit", "1", "."}); err != nil {
			t.Fatal(err)
		}
	})
	var directory directoryResponsibilitiesOutput
	if err := json.Unmarshal([]byte(jsonText), &directory); err != nil {
		t.Fatal(err)
	}
	if directory.Schema != "grepple-directory-responsibilities-v1" || len(directory.Reports) != 2 {
		t.Fatalf("directory JSON was limited: %#v", directory)
	}
}

func TestResponsibilitiesJSONIsCompleteAndNoCacheWritesFiles(t *testing.T) {
	dir := chdirTemp(t)
	writeGraphSource(t, dir, "main.go", "package sample\ntype Foo struct{}\nfunc (Foo) Save(){}\nfunc Run(value Foo){ value.Save() }\n")
	text := captureStdout(t, func() {
		if err := Run([]string{"responsibilities", "--json", "--no-cache", "--min-occurrences", "1", "--type", "Foo", "."}); err != nil {
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
		if err := Run([]string{"responsibilities", "--min-occurrences", "1", "--type", "Foo", "."}); err != nil {
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
		if err := Run([]string{"responsibilities", "--min-occurrences", "1", "--type", "Foo", "."}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(output, "consumers: 2 functions") {
		t.Fatalf("stale cache used:\n%s", output)
	}
}
