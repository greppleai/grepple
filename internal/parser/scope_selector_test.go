package parser

import "testing"

func TestResolveScopeSelectorSharesVerifiedCodeKindsAndRoles(t *testing.T) {
	content := "package sample\nfunc outer() {\n if true {\n  println(1)\n }\n}\n"
	for _, tc := range []struct {
		name   string
		input  ScopeSelector
		want   ScopeLineRange
		status ScopeSelectionStatus
	}{
		{"if exact", ScopeSelector{"if", 3, 5, true}, ScopeLineRange{"if", 3, 5}, ScopeVerified},
		{"outer exact", ScopeSelector{"func", 2, 6, true}, ScopeLineRange{"func", 2, 6}, ScopeVerified},
		{"if shorthand", ScopeSelector{"if", 4, 4, false}, ScopeLineRange{"if", 3, 5}, ScopeVerified},
		{"func shorthand", ScopeSelector{"func", 4, 4, false}, ScopeLineRange{"func", 2, 6}, ScopeVerified},
		{"wrong kind", ScopeSelector{"class", 2, 6, true}, ScopeLineRange{}, ScopeMissing},
		{"stale end", ScopeSelector{"if", 3, 4, true}, ScopeLineRange{}, ScopeMissing},
		{"unknown kind", ScopeSelector{"identifier", 3, 5, true}, ScopeLineRange{}, ScopeMissing},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, status := ResolveScopeSelector(content, "go", tc.input, nil)
			if status != tc.status || got != tc.want {
				t.Fatalf("span=%+v status=%q; want %+v %q", got, status, tc.want, tc.status)
			}
		})
	}
}

func TestResolveScopeSelectorNativeFunctionShorthand(t *testing.T) {
	for _, tc := range []struct {
		language, content, kind string
	}{
		{"python", "def run():\n  return 1\n", "def"},
		{"rust", "fn run() {}\n", "fn"},
	} {
		got, status := ResolveScopeSelector(tc.content, tc.language, ScopeSelector{Kind: tc.kind, StartLine: 1, EndLine: 1}, nil)
		if status != ScopeVerified || got.Kind != tc.kind || got.StartLine != 1 {
			t.Fatalf("%s: span=%+v status=%q", tc.language, got, status)
		}
	}
}

func TestResolveScopeSelectorAcceptsNativeKindsAndLegacyFunctionAlias(t *testing.T) {
	content := "function run() {}\n"
	for _, kind := range []string{"function", "func"} {
		got, status := ResolveScopeSelector(content, "typescript", ScopeSelector{Kind: kind, StartLine: 1, EndLine: 1, Exact: true}, nil)
		want := ScopeLineRange{Kind: "function", StartLine: 1, EndLine: 1}
		if status != ScopeVerified || got != want {
			t.Fatalf("%s: span=%+v status=%q; want %+v", kind, got, status, want)
		}
	}
	if _, status := ResolveScopeSelector(content, "typescript", ScopeSelector{Kind: "def", StartLine: 1, EndLine: 1, Exact: true}, nil); status != ScopeMissing {
		t.Fatalf("wrong native kind status=%q", status)
	}
}

func TestResolveScopeSelectorStructuredMarkdownAndUncertainty(t *testing.T) {
	for _, tc := range []struct {
		name, lang, content string
		input               ScopeSelector
		want                ScopeLineRange
		status              ScopeSelectionStatus
	}{
		{"markdown ancestor", "markdown", "# Root\nbefore\n## Child\nchild\n", ScopeSelector{"section", 1, 4, true}, ScopeLineRange{"section", 1, 4}, ScopeVerified},
		{"markdown shorthand", "markdown", "# Root\nbefore\n## Child\nchild\n", ScopeSelector{"section", 4, 4, false}, ScopeLineRange{"section", 3, 4}, ScopeVerified},
		{"json ancestor", "json", "{\n \"a\": {\n  \"b\": 1\n }\n}\n", ScopeSelector{"object", 1, 5, true}, ScopeLineRange{"object", 1, 5}, ScopeVerified},
		{"json child", "json", "{\n \"a\": {\n  \"b\": 1\n }\n}\n", ScopeSelector{"object", 2, 4, true}, ScopeLineRange{"object", 2, 4}, ScopeVerified},
		{"json same-kind shorthand", "json", "{\n \"a\": {\n  \"b\": 1\n }\n}\n", ScopeSelector{"object", 3, 3, false}, ScopeLineRange{}, ScopeAmbiguous},
		{"json sibling ambiguity", "json", "{\"a\": {}, \"b\": {}}", ScopeSelector{"object", 1, 1, true}, ScopeLineRange{}, ScopeAmbiguous},
		{"json nested same bounds", "json", "{\"a\": {\"b\": 1}}", ScopeSelector{"object", 1, 1, true}, ScopeLineRange{"object", 1, 1}, ScopeVerified},
		{"json malformed", "json", "{\"a\": {", ScopeSelector{"object", 1, 1, true}, ScopeLineRange{}, ScopeMissing},
		{"yaml bounded sibling", "yaml", "first:\n  ok: yes\nflow: [1,\n  2]\nlast:\n  id: 3\n", ScopeSelector{"mapping", 5, 6, true}, ScopeLineRange{"mapping", 5, 6}, ScopeVerified},
		{"yaml unbounded flow", "yaml", "flow: [1,\n  2]\n", ScopeSelector{"sequence", 1, 2, true}, ScopeLineRange{}, ScopeMissing},
		{"plain text", "text", "if true {\n}\n", ScopeSelector{"if", 1, 2, true}, ScopeLineRange{}, ScopeMissing},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, status := ResolveScopeSelector(tc.content, tc.lang, tc.input, nil)
			if status != tc.status || got != tc.want {
				t.Fatalf("span=%+v status=%q; want %+v %q", got, status, tc.want, tc.status)
			}
		})
	}
}
