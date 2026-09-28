package hook

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
)

func TestHookSuppressionCommentSyntaxAndAttachment(t *testing.T) {
	root := t.TempDir()
	source := `package demo
//grepple go-empty-if intentional empty branch
// Explanation of the exception.
func leading() {}
func unmatched() {}
//grepple go-empty-if
func missingReason() {}
//grepple go-empty-if intentional

func separated() {}
func trailing() {} //grepple go-empty-if explained inline
func afterTrailing() {}
var literal = "//grepple go-empty-if not a comment"
func inString() {}
//grepple:filelocal
func otherMarker() {}
//grepple different-hook intentional
func wrongID() {}
//grepple go-empty-if documented
//grepple different-hook documented
func multiple() {}
`
	writeHookTestFile(t, root, "fixture.go", source)
	findings := []Finding{}
	for _, tc := range []struct {
		line int
		id   string
	}{
		{4, "go-empty-if"}, {5, "go-empty-if"}, {7, "go-empty-if"},
		{10, "go-empty-if"}, {11, "go-empty-if"}, {12, "go-empty-if"},
		{14, "go-empty-if"}, {16, "go-empty-if"}, {18, "go-empty-if"},
		{21, "go-empty-if"}, {21, "different-hook"}, {21, "unrelated"},
	} {
		findings = append(findings, Finding{Path: "fixture.go", Line: tc.line, ID: tc.id})
	}
	filtered, err := filterSuppressedFindings(root, findings)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, finding := range filtered {
		got = append(got, finding.ID+":"+strconv.Itoa(finding.Line))
	}
	want := []string{"go-empty-if:5", "go-empty-if:7", "go-empty-if:10", "go-empty-if:12", "go-empty-if:14", "go-empty-if:16", "go-empty-if:18", "unrelated:21"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unsuppressed=%v, want %v", got, want)
	}
	if _, err := filterSuppressedFindings(root, []Finding{{Path: "missing.go", Line: 1, ID: "go-empty-if"}}); err == nil {
		t.Fatal("missing source must not silently suppress findings")
	}
}

func TestHookSuppressionUsesLanguageCommentsNotStrings(t *testing.T) {
	root := t.TempDir()
	writeHookTestFile(t, root, "fixture.js", `const literal = "//grepple js-hook not a comment";
function retained() {}
//grepple js-hook intentional exception
function suppressed() {}
`)
	findings, err := filterSuppressedFindings(root, []Finding{{Path: "fixture.js", Line: 2, ID: "js-hook"}, {Path: "fixture.js", Line: 4, ID: "js-hook"}})
	if err != nil || len(findings) != 1 || findings[0].Line != 2 {
		t.Fatalf("JavaScript comments: findings=%v err=%v", findings, err)
	}
	writeHookTestFile(t, root, "windows.go", "package demo\r\n//grepple go-empty-if intentional CRLF exception\r\nfunc suppressed() {}\r\n")
	windows, err := filterSuppressedFindings(root, []Finding{{Path: "windows.go", Line: 3, ID: "go-empty-if"}})
	if err != nil || len(windows) != 0 {
		t.Fatalf("CRLF comments: findings=%v err=%v", windows, err)
	}
}

func TestHookSuppressionAcrossEnginesAndCachedRuns(t *testing.T) {
	sourceDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := testHookRepository(t)
	standalone, err := os.ReadFile(filepath.Join(sourceDirectory, "..", "..", "..", ".grepple", "hooks", "go-standalone-functions.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	writeHookTestFile(t, root, ".grepple/hooks/go-standalone-functions.yaml", string(standalone))
	writeHookTestFile(t, root, ".grepple/hooks/go-mccabe.yaml", authoredMetricHook)
	writeHookTestFile(t, root, "api/code.go", `package api
//grepple go-standalone-functions intentional stateless helper
func Suppressed(x bool) int {
  //grepple go-empty-if branch reserved for future use
  if x {}
  return 1
}
//grepple go-mccabe measured boundary exception
func Metric(x bool) int { if x {} ; return 2 }
func Retained(x bool) int { if x {} ; return 3 }
`)
	for run := 0; run < 2; run++ { // second pass exercises both per-file and relational caches
		report, status, err := runHookTest(t, "--all", "--id", "go-empty-if", "--id", "go-standalone-functions", "--id", "go-mccabe")
		if err != nil || status != 1 {
			t.Fatalf("run %d: status=%d err=%v report=%+v", run, status, err, report)
		}
		var got []string
		for _, finding := range report.Findings {
			got = append(got, finding.ID+":"+strconv.Itoa(finding.Line))
		}
		sort.Strings(got)
		want := []string{"go-mccabe:3", "go-standalone-functions:9", "go-empty-if:9", "go-mccabe:10", "go-standalone-functions:10", "go-empty-if:10"}
		sort.Strings(want)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("run %d: findings=%v want %v", run, got, want)
		}
	}
	// Changing only the directive must reveal the original unsuppressed
	// relational finding instead of reusing a filtered cache entry.
	path := filepath.Join(root, "api", "code.go")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	writeHookTestFile(t, root, "api/code.go", strings.Replace(string(content), "//grepple go-standalone-functions intentional stateless helper", "//grepple go-standalone-functions", 1))
	report, status, err := runHookTest(t, "--all", "--id", "go-standalone-functions")
	if err != nil || status != 1 {
		t.Fatalf("changed directive: status=%d err=%v report=%+v", status, err, report)
	}
	found := false
	for _, finding := range report.Findings {
		if finding.Line == 3 && finding.ID == "go-standalone-functions" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing reason must not suppress the cached finding: %+v", report.Findings)
	}
}

func TestHookSuppressionSelectedRuleCanExitClean(t *testing.T) {
	root := testHookRepository(t)
	writeHookTestFile(t, root, "only.go", `package demo
//grepple go-empty-if intentional no-op
func example(ok bool) {
  if ok {} //grepple go-empty-if intentional no-op
}
`)
	// The function-level exception does not hide the nested empty-if finding;
	// the matching same-line exception does.
	for run := 0; run < 2; run++ {
		report, status, err := runHookTest(t, "--all", "--id", "go-empty-if")
		if err != nil || status != 0 || len(report.Findings) != 0 {
			t.Fatalf("run %d: status=%d err=%v report=%+v", run, status, err, report)
		}
	}
}
