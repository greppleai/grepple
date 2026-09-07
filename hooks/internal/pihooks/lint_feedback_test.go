package pihooks

import (
	"strings"
	"testing"
)

func testDiagnostic(file string, line int, rule, message string) Diagnostic {
	return Diagnostic{
		Severity: "warning",
		Failure:  message,
		RuleName: rule,
		Category: "style",
		Position: DiagnosticPosition{
			Start: SourcePosition{Filename: file, Line: line, Column: 1},
			End:   SourcePosition{Filename: file, Offset: 1, Line: line, Column: 2},
		},
		Confidence: 1,
	}
}

func TestParseReviveReport(t *testing.T) {
	for _, input := range []string{"", "null", "[]"} {
		diagnostics, err := ParseReviveReport(input)
		if err != nil || len(diagnostics) != 0 {
			t.Fatalf("ParseReviveReport(%q) = %#v, %v", input, diagnostics, err)
		}
	}
	if _, err := ParseReviveReport("{}"); err == nil || !strings.Contains(err.Error(), "unmarshal") {
		t.Fatalf("ParseReviveReport object error = %v", err)
	}
}

func TestSelectNextDiagnosticGroup(t *testing.T) {
	diagnostics := []Diagnostic{
		testDiagnostic("internal/b.go", 5, "exported", "wrong"),
		testDiagnostic("internal/a.go", 10, "exported", "wrong"),
		testDiagnostic("internal/a.go", 20, "var-naming", "wrong"),
		testDiagnostic("internal/a.go", 30, "exported", "wrong"),
	}
	group := SelectNextDiagnosticGroup(diagnostics)
	if len(group) != 2 || group[0].Position.Start.Line != 10 || group[1].Position.Start.Line != 30 {
		t.Fatalf("unexpected group: %#v", group)
	}

	tied := SelectNextDiagnosticGroup([]Diagnostic{
		testDiagnostic("b/x.go", 1, "exported", "wrong"),
		testDiagnostic("a/x.go", 1, "exported", "wrong"),
	})
	if diagnosticPath(tied[0]) != "a/x.go" {
		t.Fatalf("tie selected %q", diagnosticPath(tied[0]))
	}
}

func TestSelectNextDiagnosticGroupMergesCommentRules(t *testing.T) {
	group := SelectNextDiagnosticGroup([]Diagnostic{
		testDiagnostic("a.go", 10, "exported", "one"),
		testDiagnostic("a.go", 1, "package-comments", "two"),
		testDiagnostic("a.go", 30, "exported", "three"),
		testDiagnostic("a.go", 40, "var-naming", "four"),
	})
	if len(group) != 3 || group[0].RuleName != "package-comments" || group[1].RuleName != "exported" {
		t.Fatalf("unexpected comment group: %#v", group)
	}
}

func TestLoadGuideForGroup(t *testing.T) {
	root := testProjectRoot(t)
	comments, err := LoadGuideForGroup(root, []Diagnostic{
		testDiagnostic("a.go", 1, "package-comments", "one"),
		testDiagnostic("a.go", 5, "exported", "two"),
	})
	if err != nil || comments.Name != "comments" {
		t.Fatalf("comments guide = %#v, %v", comments, err)
	}
	fallback, err := LoadGuide(root, testDiagnostic("a.go", 1, "no-such-rule", "wrong"))
	if err != nil || fallback.Name != "general" {
		t.Fatalf("fallback guide = %#v, %v", fallback, err)
	}
	if got := GuideNameForDiagnostic(testDiagnostic("a.go", 1, "INVALID/name", "wrong")); got != "general" {
		t.Fatalf("unsafe guide name = %q", got)
	}
}

func TestFormatDiagnosticFeedback(t *testing.T) {
	root := testProjectRoot(t)
	group := []Diagnostic{
		testDiagnostic("f.go", 1, "package-comments", "should have a package comment"),
		testDiagnostic("f.go", 5, "exported", "exported func X should have comment"),
	}
	guide, err := LoadGuideForGroup(root, group)
	if err != nil {
		t.Fatal(err)
	}
	text := FormatDiagnosticFeedback(group, 9, guide, []string{"f.go"})
	for _, expected := range []string{
		"multiple comment-related violations in one file",
		"Issues: package-comments, exported (warning)",
		"Diagnostics:\n- f.go:1:1",
		"Guide: hooks/guides/comments.md",
		"Auto-fixed by gofmt in this pass:\n- f.go",
	} {
		if !strings.Contains(text, expected) {
			t.Errorf("feedback missing %q:\n%s", expected, text)
		}
	}

	single := []Diagnostic{testDiagnostic("internal/x.go", 42, "exported", "exported func X should have comment")}
	singleGuide, err := LoadGuideForGroup(root, single)
	if err != nil {
		t.Fatal(err)
	}
	singleText := FormatDiagnosticFeedback(single, 7, singleGuide, nil)
	for _, expected := range []string{"Resolve only this issue", "Remaining diagnostics in this run: 7", "Issue: exported (warning)", "Location: internal/x.go:42:1"} {
		if !strings.Contains(singleText, expected) {
			t.Errorf("single feedback missing %q", expected)
		}
	}
}

func testProjectRoot(t *testing.T) string {
	t.Helper()
	root := "../.."
	if _, err := LoadGuide(root, Diagnostic{}); err != nil {
		t.Fatalf("test project root: %v", err)
	}
	return root
}
