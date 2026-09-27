package gritql

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

type languageEvaluationCase struct {
	name, language, snippet, source, path, want string
}
type targetConformanceFixture struct {
	SchemaVersion int                     `json:"schema_version"`
	Suite         string                  `json:"suite"`
	Provenance    map[string]string       `json:"provenance"`
	Cases         []targetConformanceCase `json:"cases"`
}

type targetConformanceCase struct {
	Name            string   `json:"name"`
	Language        string   `json:"language"`
	Features        []string `json:"features,omitempty"`
	Query           string   `json:"query"`
	ContextQuery    string   `json:"context_query,omitempty"`
	MinContexts     int      `json:"min_contexts,omitempty"`
	Path            string   `json:"path"`
	Source          string   `json:"source"`
	MalformedSource string   `json:"malformed_source,omitempty"`
	Findings        []string `json:"findings"`
	Diagnostics     []string `json:"diagnostics,omitempty"`
}

func TestTypeScriptAndTSXStructuralEvaluation(t *testing.T) {
	t.Parallel()
	tests := []languageEvaluationCase{
		{name: "call", language: "typescript", snippet: "foo($args)", source: "const value = foo(1, item);\n", path: "src/app.ts", want: "foo(1, item)"},
		{name: "type interpretation", language: "typescript", snippet: "Promise<$type>", source: "type Result = Promise<string>;\n", path: "src/types.mts", want: "Promise<string>"},
		{name: "object property", language: "typescript", snippet: "{$key: $value}", source: "const value = {answer: 42};\n", path: "src/app.cts", want: "{answer: 42}"},
		{name: "import source", language: "typescript", snippet: "import {value} from $source;", source: "import {value} from \"pkg\";\n", path: "src/app.ts", want: "import {value} from \"pkg\";"},
		{name: "default import identifier", language: "typescript", snippet: "import $name from $source;", source: "import Client from \"./client\";\n", path: "src/app.ts", want: "import Client from \"./client\";"},
		{name: "class member", language: "typescript", snippet: "$name: $type;", source: "class Client { value: string; }\n", path: "src/app.ts", want: "value: string;"},
		{name: "typed declaration", language: "typescript", snippet: "function f($name: $type) {}", source: "function f(name: string) {}\n", path: "src/app.ts", want: "function f(name: string) {}"},
		{name: "statement sequence", language: "typescript", snippet: "first(); second();", source: "function f(){ first(); second(); third(); }\n", path: "src/app.ts", want: "first(); second();"},
		{name: "declaration sequence", language: "typescript", snippet: "interface A {}\ntype B = string;", source: "interface A {}\ntype B = string;\nconst c = 1;\n", path: "src/app.ts", want: "interface A {}\ntype B = string;"},
		{name: "tsx", language: "tsx", snippet: "<Button value={$value} />", source: "const view = <Button value={item} />;\n", path: "src/view.tsx", want: "<Button value={item} />"},
		{name: "tsx identifier positions", language: "tsx", snippet: "<$component $property={$value} />", source: "const view = <Button value={item} />;\n", path: "src/view.tsx", want: "<Button value={item} />"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assertLanguageEvaluation(t, test)
		})
	}
}

