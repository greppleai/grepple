package gritql

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFindingOrderingTieBreakers(t *testing.T) {
	binding := func(lexeme string) map[string]Binding {
		return map[string]Binding{"x": {
			Kind: "node", Range: &PositionRange{},
			Structural: json.RawMessage(`{"node_kind":"identifier","children":[{"token_kind":"IDENT","lexeme":"` + lexeme + `"}]}`),
		}}
	}
	base := ExpectedFinding{Path: "a.go", PositionRange: PositionRange{StartByte: 1, EndByte: 2}, PatternID: "a", Message: "a", Bindings: binding("a")}
	pairs := []struct {
		name string
		a, b ExpectedFinding
	}{
		{"path", withFinding(base, func(f *ExpectedFinding) { f.Path = "a.go" }), withFinding(base, func(f *ExpectedFinding) { f.Path = "b.go" })},
		{"start", withFinding(base, func(f *ExpectedFinding) { f.StartByte = 1 }), withFinding(base, func(f *ExpectedFinding) { f.StartByte = 2 })},
		{"end", withFinding(base, func(f *ExpectedFinding) { f.EndByte = 2 }), withFinding(base, func(f *ExpectedFinding) { f.EndByte = 3 })},
		{"pattern", withFinding(base, func(f *ExpectedFinding) { f.PatternID = "a" }), withFinding(base, func(f *ExpectedFinding) { f.PatternID = "b" })},
		{"message", withFinding(base, func(f *ExpectedFinding) { f.Message = "a" }), withFinding(base, func(f *ExpectedFinding) { f.Message = "b" })},
		{"bindings", withFinding(base, func(f *ExpectedFinding) { f.Bindings = binding("a") }), withFinding(base, func(f *ExpectedFinding) { f.Bindings = binding("b") })},
		{"complete finding", withFinding(base, func(f *ExpectedFinding) { f.StartLine = 1 }), withFinding(base, func(f *ExpectedFinding) { f.StartLine = 2 })},
	}
	for _, pair := range pairs {
		t.Run(pair.name, func(t *testing.T) {
			if compareFindings(pair.a, pair.b) >= 0 {
				t.Fatalf("tie breaker %q did not order values directly", pair.name)
			}
		})
	}
}

func TestDiagnosticOrderingTieBreakers(t *testing.T) {
	a, b := "a", "b"
	r0, r1 := &PositionRange{StartByte: 0}, &PositionRange{StartByte: 1}
	base := ExpectedDiagnostic{Code: "A", Message: "a"}
	pairs := []struct {
		name string
		a, b ExpectedDiagnostic
	}{
		{"null path", withDiagnostic(base, func(d *ExpectedDiagnostic) { d.Path = nil }), withDiagnostic(base, func(d *ExpectedDiagnostic) { d.Path = &a })},
		{"path", withDiagnostic(base, func(d *ExpectedDiagnostic) { d.Path = &a }), withDiagnostic(base, func(d *ExpectedDiagnostic) { d.Path = &b })},
		{"null range", withDiagnostic(base, func(d *ExpectedDiagnostic) { d.Range = nil }), withDiagnostic(base, func(d *ExpectedDiagnostic) { d.Range = r0 })},
		{"range", withDiagnostic(base, func(d *ExpectedDiagnostic) { d.Range = r0 }), withDiagnostic(base, func(d *ExpectedDiagnostic) { d.Range = r1 })},
		{"code", withDiagnostic(base, func(d *ExpectedDiagnostic) { d.Code = "A" }), withDiagnostic(base, func(d *ExpectedDiagnostic) { d.Code = "B" })},
		{"null pattern", withDiagnostic(base, func(d *ExpectedDiagnostic) { d.PatternID = nil }), withDiagnostic(base, func(d *ExpectedDiagnostic) { d.PatternID = &a })},
		{"pattern", withDiagnostic(base, func(d *ExpectedDiagnostic) { d.PatternID = &a }), withDiagnostic(base, func(d *ExpectedDiagnostic) { d.PatternID = &b })},
		{"message", withDiagnostic(base, func(d *ExpectedDiagnostic) { d.Message = "a" }), withDiagnostic(base, func(d *ExpectedDiagnostic) { d.Message = "b" })},
	}
	for _, pair := range pairs {
		t.Run(pair.name, func(t *testing.T) {
			if compareDiagnostics(pair.a, pair.b) >= 0 {
				t.Fatalf("tie breaker %q did not preserve null-first/direct ordering", pair.name)
			}
		})
	}
}

