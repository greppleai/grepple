package parser

import (
	"reflect"
	"testing"
)

// The registry drives the outer loop. Adding a supported content ID without an
// independently specified declaration/body fixture fails rather than silently
// skipping the new language.
func TestScopeAnnotationsExactRegisteredContentMatrix(t *testing.T) {
	type expected struct{ nearest, scope *ScopeLineRange }
	type fixture struct {
		content               string
		declaration, body     int
		atDeclaration, atBody expected
	}
	span := scopeSpan
	same := func(kind string, start, end int) expected { s := span(kind, start, end); return expected{s, s} }
	cases := map[string]fixture{
		"go":         {"package p\nfunc run() {\n work()\n}\n", 2, 3, same("func", 2, 4), same("func", 2, 4)},
		"java":       {"class App {\n void run() {\n  work();\n }\n}\n", 2, 3, same("method", 2, 4), same("method", 2, 4)},
		"kotlin":     {"fun run() {\n work()\n}\n", 1, 2, same("func", 1, 3), same("func", 1, 3)},
		"dart":       {"void run() {\n work();\n}\n", 1, 2, same("func", 1, 3), same("func", 1, 3)},
		"swift":      {"func run() {\n work()\n}\n", 1, 2, same("func", 1, 3), same("func", 1, 3)},
		"hcl":        {"resource \"test\" \"example\" {\n name = \"work\"\n}\n", 1, 2, same("block", 1, 3), same("block", 1, 3)},
		"svelte":     {"{#if ready}\n <p>work</p>\n{/if}\n", 1, 2, expected{span("if", 1, 3), nil}, expected{span("if", 1, 3), nil}},
		"css":        {".box {\n color: red; /* work */\n}\n", 1, 2, same("rule", 1, 3), expected{span("property", 2, 2), span("rule", 1, 3)}},
		"javascript": {"function run() {\n work();\n}\n", 1, 2, same("func", 1, 3), same("func", 1, 3)},
		"typescript": {"function run(): void {\n work();\n}\n", 1, 2, same("func", 1, 3), same("func", 1, 3)},
		"tsx":        {"function Run(): JSX.Element {\n return <div>work</div>;\n}\n", 1, 2, same("func", 1, 3), same("func", 1, 3)},
		"python":     {"def run():\n    work()\n", 1, 2, same("func", 1, 2), same("func", 1, 2)},
		"csharp":     {"class App {\n void Run() {\n  work();\n }\n}\n", 2, 3, same("method", 2, 4), same("method", 2, 4)},
		"c":          {"int run(void) {\n return work();\n}\n", 1, 2, same("func", 1, 3), same("func", 1, 3)},
		"cpp":        {"int run() {\n return work();\n}\n", 1, 2, same("func", 1, 3), same("func", 1, 3)},
		"rust":       {"fn run() {\n work();\n}\n", 1, 2, same("func", 1, 3), same("func", 1, 3)},
		"php":        {"<?php\nfunction run() {\n work();\n}\n", 2, 3, same("func", 2, 4), same("func", 2, 4)},
		"shell":      {"run() {\n echo work\n}\n", 1, 2, same("func", 1, 3), same("func", 1, 3)},
		"markdown":   {"# Section\nwork\n", 1, 2, same("section", 1, 2), same("section", 1, 2)},
		"json":       {"{\n  \"items\": [\"work\"]\n}\n", 1, 2, same("object", 1, 3), expected{span("array", 2, 2), span("object", 1, 3)}},
		"yaml":       {"config:\n  value: work\n", 1, 2, same("mapping", 1, 2), same("mapping", 1, 2)},
		"text":       {"work\nwork\n", 1, 2, expected{}, expected{}},
	}
	registry := NewParser().SupportedContentLanguages()
	if len(registry) != len(cases) {
		t.Fatalf("fixture count=%d, registered=%d", len(cases), len(registry))
	}
	seen := map[string]bool{}
	for _, language := range registry {
		tt, ok := cases[language.ID]
		if !ok || seen[language.ID] {
			t.Fatalf("missing or duplicate registered language %q", language.ID)
		}
		seen[language.ID] = true
		t.Run(language.ID, func(t *testing.T) {
			hits := map[int]bool{tt.declaration: true, tt.body: true}
			got := LineScopeAnnotations(tt.content, language.ID, hits)
			for line, want := range map[int]expected{tt.declaration: tt.atDeclaration, tt.body: tt.atBody} {
				roles := got[line]
				if !reflect.DeepEqual(roles.Nearest, want.nearest) || !reflect.DeepEqual(roles.Scope, want.scope) {
					t.Errorf("line %d: nearest=%+v scope=%+v, want %+v", line, roles.Nearest, roles.Scope, want)
				}
			}
			for line := range got {
				if !hits[line] {
					t.Errorf("unmatched line %d gained a role", line)
				}
			}
		})
	}
}
