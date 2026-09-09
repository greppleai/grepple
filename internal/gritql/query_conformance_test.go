package gritql

import (
	"errors"
	"path/filepath"
	"testing"
)

// This test deliberately consumes the parser conformance JSON through the
// existing strict, test-only decoder so every parser fixture reaches the
// production syntax layer while the readiness-gated engine test stays skipped.
func TestQueryParserConformanceFixtures(t *testing.T) {
	t.Parallel()
	fixture, err := decodeFixture(filepath.Join("testdata", "conformance", "parser", "cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, fixtureCase := range fixture.Cases {
		fixtureCase := fixtureCase
		t.Run(fixtureCase.Name, func(t *testing.T) {
			assertQueryParserConformanceCase(t, fixtureCase)
		})
	}
}

func assertQueryParserConformanceCase(t *testing.T, fixtureCase Case) {
	t.Helper()
	doc, parseErr := parseQuery([]byte(fixtureCase.Pattern))
	if doc != nil {
		defer doc.close()
	}
	if len(fixtureCase.Expected.Diagnostics) == 0 {
		if parseErr != nil {
			t.Fatalf("parseQuery: %v", parseErr)
		}
		return
	}
	var syntaxErr *querySyntaxError
	if !errors.As(parseErr, &syntaxErr) {
		t.Fatalf("parseQuery error = %#v, want querySyntaxError", parseErr)
	}
	wantKind := map[string]queryErrorKind{
		"PATTERN_PARSE":           queryMalformed,
		"PATTERN_UNSUPPORTED":     queryUnsupported,
		"PATTERN_INVALID_CONTEXT": queryInvalidContext,
	}[fixtureCase.Expected.Diagnostics[0].Code]
	if syntaxErr.Kind != wantKind {
		t.Fatalf("error kind = %v, want %v", syntaxErr.Kind, wantKind)
	}
	want := fixtureCase.Expected.Diagnostics[0].Range
	if want == nil || syntaxErr.Range.StartByte != want.StartByte || syntaxErr.Range.EndByte != want.EndByte ||
		syntaxErr.Range.Start.Line != want.StartLine || syntaxErr.Range.Start.Column != want.StartColumn ||
		syntaxErr.Range.End.Line != want.EndLine || syntaxErr.Range.End.Column != want.EndColumn {
		t.Fatalf("error range = %+v, want %+v", syntaxErr.Range, want)
	}
}