func assertLanguageEvaluation(t *testing.T, test languageEvaluationCase) {
	t.Helper()
	program, err := Compile([]byte("language "+test.language+"\n`"+test.snippet+"`"), CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if program.Language() != test.language || program.Compatibility() != Compatibility {
		t.Fatalf("program language=%q compatibility=%q", program.Language(), program.Compatibility())
	}
	result := EvaluateFile(context.Background(), program, FileInput{Path: test.path, Content: []byte(test.source)}, EvaluateOptions{})
	if diagnostics := result.Diagnostics(); len(diagnostics) != 0 {
		t.Fatalf("diagnostics=%v", diagnostics)
	}
	findings := result.Findings()
	if len(findings) != 1 || findings[0].Text() != test.want {
		t.Fatalf("findings=%v, want %q", findings, test.want)
	}
}
func TestLanguageReliabilityConformanceFixture(t *testing.T) {
	runTargetConformanceFixture(t, "testdata/conformance/language-reliability/cases.json")
}

func TestTypeScriptConformanceFixture(t *testing.T) {
	runTargetConformanceFixture(t, "testdata/conformance/typescript/cases.json")
}

func TestJavaScriptConformanceFixture(t *testing.T) {
	runTargetConformanceFixture(t, "testdata/conformance/javascript/cases.json")
}

func TestPythonConformanceFixture(t *testing.T) {
	runTargetConformanceFixture(t, "testdata/conformance/python/cases.json")
}

func TestHookSelectorsConformanceFixture(t *testing.T) {
	runTargetConformanceFixture(t, "testdata/conformance/hook-selectors/cases.json")
}

func runTargetConformanceFixture(t *testing.T, path string) {
	t.Helper()
	cases := loadTargetConformanceFixture(t, path)
	for _, test := range cases {
		test := test
		t.Run(test.Name, func(t *testing.T) {
			assertTargetConformanceCase(t, test)
		})
	}
}

func loadTargetConformanceFixture(t *testing.T, path string) []targetConformanceCase {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(bytes.TrimSpace(content)) == 0 {
		t.Fatalf("conformance fixture %s is empty", path)
	}
	if bytes.TrimSpace(content)[0] == '[' {
		var cases []targetConformanceCase
		if err := json.Unmarshal(content, &cases); err != nil {
			t.Fatal(err)
		}
		return cases
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	var fixture targetConformanceFixture
	if err := decoder.Decode(&fixture); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatalf("conformance fixture %s has trailing JSON", path)
	}
	if fixture.SchemaVersion != 1 || fixture.Suite == "" || len(fixture.Cases) == 0 {
		t.Fatalf("invalid gritql-v1 target fixture %s: schema=%d suite=%q cases=%d", path, fixture.SchemaVersion, fixture.Suite, len(fixture.Cases))
	}
	return fixture.Cases
}
func assertTargetConformanceCase(t *testing.T, test targetConformanceCase) {
	t.Helper()
	program, err := Compile([]byte(test.Query), CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if program.Compatibility() != Compatibility || program.Language() != test.Language {
		t.Fatalf("compiled contract=%q language=%q want %q/%q", program.Compatibility(), program.Language(), Compatibility, test.Language)
	}
	result := EvaluateFile(context.Background(), program, FileInput{Path: test.Path, Language: test.Language, Content: []byte(test.Source)}, EvaluateOptions{})
	assertTargetDiagnostics(t, result, test.Diagnostics)
	assertTargetFindings(t, result, test.Findings)
	if len(test.Features) > 0 {
		assertLanguageReliability(t, test, program, result)
	}
}

func assertTargetDiagnostics(t *testing.T, result FileEvaluation, want []string) {
	t.Helper()
	diagnostics := result.Diagnostics()
	codes := make([]string, len(diagnostics))
	for index := range diagnostics {
		codes[index] = diagnostics[index].Code()
	}
	if !slices.Equal(codes, want) {
		t.Fatalf("diagnostic codes=%q want %q diagnostics=%v", codes, want, diagnostics)
	}
}

func assertTargetFindings(t *testing.T, result FileEvaluation, want []string) {
	t.Helper()
	findings := result.Findings()
	texts := make([]string, len(findings))
	for index := range findings {
		texts[index] = findings[index].Text()
	}
	if !slices.Equal(texts, want) {
		t.Fatalf("finding texts=%q want %q", texts, want)
	}
}

func assertLanguageReliability(t *testing.T, test targetConformanceCase, program *Program, result FileEvaluation) {
	t.Helper()
	assertReliabilityBindings(t, result)
	assertReliabilityContexts(t, test)
	assertReliabilityMalformedSyntax(t, test, program)
	assertReliabilityCancellation(t, test, program)
	assertReliabilitySourceLimit(t, test, program)
}

func assertReliabilityBindings(t *testing.T, result FileEvaluation) {
	t.Helper()
	findings := result.Findings()
	if len(findings) != 1 {
		t.Fatalf("reliability finding count=%d want 1", len(findings))
	}
	kinds := map[string]BindingKind{}
	for _, binding := range findings[0].Bindings() {
		kinds[binding.Name()] = binding.Kind()
	}
	if kinds["args"] != BindingList {
		t.Fatalf("binding kinds=%v want named args=list", kinds)
	}
}

func assertReliabilityContexts(t *testing.T, test targetConformanceCase) {
	t.Helper()
	program, err := Compile([]byte(test.ContextQuery), CompileOptions{})
	if err != nil {
		t.Fatalf("compile context query: %v", err)
	}
	contexts := len(program.Root().Templates())
	if contexts < test.MinContexts {
		t.Fatalf("context templates=%d want at least %d", contexts, test.MinContexts)
	}
}

func assertReliabilityMalformedSyntax(t *testing.T, test targetConformanceCase, program *Program) {
	t.Helper()
	result := EvaluateFile(context.Background(), program, FileInput{Path: test.Path, Language: test.Language, Content: []byte(test.MalformedSource)}, EvaluateOptions{})
	assertTargetDiagnostics(t, result, []string{"SOURCE_PARSE"})
	if _, err := Compile([]byte("language "+test.Language+"\n`unterminated"), CompileOptions{}); err == nil {
		t.Fatal("malformed query compiled successfully")
	}
}

func assertReliabilityCancellation(t *testing.T, test targetConformanceCase, program *Program) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result := EvaluateFile(ctx, program, FileInput{Path: test.Path, Language: test.Language, Content: []byte(test.Source)}, EvaluateOptions{})
	assertTargetDiagnostics(t, result, []string{"EVALUATION_CANCELLED"})
}

func assertReliabilitySourceLimit(t *testing.T, test targetConformanceCase, program *Program) {
	t.Helper()
	result := EvaluateFile(context.Background(), program, FileInput{Path: test.Path, Language: test.Language, Content: []byte(test.Source)}, EvaluateOptions{MaxSourceBytes: 1})
	assertTargetDiagnostics(t, result, []string{"LIMIT_SOURCE_BYTES"})
}

func TestLanguageReliabilityFixtureCoversRegisteredLanguages(t *testing.T) {
	cases := loadTargetConformanceFixture(t, "testdata/conformance/language-reliability/cases.json")
	registeredLanguages := SupportedLanguages()
	registered := registeredLanguageIDs(registeredLanguages)
	featuresByLanguage := reliabilityFeaturesByLanguage(t, cases, registered)
	required := []string{"named-metavariable", "list-metavariable", "ambiguous-snippet-context", "malformed-syntax", "cancellation", "resource-limit"}
	for _, language := range registeredLanguages {
		assertReliabilityFeatureCoverage(t, language.ID, featuresByLanguage[language.ID], required)
	}
	if len(featuresByLanguage) != len(registeredLanguages) {
		t.Errorf("fixture languages=%d registered languages=%d", len(featuresByLanguage), len(registeredLanguages))
	}
}

func registeredLanguageIDs(languages []LanguageCapabilities) map[string]bool {
	registered := make(map[string]bool, len(languages))
	for _, language := range languages {
		registered[language.ID] = true
	}
	return registered
}

func reliabilityFeaturesByLanguage(t *testing.T, cases []targetConformanceCase, registered map[string]bool) map[string]map[string]bool {
	t.Helper()
	featuresByLanguage := map[string]map[string]bool{}
	for _, fixtureCase := range cases {
		if !registered[fixtureCase.Language] {
			t.Errorf("reliability fixture language %q is not registered", fixtureCase.Language)
		}
		if _, duplicate := featuresByLanguage[fixtureCase.Language]; duplicate {
			t.Errorf("reliability fixture language %q is duplicated", fixtureCase.Language)
		}
		features := map[string]bool{}
		for _, feature := range fixtureCase.Features {
			features[feature] = true
		}
		featuresByLanguage[fixtureCase.Language] = features
	}
	return featuresByLanguage
}

func assertReliabilityFeatureCoverage(t *testing.T, language string, features map[string]bool, required []string) {
	t.Helper()
	if features == nil {
		t.Errorf("registered language %q lacks a reliability fixture", language)
		return
	}
	for _, feature := range required {
		if !features[feature] {
			t.Errorf("registered language %q lacks reliability feature %q", language, feature)
		}
	}
}

func TestTypeScriptUsesSharedQueryAlgebraAndBindingEquality(t *testing.T) {
	t.Parallel()
	query := "language typescript\nand { `pair($value, $value)`, contains `foo` }"
	program, err := Compile([]byte(query), CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	source := "const yes = pair(foo, foo);\nconst no = pair(foo, bar);\n"
	result := EvaluateFile(context.Background(), program, FileInput{Path: "src/app.ts", Language: "typescript", Content: []byte(source)}, EvaluateOptions{})
	findings := result.Findings()
	if len(result.Diagnostics()) != 0 || len(findings) != 1 || findings[0].Text() != "pair(foo, foo)" {
		t.Fatalf("findings=%v diagnostics=%v", findings, result.Diagnostics())
	}
}

func TestTypeScriptMalformedQueryNamesUnifiedContract(t *testing.T) {
	t.Parallel()
	_, err := Compile([]byte("language typescript\n`unterminated"), CompileOptions{})
	if err == nil || !strings.Contains(err.Error(), Compatibility) {
		t.Fatalf("compile error=%v", err)
	}
}

func TestTypeScriptAmbiguousSnippetRetainsExpressionAndTypeTemplates(t *testing.T) {
	t.Parallel()
	program, err := Compile([]byte("language typescript\n`Promise<$type>`"), CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	templates := program.Root().Templates()
	if len(templates) < 2 {
		t.Fatalf("templates=%d, want expression and type interpretations", len(templates))
	}
	contexts := make(map[SnippetContext]bool)
	for _, template := range templates {
		contexts[template.Context()] = true
	}
	if !contexts[SnippetContextExpression] || !contexts[SnippetContextType] {
		t.Fatalf("template contexts=%v", contexts)
	}
}

func TestTypeScriptWholePlaceholderRetainsDeclarationRole(t *testing.T) {
	t.Parallel()
	program, err := Compile([]byte("language typescript\n`$declaration`"), CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, template := range program.Root().Templates() {
		if template.Context() == SnippetContextDeclaration && templateNodeContainsVariable(template.Root(), "$declaration") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("whole declaration interpretations=%#v", program.Root().Templates())
	}
}

func templateNodeContainsVariable(node TemplateNode, name string) bool {
	if !node.Valid() {
		return false
	}
	for _, child := range node.Children() {
		if child.IsSlot() && child.Slot().Variable().Name == name {
			return true
		}
		if templateNodeContainsVariable(child.Node(), name) {
			return true
		}
	}
	return false
}

func TestTypeScriptScannerDetectsCanonicalExtensions(t *testing.T) {
	t.Parallel()
	program, err := Compile([]byte("language typescript\n`target($value)`"), CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	filesystem := fstest.MapFS{
		"src/app.ts":  &fstest.MapFile{Data: []byte("target(one);\n")},
		"src/app.mts": &fstest.MapFile{Data: []byte("target(two);\n")},
		"src/app.go":  &fstest.MapFile{Data: []byte("package p\nfunc f(){ target(three) }\n")},
	}
	candidates := []ScanCandidate{{ReadPath: "src/app.ts", Path: "src/app.ts"}, {ReadPath: "src/app.mts", Path: "src/app.mts"}, {ReadPath: "src/app.go", Path: "src/app.go"}}
	result := ScanFiles(context.Background(), filesystem, program, candidates, ScanOptions{Workers: 1})
	if len(result.Findings()) != 2 || result.Stats().SkippedLanguage != 1 {
		t.Fatalf("findings=%d stats=%#v diagnostics=%v", len(result.Findings()), result.Stats(), result.Diagnostics())
	}
	metadata := result.Metadata()
	if metadata.Contract != Compatibility || metadata.Language != "typescript" || metadata.Grammar != TypeScriptGrammar || metadata.GoGrammar != "" || metadata.TreeSitterGrammar != TreeSitterTypeScriptGrammar {
		t.Fatalf("metadata=%#v", metadata)
	}
}
func TestTypeScriptMalformedSourceIsTransactional(t *testing.T) {
	t.Parallel()
	program, err := Compile([]byte("language typescript\n`target($value)`"), CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	result := EvaluateFile(context.Background(), program, FileInput{Path: "src/app.ts", Language: "typescript", Content: []byte("function broken(\n")}, EvaluateOptions{})
	if len(result.Findings()) != 0 || len(result.Diagnostics()) != 1 || result.Diagnostics()[0].Code() != "SOURCE_PARSE" {
		t.Fatalf("findings=%v diagnostics=%v", result.Findings(), result.Diagnostics())
	}
}

func TestJavaScriptScannerHandlesJSAndJSXTransactionally(t *testing.T) {
	t.Parallel()
	program, err := Compile([]byte("language javascript\n`target($value)`"), CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	filesystem := fstest.MapFS{
		"src/app.js":   &fstest.MapFile{Data: []byte("target(one);\n")},
		"src/view.jsx": &fstest.MapFile{Data: []byte("const view = <Button>{target(two)}</Button>;\n")},
		"src/app.ts":   &fstest.MapFile{Data: []byte("target(three);\n")},
	}
	candidates := []ScanCandidate{{ReadPath: "src/app.js", Path: "src/app.js"}, {ReadPath: "src/view.jsx", Path: "src/view.jsx"}, {ReadPath: "src/app.ts", Path: "src/app.ts"}}
	result := ScanFiles(context.Background(), filesystem, program, candidates, ScanOptions{Workers: 1})
	if len(result.Findings()) != 2 || result.Stats().SkippedLanguage != 1 {
		t.Fatalf("findings=%d stats=%#v diagnostics=%v", len(result.Findings()), result.Stats(), result.Diagnostics())
	}
	metadata := result.Metadata()
	if metadata.Contract != Compatibility || metadata.Language != "javascript" || metadata.Grammar != JavaScriptGrammar || metadata.TreeSitterGrammar != TreeSitterJavaScriptGrammar {
		t.Fatalf("metadata=%#v", metadata)
	}
	malformed := EvaluateFile(context.Background(), program, FileInput{Path: "src/broken.js", Language: "javascript", Content: []byte("function broken(\n")}, EvaluateOptions{})
	if len(malformed.Findings()) != 0 || len(malformed.Diagnostics()) != 1 || malformed.Diagnostics()[0].Code() != "SOURCE_PARSE" {
		t.Fatalf("findings=%v diagnostics=%v", malformed.Findings(), malformed.Diagnostics())
	}
}

func TestTypeScriptProgramRejectsMismatchedDocumentLanguage(t *testing.T) {
	t.Parallel()
	program, err := Compile([]byte("language typescript\n`value`"), CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	result := EvaluateFile(context.Background(), program, FileInput{Path: "main.go", Language: "go", Content: []byte("package p\nvar value = 1\n")}, EvaluateOptions{})
	if diagnostics := result.Diagnostics(); len(diagnostics) != 1 || diagnostics[0].Code() != "SOURCE_PARSE" {
		t.Fatalf("diagnostics=%v", diagnostics)
	}
}
