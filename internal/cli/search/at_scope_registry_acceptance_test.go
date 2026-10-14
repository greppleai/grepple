package search

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/hashline"
	"github.com/greppleai/grepple/internal/parser"
)

// Exercise the public search renderer first, then feed only its emitted tokens
// to --at. The live content registry drives the loop: adding an adapter without
// a known declaration/body fixture makes this test fail.
func TestAtScopeRoundTripEmittedRegistryAnnotations(t *testing.T) {
	fixtures := map[string]struct{ suffix, content string }{
		"go":         {".go", "package p\nfunc run() {\n work()\n}\n"},
		"java":       {".java", "class App {\n void run() {\n  work();\n }\n}\n"},
		"kotlin":     {".kt", "fun run() {\n work()\n}\n"},
		"dart":       {".dart", "void run() {\n work();\n}\n"},
		"swift":      {".swift", "func run() {\n work()\n}\n"},
		"hcl":        {".tf", "resource \"test\" \"example\" {\n name = \"work\"\n}\n"},
		"svelte":     {".svelte", "{#if ready}\n <p>work</p>\n{/if}\n"},
		"css":        {".css", ".box {\n color: red; /* work */\n}\n"},
		"javascript": {".js", "function run() {\n work();\n}\n"},
		"typescript": {".ts", "function run(): void {\n work();\n}\n"},
		"tsx":        {".tsx", "function Run(): JSX.Element {\n return <div>work</div>;\n}\n"},
		"python":     {".py", "def run():\n    work()\n"},
		"csharp":     {".cs", "class App {\n void Run() {\n  work();\n }\n}\n"},
		"c":          {".c", "int run(void) {\n return work();\n}\n"},
		"cpp":        {".cpp", "int run() {\n return work();\n}\n"},
		"rust":       {".rs", "fn run() {\n work();\n}\n"},
		"php":        {".php", "<?php\nfunction run() {\n work();\n}\n"},
		"shell":      {".sh", "run() {\n echo work\n}\n"},
		"markdown":   {".md", "# Section\nwork\n"},
		"json":       {".json", "{\n  \"items\": [\"work\"]\n}\n"},
		"yaml":       {".yaml", "config:\n  value: work\n"},
		"text":       {".txt", "work\nwork\n"},
	}
	registry := parser.NewParser().SupportedContentLanguages()
	if len(registry) != len(fixtures) {
		t.Fatalf("fixtures=%d registered=%d", len(fixtures), len(registry))
	}
	root := chdirTemp(t)
	t.Setenv("GREPPLE_SETTINGS", filepath.Join(t.TempDir(), "missing.json"))
	tokenPattern := regexp.MustCompile(`@[a-z]+:[0-9]+-[0-9]+`)
	seen := map[string]bool{}
	for _, language := range registry {
		fixture, ok := fixtures[language.ID]
		if !ok || seen[language.ID] {
			t.Fatalf("missing or duplicate fixture for %q", language.ID)
		}
		seen[language.ID] = true
		t.Run(language.ID, func(t *testing.T) {
			assertEmittedScopeFixture(t, root, language.ID, "roundtrip"+fixture.suffix, fixture.content, tokenPattern)
		})
	}
}

func assertEmittedScopeFixture(t *testing.T, root, language, path, content string, tokens *regexp.Regexp) {
	t.Helper()
	writeGraphSource(t, root, path, content)
	annotated := captureStdout(t, func() {
		if err := runTestSearch([]string{"--line-only", "-F", "work", path}); err != nil {
			t.Fatal(err)
		}
	})
	emitted := tokens.FindAllString(annotated, -1)
	if language == "text" {
		if len(emitted) != 0 {
			t.Fatalf("plain source annotated: %s", annotated)
		}
		return
	}
	if len(emitted) == 0 {
		t.Fatalf("missing emitted role for %s: %s", language, annotated)
	}
	visited := map[string]bool{}
	for _, token := range emitted {
		if !visited[token] {
			visited[token] = true
			assertEmittedScopeToken(t, path, content, token)
		}
	}
}

func assertEmittedScopeToken(t *testing.T, path, content, token string) {
	t.Helper()
	bounds := strings.SplitN(strings.SplitN(token, ":", 2)[1], "-", 2)
	start, _ := strconv.Atoi(bounds[0])
	end, _ := strconv.Atoi(bounds[1])
	exact := captureStdout(t, func() {
		if err := runTestSearch([]string{"--line-only", "--at", path, token}); err != nil {
			t.Fatalf("emitted %s: %v", token, err)
		}
	})
	if strings.Contains(exact, " @") {
		t.Fatalf("selector decorated its clean source: %s", exact)
	}
	if end > start {
		legacy := captureStdout(t, func() {
			if err := runTestSearch([]string{"--line-only", "--at", fmt.Sprintf("%s:%d-%d", path, start, end)}); err != nil {
				t.Fatal(err)
			}
		})
		if legacy != exact {
			t.Fatalf("verified %s differs from exact legacy range:\n%s\nvs\n%s", token, exact, legacy)
		}
	}
	assertScopeRows(t, content, token, exact, start, end)
}

func assertScopeRows(t *testing.T, content, token, output string, start, end int) {
	t.Helper()
	lines := strings.Split(content, "\n")
	for index, hash := range hashline.Lines(content) {
		line := index + 1
		needle := fmt.Sprintf("%s│%d│%s", hash, line, lines[index])
		present := strings.Contains(output, needle)
		if present != (line >= start && line <= end) {
			t.Fatalf("emitted %s selected line %d incorrectly (present=%v): %s", token, line, present, output)
		}
	}
}
