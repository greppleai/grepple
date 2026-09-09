package gritql

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestConformanceCorpusIntegrity(t *testing.T) {
	t.Parallel()

	corpus, err := LoadConformance(filepath.Join("testdata", "conformance"))
	if err != nil {
		t.Fatal(err)
	}
	if len(corpus.Files) < len(ConformanceSuites) {
		t.Fatalf("loaded %d fixture files, want at least %d", len(corpus.Files), len(ConformanceSuites))
	}
	if len(corpus.Cases) < 50 {
		t.Fatalf("loaded only %d cases; corpus was probably truncated", len(corpus.Cases))
	}
}

// TestMatcherConformanceExecution runs every matcher case through the production
// compile, parse/evaluate, and global normalization path. The JSON comparison is
// deliberately against the fixture schema rather than a test-only projection.
func TestMatcherConformanceExecution(t *testing.T) {
	corpus, err := LoadConformance(filepath.Join("testdata", "conformance"))
	if err != nil {
		t.Fatal(err)
	}
	for _, fixtureCase := range corpus.Cases {
		if fixtureCase.Suite != "matcher" {
			continue
		}
		fixtureCase := fixtureCase
		t.Run(strings.TrimPrefix(fixtureCase.Name, "matcher/"), func(t *testing.T) {
			runMatcherConformanceCase(t, fixtureCase)
		})
	}
}

func runMatcherConformanceCase(t *testing.T, fixtureCase Case) {
	t.Helper()
	program, err := Compile([]byte(fixtureCase.Pattern), CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	perFile, err := evaluateMatcherSources(program, fixtureCase)
	if err != nil {
		t.Fatal(err)
	}
	actual := NormalizeEvaluations(context.Background(), perFile, EvaluateOptions{})
	assertConformanceJSON(t, "findings", actual.Findings(), fixtureCase.Expected.Findings)
	assertConformanceJSON(t, "diagnostics", actual.Diagnostics(), fixtureCase.Expected.Diagnostics)
}

func evaluateMatcherSources(program *Program, fixtureCase Case) ([]FileEvaluation, error) {
	perFile := make([]FileEvaluation, 0, len(fixtureCase.Sources))
	for _, source := range fixtureCase.Sources {
		content, err := matcherSourceContent(source)
		if err != nil {
			return nil, err
		}
		perFile = append(perFile, EvaluateFile(context.Background(), program, FileInput{
			Path: source.Path, Content: content, PatternID: fixtureCase.PatternID, Message: fixtureCase.Message,
		}, EvaluateOptions{}))
	}
	return perFile, nil
}

func matcherSourceContent(source Source) ([]byte, error) {
	var content []byte
	if source.Text != nil {
		content = []byte(*source.Text)
	}
	if source.BytesBase64 == nil {
		return content, nil
	}
	return base64.StdEncoding.Strict().DecodeString(*source.BytesBase64)
}

func assertConformanceJSON(t *testing.T, label string, actual, expected any) {
	t.Helper()
	actualJSON, err := json.Marshal(actual)
	if err != nil {
		t.Fatal(err)
	}
	decoded := reflect.New(reflect.TypeOf(expected))
	if err := json.Unmarshal(actualJSON, decoded.Interface()); err != nil {
		t.Fatal(err)
	}
	decodedJSON, err := json.Marshal(decoded.Elem().Interface())
	if err != nil {
		t.Fatal(err)
	}
	expectedJSON, err := json.Marshal(expected)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decodedJSON, expectedJSON) {
		t.Fatalf("%s mismatch\nactual:   %s\nexpected: %s", label, actualJSON, expectedJSON)
	}
}

// TestConformanceReadiness is the sole readiness skip. Parser/compiler behavior
// has focused production tests and matcher is executable above; scanner/API/wire
// integration fixtures are enabled when those task-boundary components arrive.
func TestConformanceReadiness(t *testing.T) {
	t.Skip("scanner and API/wire integration conformance awaits later task components")
}

func TestRangeCoordinatesUseBytesAndScalarColumns(t *testing.T) {
	t.Parallel()

	source := []byte("a茶\r\nbé\n")
	valid := PositionRange{StartByte: 1, EndByte: 4, StartLine: 1, StartColumn: 2, EndLine: 1, EndColumn: 3}
	if err := validateRange(valid, source); err != nil {
		t.Fatalf("valid UTF-8 range rejected: %v", err)
	}
	invalid := valid
	invalid.EndByte = 3
	if err := validateRange(invalid, source); err == nil {
		t.Fatal("range ending inside a UTF-8 scalar was accepted")
	}
}

func TestConformanceLoaderRejectsDuplicateNames(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dir := filepath.Join(root, "parser")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	c := Case{
		Name: "same", Description: "duplicate sentinel", Features: []string{"positive"},
		PatternID: "r", Message: "m", Pattern: "language go\n`x`",
		Execution: Execution{Mode: "local"}, Expected: Expected{Findings: []ExpectedFinding{}, Diagnostics: []ExpectedDiagnostic{}},
	}
	fixture := FixtureFile{
		SchemaVersion: 1, Suite: "parser",
		Provenance: Provenance{Kind: "grepple-original", Note: "test sentinel"},
		Cases:      []Case{c, c},
	}
	data, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "duplicate.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConformance(root); err == nil || !strings.Contains(err.Error(), "duplicate case name") {
		t.Fatalf("LoadConformance error = %v, want duplicate-name rejection", err)
	}
}

func TestFixtureDecoderRejectsUnknownFields(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "unknown.json")
	data := []byte(`{"schema_version":1,"suite":"parser","provenance":{"kind":"grepple-original","note":"test"},"cases":[],"unknown":true}`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := decodeFixture(path); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("decodeFixture error = %v, want unknown-field rejection", err)
	}
}