func TestCaseValidationRejectsObviousFixtureInconsistencies(t *testing.T) {
	tests := []struct {
		name, want string
		mutate     func(*Case)
	}{
		{"source path", "not normalized", func(c *Case) { c.Sources[0].Path = "dir/../main.go" }},
		{"finding path", "unknown or non-normalized", func(c *Case) { c.Expected.Findings[0].Path = "./main.go" }},
		{"parse count path", "not normalized", func(c *Case) { c.Expected.ParseCounts = map[string]int{"./main.go": 1} }},
		{"diagnostic path without range", "unknown or non-normalized", func(c *Case) {
			path, pattern := "missing.go", "rule"
			c.Expected.Diagnostics = []ExpectedDiagnostic{{Code: "SOURCE_PARSE", Class: "source", Severity: "error", Message: "bad", PatternID: &pattern, Path: &path}}
		}},
		{"binding name", "invalid binding", func(c *Case) {
			c.Expected.Findings[0].Bindings["1x"] = c.Expected.Findings[0].Bindings["x"]
			delete(c.Expected.Findings[0].Bindings, "x")
		}},
		{"binding outside finding", "outside finding", func(c *Case) { c.Expected.Findings[0].EndByte = 2; c.Expected.Findings[0].EndColumn = 3 }},
		{"list overlap", "overlap", func(c *Case) {
			b := c.Expected.Findings[0].Bindings["x"]
			b.Kind, b.Ranges = "list", []PositionRange{{StartByte: 0, EndByte: 2, StartLine: 1, StartColumn: 1, EndLine: 1, EndColumn: 3}, {StartByte: 1, EndByte: 3, StartLine: 1, StartColumn: 2, EndLine: 1, EndColumn: 4}}
			b.Structural = json.RawMessage(`[{"node_kind":"identifier","children":[]},{"node_kind":"identifier","children":[]}]`)
			c.Expected.Findings[0].Bindings["x"] = b
		}},
		{"structural shape", "normalized", func(c *Case) {
			b := c.Expected.Findings[0].Bindings["x"]
			b.Structural = json.RawMessage(`{"node_kind":"identifier","lexeme":"abc"}`)
			c.Expected.Findings[0].Bindings["x"] = b
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := validFixtureCase()
			test.mutate(&fixture)
			if err := validateCase(fixture); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validateCase error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestPatternDiagnosticRangeValidation(t *testing.T) {
	patternID := "extra"
	wholeAdditionalPattern := PositionRange{
		StartByte: 0, EndByte: 17, StartLine: 1, StartColumn: 1, EndLine: 2, EndColumn: 4,
	}
	base := validFixtureCase()
	base.Expected.Findings = []ExpectedFinding{}
	base.AdditionalPatterns = []PatternSpec{{
		PatternID: patternID, Message: "match", Pattern: "language go\n`茶`",
	}}
	base.Expected.Diagnostics = []ExpectedDiagnostic{{
		Code: "PATTERN_PARSE", Class: "pattern", Severity: "error", Message: "bad pattern",
		PatternID: &patternID, Range: &wholeAdditionalPattern,
	}}

	if err := validateCase(base); err != nil {
		t.Fatalf("valid additional-pattern diagnostic rejected: %v", err)
	}

	tests := []struct {
		name, want string
		mutate     func(*ExpectedDiagnostic)
	}{
		{"null range", "require pattern_id and range", func(d *ExpectedDiagnostic) { d.Range = nil }},
		{"source path", "must have null path", func(d *ExpectedDiagnostic) { path := "main.go"; d.Path = &path }},
		{"wrong pattern coordinates", "pattern range", func(d *ExpectedDiagnostic) { d.Range.EndColumn = 6 }},
		{"wrong pattern bytes", "pattern range", func(d *ExpectedDiagnostic) { d.Range.EndByte = 18 }},
		{"partial pattern range", "whole submitted pattern", func(d *ExpectedDiagnostic) {
			d.Range.StartByte, d.Range.StartLine, d.Range.StartColumn = 12, 2, 1
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := base
			diagnostic := base.Expected.Diagnostics[0]
			if diagnostic.Range != nil {
				diagnosticRange := *diagnostic.Range
				diagnostic.Range = &diagnosticRange
			}
			fixture.Expected.Diagnostics = []ExpectedDiagnostic{diagnostic}
			test.mutate(&fixture.Expected.Diagnostics[0])
			if err := validateCase(fixture); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validateCase error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestRejectedPathInputsMustBeUnsafe(t *testing.T) {
	fixture := validFixtureCase()
	fixture.RejectedPaths = []string{"dir/../main.go"}
	if err := validateCase(fixture); err == nil || !strings.Contains(err.Error(), "is safe") {
		t.Fatalf("validateCase error = %v, want safe rejected-path rejection", err)
	}

	fixture.RejectedPaths = []string{"../outside.go"}
	if err := validateCase(fixture); err == nil || !strings.Contains(err.Error(), "requires a PATH_INVALID") {
		t.Fatalf("validateCase error = %v, want missing PATH_INVALID rejection", err)
	}
}

func validFixtureCase() Case {
	text := "abc"
	span := PositionRange{StartByte: 0, EndByte: 3, StartLine: 1, StartColumn: 1, EndLine: 1, EndColumn: 4}
	return Case{
		Name: "valid", Description: "valid fixture", Features: []string{"positive"}, PatternID: "rule", Message: "match", Pattern: "language go\n`abc`",
		Sources: []Source{{Path: "main.go", Text: &text}}, Execution: Execution{Mode: "local"},
		Expected: Expected{Findings: []ExpectedFinding{{Path: "main.go", PositionRange: span, PatternID: "rule", Message: "match", Bindings: map[string]Binding{"x": {Kind: "node", Range: &span, Structural: json.RawMessage(`{"node_kind":"identifier","children":[{"token_kind":"IDENT","lexeme":"abc"}]}`)}}}}, Diagnostics: []ExpectedDiagnostic{}},
	}
}

func withFinding(value ExpectedFinding, mutate func(*ExpectedFinding)) ExpectedFinding {
	mutate(&value)
	return value
}
func withDiagnostic(value ExpectedDiagnostic, mutate func(*ExpectedDiagnostic)) ExpectedDiagnostic {
	mutate(&value)
	return value
}
