package parser

import "testing"

func TestVerifiedScopeSelectorsUsesParserKindsAndExactRanges(t *testing.T) {
	for _, test := range []struct {
		name, language, content string
		candidate               ScopeCandidate
		want                    ScopeLineRange
	}{
		{"go comment excluded", "go", "// doc\nfunc Run() {}\n", ScopeCandidate{1, 2}, ScopeLineRange{}},
		{"go function", "go", "// doc\nfunc Run() {}\n", ScopeCandidate{2, 2}, ScopeLineRange{"func", 2, 2}},
		{"typescript function", "typescript", "function Run() {}\ntype Name = string\n", ScopeCandidate{1, 1}, ScopeLineRange{"function", 1, 1}},
		{"typescript type omitted", "typescript", "function Run() {}\ntype Name = string\n", ScopeCandidate{2, 2}, ScopeLineRange{}},
		{"svelte injected typescript", "svelte", "<script lang='ts'>\nexport function Run() {}\n</script>\n", ScopeCandidate{2, 2}, ScopeLineRange{"function", 2, 2}},
		{"svelte injected javascript", "svelte", "<script>\nfunction Run() {}\n</script>\n", ScopeCandidate{2, 2}, ScopeLineRange{"function", 2, 2}},
		{"svelte injected css", "svelte", "<style>\n.box {\n color: red;\n}\n</style>\n", ScopeCandidate{2, 4}, ScopeLineRange{"rule", 2, 4}},
		{"python function", "python", "def run():\n  return 1\n", ScopeCandidate{1, 2}, ScopeLineRange{"def", 1, 2}},
		{"rust trait", "rust", "trait Runner {\n fn run(&self);\n}\n", ScopeCandidate{1, 3}, ScopeLineRange{"trait", 1, 3}},
		{"rust function", "rust", "fn run() {}\n", ScopeCandidate{1, 1}, ScopeLineRange{"fn", 1, 1}},
		{"json object", "json", "{\n  \"item\": {}\n}\n", ScopeCandidate{1, 3}, ScopeLineRange{"object", 1, 3}},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := VerifiedScopeSelectors(test.content, test.language, []ScopeCandidate{test.candidate})[test.candidate]
			if got != test.want {
				t.Fatalf("selector=%#v want %#v", got, test.want)
			}
			if got != (ScopeLineRange{}) {
				if verified, status := ResolveScopeSelector(test.content, test.language, ScopeSelector{Kind: got.Kind, StartLine: got.StartLine, EndLine: got.EndLine, Exact: true}, nil); status != ScopeVerified || verified != got {
					t.Fatalf("round trip=%#v status=%s", verified, status)
				}
			}
		})
	}
}
