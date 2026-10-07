package hook

import (
	"github.com/greppleai/grepple/internal/gritql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const pathAssertion = `version: 1
id: path-name
engine: gritql-v1
include: ["**/*.go"]
severity: error
unsuppressible: true
message: "expected {{expected}}, got {{actual}} at {{path}}"
query: |
  language go
  package_clause($name)
assert:
  equals:
    - binding: name
    - concat:
        - segment: {input: {path: true}, separator: "/", index: 1}
        - segment: {input: {path: true}, separator: "/", index: 2}
`

func TestStringAssertionLanguagesAndExactComparison(t *testing.T) {
	for _, language := range []string{"go", "js"} {
		t.Run(language, func(t *testing.T) {
			root := testHookRepository(t)
			config := pathAssertion
			extension := ".go"
			source := "package catalogcontroller\n// package wrong is a comment\n"
			if language == "js" {
				extension = ".js"
				source = "const catalogcontroller = 1; // const wrong = 2;\n"
				config = strings.ReplaceAll(config, "**/*.go", "**/*.js")
				config = strings.Replace(config, "language go\n  package_clause($name)", "language javascript\n  identifier() as $name", 1)
			}
			writeHookTestFile(t, root, ".grepple/hooks/path-name.yaml", config)
			path := "src/catalog/controller/types" + extension
			writeHookTestFile(t, root, path, source)
			assertPathReport(t, 0)
			bad := strings.Replace(source, "catalogcontroller", "wrong", 1)
			writeHookTestFile(t, root, path, bad)
			report, status, err := runHookTest(t, "--all", "--id", "path-name")
			if err != nil || status != 1 || len(report.Findings) != 1 {
				t.Fatalf("report: %+v %d %v", report, status, err)
			}
			if !strings.Contains(report.Findings[0].Message, "expected catalogcontroller, got wrong") {
				t.Fatal(report.Findings)
			}
			assertAssertionSource(t, root, path, bad)
		})
	}
}

func assertPathReport(t *testing.T, want int) {
	t.Helper()
	report, status, err := runHookTest(t, "--all", "--id", "path-name")
	if err != nil || len(report.Findings) != want || (status != 0) != (want != 0) {
		t.Fatalf("report: %+v %d %v", report, status, err)
	}
}

func TestStringAssertionCacheConfigurationAndMoves(t *testing.T) {
	root := testHookRepository(t)
	writeHookTestFile(t, root, ".grepple/hooks/path-name.yaml", pathAssertion)
	body := "//grepple path-name ignored\npackage wrong\n"
	original := "src/catalog/controller/types.go"
	writeHookTestFile(t, root, original, body)
	assertPathReport(t, 1)
	assertPathReport(t, 1)
	writeHookTestFile(t, root, original, "package catalogcontroller\n")
	assertPathReport(t, 0)
	destination := "src/orders/controller/types.go"
	if err := os.Rename(filepath.Join(root, original), filepath.Join(root, "src/catalog/controller/saved.txt")); err != nil {
		t.Fatal(err)
	}
	writeHookTestFile(t, root, destination, "package catalogcontroller\n")
	assertPathReport(t, 1)
	writeHookTestFile(t, root, ".grepple/hooks/path-name.yaml", strings.Replace(pathAssertion, "- binding: name", "- literal: orderscontroller", 1))
	assertPathReport(t, 0)
}

func TestStringAssertionInvalidConfigFailsClosed(t *testing.T) {
	for _, config := range []string{
		strings.Replace(pathAssertion, "binding: name", "binding: absent", 1),
		strings.Replace(pathAssertion, "binding: name", "binding: name, typo: x", 1),
		strings.Replace(pathAssertion, "- binding: name", "- {binding: name, literal: x}", 1),
		strings.Replace(pathAssertion, "path: true", "path: false", 1),
		strings.Replace(pathAssertion, "separator: \"/\"", "separator: \"\"", 1),
		strings.Replace(pathAssertion, "{{actual}}", "{{unsupported}}", 1),
		strings.Split(pathAssertion, "assert:")[0] + "assert: {equals: []}\n",
	} {
		t.Run(config, func(t *testing.T) {
			root := testHookRepository(t)
			writeHookTestFile(t, root, ".grepple/hooks/path-name.yaml", config)
			if _, _, err := runHookTest(t, "--all", "--id", "path-name"); err == nil {
				t.Fatal("invalid assertion accepted")
			}
		})
	}
}

func TestStringAssertionRuntimeErrorsFailClosed(t *testing.T) {
	for _, config := range []string{
		strings.Replace(pathAssertion, "index: 1", "index: 99", 1),
		strings.Replace(pathAssertion, "package_clause($name)", "and { struct_type(), within type_spec(name=$name) }", 1),
		strings.Replace(pathAssertion, "package_clause($name)", "or { package_clause($name), type_identifier() as $other }", 1),
	} {
		t.Run(config, func(t *testing.T) {
			root := testHookRepository(t)
			writeHookTestFile(t, root, ".grepple/hooks/path-name.yaml", config)
			writeHookTestFile(t, root, "src/catalog/controller/types.go", "package catalogcontroller\ntype Example struct{}\n")
			if _, _, err := runHookTest(t, "--all", "--id", "path-name"); err == nil {
				t.Fatal("incomplete expression evaluation accepted")
			}
		})
	}
}

func TestStringExpressionOperationsAndBounds(t *testing.T) {
	literal := "  HTTP-client.go  "
	input := stringExpression{Literal: &literal}
	config := stringExpression{Upper: &stringExpression{Replace: &stringReplace{Input: stringExpression{Trim: &input}, Old: "-", New: "_"}}}
	if err := validateStringExpression(config, nil, 0, new(int)); err != nil {
		t.Fatal(err)
	}
	// Literal operations are independent of syntax and source-language semantics.
	got, err := evaluateString(config, gritql.Finding{})
	if err != nil || got != "HTTP_CLIENT.GO" {
		t.Fatalf("%q: %v", got, err)
	}
	for range 10 {
		config = stringExpression{Lower: &config}
	}
	if err := validateStringExpression(config, nil, 0, new(int)); err == nil {
		t.Fatal("depth limit bypassed")
	}
	huge := strings.Repeat("x", maxAssertionText+1)
	if err := validateStringExpression(stringExpression{Literal: &huge}, nil, 0, new(int)); err == nil {
		t.Fatal("text limit bypassed")
	}
}

func assertAssertionSource(t *testing.T, root, path, source string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, path))
	if err != nil || string(data) != source {
		t.Fatal("assertion changed source")
	}
}

func TestStringAssertionExpansionAndNodeLimits(t *testing.T) {
	literal := strings.Repeat("x", maxAssertionText)
	expression := stringExpression{Literal: &literal}
	if _, err := evaluateString(stringExpression{Concat: []stringExpression{expression, expression}}, gritql.Finding{}); err == nil {
		t.Fatal("concat allocation limit bypassed")
	}
	if _, err := evaluateString(stringExpression{Replace: &stringReplace{Input: expression, Old: "x", New: "xx"}}, gritql.Finding{}); err == nil {
		t.Fatal("replace allocation limit bypassed")
	}
	if _, err := assertionMessage("{{actual}}{{actual}}", literal, "", ""); err == nil {
		t.Fatal("message allocation limit bypassed")
	}
	expression = stringExpression{Concat: make([]stringExpression, 65)}
	empty := ""
	for i := range expression.Concat {
		expression.Concat[i] = stringExpression{Literal: &empty}
	}
	if err := validateStringExpression(expression, nil, 0, new(int)); err == nil {
		t.Fatal("node limit bypassed")
	}
}

func TestStringAssertionPathOperatorsAndNegativeSegments(t *testing.T) {
	literal := "Alpha/Beta/File.go"
	input := stringExpression{Literal: &literal}
	expressions := map[string]stringExpression{
		"File.go":            {Basename: &input},
		"Alpha/Beta":         {Dirname: &input},
		"alpha/beta/file.go": {Lower: &input},
		"Beta":               {Segment: &stringSegment{Input: input, Separator: "/", Index: -2}},
	}
	for expected, e := range expressions {
		value, err := evaluateString(e, gritql.Finding{})
		if err != nil || value != expected {
			t.Fatalf("want=%q got=%q err=%v", expected, value, err)
		}
	}
}
