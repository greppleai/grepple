package mermaidcode

import (
	goparser "go/parser"
	"strings"
	"testing"
)

func TestNormalizeGoTypePreservesRequiredTokenSeparators(t *testing.T) {
	tests := map[string]struct {
		expression string
		want       string
	}{
		"channel":          {"chan   evalJob", "chan evalJob"},
		"empty channel":    {"chan struct {}", "chan struct{}"},
		"anonymous struct": {"struct { Name string; Ready bool }", "struct{Name string;Ready bool;}"},
		"function":         {"func(job evalJob, done chan struct{}) chan evalJob", "func(job evalJob,done chan struct{})chan evalJob"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			normalized := normalizeGoType(test.expression)
			if normalized != test.want {
				t.Fatalf("normalizeGoType(%q) = %q, want %q", test.expression, normalized, test.want)
			}
			if _, err := goparser.ParseExpr(normalized); err != nil {
				t.Fatalf("normalized type is not a Go expression: %q: %v", normalized, err)
			}
		})
	}
}

func TestNormalizeGoTupleNormalizesEachComponent(t *testing.T) {
	got := normalizeGoType("tuple~ chan   evalJob , struct { Value evalJob }, func(job evalJob) error ~")
	want := "tuple<chan evalJob,struct{Value evalJob},func(job evalJob)error>"
	if got != want {
		t.Fatalf("normalized tuple = %q, want %q", got, want)
	}
	for _, component := range splitParameters(strings.TrimSuffix(strings.TrimPrefix(got, "tuple<"), ">")) {
		if _, err := goparser.ParseExpr(component); err != nil {
			t.Fatalf("normalized tuple component is not a Go expression: %q: %v", component, err)
		}
	}
}

func TestGoComplexTypesRetainSourceFidelityAndRoundTrip(t *testing.T) {
	source := Source{Path: "types.go", Text: `package types

type evalJob struct{}
type result struct{}
type Worker struct {
	Jobs chan evalJob
	Done chan struct{}
	State struct { Value evalJob; Ready bool }
	Evaluate func(job evalJob, done chan struct{}) chan evalJob
}
func Process(jobs chan evalJob, callback func(job evalJob) error) (chan struct{}, struct { Value evalJob }) {
	return nil, struct { Value evalJob }{}
}
`}
	analysis, err := Analyze([]Source{source})
	if err != nil {
		t.Fatal(err)
	}
	worker := analysis.Declarations["Worker"]
	if worker == nil {
		t.Fatal("Worker declaration not analyzed")
	}
	assertWorkerComplexTypes(t, worker)

	process := analysis.Functions["Process"][0]
	if got, want := process.Parameters, []string{"chan evalJob", "func(job evalJob)error"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("Process parameters = %#v, want %#v", got, want)
	}
	if got, want := process.Type, "tuple<chan struct{},struct{Value evalJob}>"; got != want {
		t.Fatalf("Process result = %q, want %q", got, want)
	}

	diagram, err := GenerateClassDiagram("Worker", source, []Source{source}, GenerateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	assertDiagramFragments(t, diagram, []string{"chan evalJob", "chan struct&#123;&#125;", "struct&#123;Value evalJob&#59;Ready bool&#59;&#125;", "func&#40;job evalJob&#44;done chan struct&#123;&#125;&#41;chan evalJob"})
	diagnostics, err := CheckClassDiagram(diagram, []Source{source})
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("complex Go types did not round trip: %v %+v\n%s", err, diagnostics, diagram)
	}
}

func assertWorkerComplexTypes(t *testing.T, worker *Declaration) {
	t.Helper()
	wantFields := map[string]string{
		"Jobs": "chan evalJob", "Done": "chan struct{}",
		"State":    "struct{Value evalJob;Ready bool;}",
		"Evaluate": "func(job evalJob,done chan struct{})chan evalJob",
	}
	for _, member := range worker.Members {
		if want, ok := wantFields[member.Name]; ok {
			if member.Type != want {
				t.Errorf("%s type = %q, want %q", member.Name, member.Type, want)
			}
			delete(wantFields, member.Name)
		}
	}
	if len(wantFields) != 0 {
		t.Fatalf("missing fields: %v", wantFields)
	}
}

func assertDiagramFragments(t *testing.T, diagram string, fragments []string) {
	t.Helper()
	for _, fragment := range fragments {
		if !strings.Contains(diagram, fragment) {
			t.Errorf("generated diagram missing %q:\n%s", fragment, diagram)
		}
	}
}
