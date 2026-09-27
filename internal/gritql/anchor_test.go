package gritql

import (
	"context"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestAnalyzeAnchorsRespectsBooleanPolarity(t *testing.T) {
	tests := []struct {
		name       string
		pattern    string
		want       []string
		wantAnchor bool
	}{
		{name: "snippet", pattern: "`required($x)`", want: []string{"required"}, wantAnchor: true},
		{name: "and union", pattern: "and { `left($x)`, `right($x)` }", want: []string{"left", "right"}, wantAnchor: true},
		{name: "or intersection", pattern: "or { `shared(left)`, `shared(right)` }", want: []string{"shared"}, wantAnchor: true},
		{name: "or disjoint", pattern: "or { `left($x)`, `right($x)` }", wantAnchor: false},
		{name: "negative only", pattern: "not `forbidden($x)`", wantAnchor: false},
		{name: "optional only", pattern: "maybe `optional($x)`", wantAnchor: false},
		{name: "contains", pattern: "contains `nested($x)`", want: []string{"nested"}, wantAnchor: true},
		{name: "within", pattern: "within `container($x)`", want: []string{"container"}, wantAnchor: true},
		{name: "where structural", pattern: "`outer($x)` where { $x <: `inner($y)` }", want: []string{"inner", "outer"}, wantAnchor: true},
		{name: "where regex is not source literal", pattern: "`outer($x)` where { $x <: r\"^inner$\" }", want: []string{"outer"}, wantAnchor: true},
		{name: "metavariable only", pattern: "`$x`", wantAnchor: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			program := compileFindingPattern(t, tt.pattern)
			plan := AnalyzeAnchors(program)
			if plan.HasSafeAnchor() != tt.wantAnchor {
				t.Fatalf("HasSafeAnchor()=%v literals=%q", plan.HasSafeAnchor(), plan.RequiredLiterals())
			}
			got := plan.RequiredLiterals()
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("required literals=%q want %q", got, tt.want)
			}
			copyOfGot := append([]string(nil), got...)
			if len(got) != 0 {
				got[0] = "mutated"
			}
			if !reflect.DeepEqual(plan.RequiredLiterals(), copyOfGot) {
				t.Fatal("RequiredLiterals exposed mutable plan state")
			}
		})
	}
}

func TestAnchorPrefilterPreservesEvaluationResults(t *testing.T) {
	patterns := []string{
		"`target($x)`",
		"or { `left($x)`, `right($x)` }",
		"and { contains `target($x)`, not `blocked($x)` }",
		"`outer($x)` where { $x <: `inner($y)` }",
	}
	sources := map[string]string{
		"a.go": "package p\nvar _ = target(inner)\n",
		"b.go": "package p\nvar _ = left(value)\n",
		"c.go": "package p\nvar _ = outer(inner(value))\n",
		"d.go": "package p\nvar _ = unrelated(value)\n",
	}
	for _, pattern := range patterns {
		t.Run(pattern, func(t *testing.T) {
			program := compileFindingPattern(t, pattern)
			all := evaluateSourcesForAnchorTest(t, program, sources, false)
			optimized := evaluateSourcesForAnchorTest(t, program, sources, true)
			if !reflect.DeepEqual(optimized, all) {
				t.Fatalf("optimized=%q all=%q", optimized, all)
			}
		})
	}
}

func evaluateSourcesForAnchorTest(t *testing.T, program *Program, sources map[string]string, optimized bool) []string {
	t.Helper()
	literals := AnalyzeAnchors(program).RequiredLiterals()
	var findings []string
	for path, source := range sources {
		if optimized && !containsAllLiterals(source, literals) {
			continue
		}
		result := EvaluateFile(context.Background(), program, FileInput{Path: path, Language: "go", Content: []byte(source)}, EvaluateOptions{})
		for _, finding := range result.Findings() {
			findings = append(findings, finding.Path()+":"+finding.Text())
		}
	}
	sort.Strings(findings)
	return findings
}

func containsAllLiterals(source string, literals []string) bool {
	for _, literal := range literals {
		if !strings.Contains(source, literal) {
			return false
		}
	}
	return true
}
