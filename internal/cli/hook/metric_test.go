package hook

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const authoredMetricHook = `version: 1
id: go-mccabe
event: Stop
engine: gritql-metric-v1
include: ["**/*.go"]
severity: warning
message: "function {{name}} has McCabe complexity {{score}} (> {{above}})"
query: |
  language go
  { metric: {
    scope: or { function_declaration(), method_declaration() },
    name: "name", base: 1, above: 1,
    boundary: func_literal(),
    rules: [
      {id: "if", query: if_statement(), points: 1},
      {id: "loop", query: for_statement(), points: 1},
      {id: "arm", query: or { expression_case(), type_case(), communication_case() }, points: 1},
      {id: "logic", query: binary_expression(), points: 1, logical: "each", operators: ["&&", "||"]}
    ]
  } }
`

func TestMetricHookAuthoringThresholdAndCache(t *testing.T) {
	root := testHookRepository(t)
	writeHookTestFile(t, root, ".grepple/hooks/go-mccabe.yaml", authoredMetricHook)
	writeHookTestFile(t, root, "code.go", "package demo\nfunc f(x bool) { if x {} }\nfunc g() {}\n")
	for attempt := 0; attempt < 2; attempt++ {
		report, status, err := runHookTest(t, "--all", "--id", "go-mccabe")
		if err != nil || status != 1 || len(report.Findings) != 1 || report.Findings[0].Message != "function f has McCabe complexity 2 (> 1)" {
			t.Fatalf("attempt=%d report=%+v status=%d err=%v", attempt, report, status, err)
		}
	}
	writeHookTestFile(t, root, "code.go", "package demo\nfunc f(x bool) { _ = x }\n")
	if report, status, err := runHookTest(t, "--all", "--id", "go-mccabe"); err != nil || status != 0 || len(report.Findings) != 0 {
		t.Fatalf("stale cache report=%+v status=%d err=%v", report, status, err)
	}
	writeHookTestFile(t, root, "code.go", "package demo\nfunc f(x bool) { if { } }\n")
	if _, _, err := runHookTest(t, "--all", "--id", "go-mccabe"); err == nil || !strings.Contains(err.Error(), "SOURCE_PARSE") {
		t.Fatalf("source diagnostics=%v", err)
	}
	writeHookTestFile(t, root, ".grepple/hooks/go-mccabe.yaml", strings.Replace(authoredMetricHook, "if_statement()", "not_a_go_kind()", 1))
	if _, _, err := runHookTest(t, "--all", "--id", "go-mccabe"); err == nil || !strings.Contains(err.Error(), "unknown syntax-node kind") {
		t.Fatalf("invalid GritQL=%v", err)
	}
	writeHookTestFile(t, root, ".grepple/hooks/go-mccabe.yaml", strings.Replace(authoredMetricHook, "{{score}}", "{{unknown}}", 1))
	if _, _, err := runHookTest(t, "--all", "--id", "go-mccabe"); err == nil || !strings.Contains(err.Error(), "unknown metric message") {
		t.Fatalf("invalid message=%v", err)
	}
}

func TestAuthoredMetricExamplesCompile(t *testing.T) {
	source, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := testHookRepository(t)
	for _, name := range []string{"go-mccabe", "go-cognitive", "go-nested-loops"} {
		content, err := os.ReadFile(filepath.Join(source, "..", "..", "..", "examples", name+".yaml"))
		if err != nil {
			t.Fatal(err)
		}
		writeHookTestFile(t, root, ".grepple/hooks/"+name+".yaml", string(content))
		rules, err := loadRules(root, []string{name})
		if err != nil || len(rules) != 1 || rules[0].metric == nil {
			t.Fatalf("example %s: %v %+v", name, err, rules)
		}
	}
}

func TestMetricHookChangedFilesOnly(t *testing.T) {
	root := testHookRepository(t)
	writeHookTestFile(t, root, ".grepple/hooks/go-mccabe.yaml", authoredMetricHook)
	writeHookTestFile(t, root, "code.go", "package demo\nfunc f(x bool) { _ = x }\n")
	gitHookTest(t, root, "init", "-q")
	gitHookTest(t, root, "add", ".")
	gitHookTest(t, root, "commit", "-qm", "baseline")
	writeHookTestFile(t, root, "code.go", "package demo\nfunc f(x bool) { if x {} }\n")
	report, status, err := runHookTest(t, "--id", "go-mccabe")
	if err != nil || status != 1 || report.Mode != "changed" || len(report.Findings) != 1 || report.Findings[0].Path != "code.go" {
		t.Fatalf("report=%+v status=%d err=%v", report, status, err)
	}
}

func TestNestedLoopMetricHookDistinguishesNestingAndInvalidatesCache(t *testing.T) {
	source, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(source, "..", "..", "..", "examples", "go-nested-loops.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	root := testHookRepository(t)
	writeHookTestFile(t, root, ".grepple/hooks/go-nested-loops.yaml", string(content))
	writeHookTestFile(t, root, "code.go", "package demo\nfunc single() { for range a { for range b {} } }\nfunc twoPairs() { for range a { for range b {} }; for range a { for range b {} } }\nfunc triple() { for range a { for range b { for range c {} } } }\nfunc serial() { for range a {}; for range b {} }\n")
	for attempt := 0; attempt < 2; attempt++ {
		report, status, err := runHookTest(t, "--all", "--id", "go-nested-loops")
		if err != nil || status != 1 || len(report.Findings) != 2 || report.Findings[0].Line != 3 || report.Findings[0].Message != "function twoPairs contains at least two nested for/range loops (nesting score 2)" || report.Findings[1].Line != 4 || report.Findings[1].Message != "function triple contains at least two nested for/range loops (nesting score 3)" {
			t.Fatalf("attempt=%d report=%+v status=%d error=%v", attempt, report, status, err)
		}
	}
	writeHookTestFile(t, root, "code.go", "package demo\nfunc twoPairs() { for range a { for range b {} } }\n")
	if report, status, err := runHookTest(t, "--all", "--id", "go-nested-loops"); err != nil || status != 0 || len(report.Findings) != 0 {
		t.Fatalf("stale cache: report=%+v status=%d error=%v", report, status, err)
	}
}
