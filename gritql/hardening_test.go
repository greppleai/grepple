package gritql

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"testing/fstest"
)

func FuzzCompileNoPanicDeterministic(f *testing.F) {
	for _, query := range []string{
		"language go\n`target($x)`",
		"language go\n`$x` where { $x <: `return $_` }",
		"language typescript\n`target($x)`", "language tsx\n`<Button value={$x} />`",
		"", "language javascript\n`x`", "language go\n`unterminated", "language typescript\n`unterminated",
	} {
		f.Add(query)
	}
	f.Fuzz(func(t *testing.T, query string) {
		if len(query) > 128<<10 {
			t.Skip()
		}
		first, firstErr := Compile([]byte(query), CompileOptions{})
		second, secondErr := Compile([]byte(query), CompileOptions{})
		if errorText(firstErr) != errorText(secondErr) {
			t.Fatalf("non-deterministic errors: %q != %q", errorText(firstErr), errorText(secondErr))
		}
		if (first == nil) != (second == nil) {
			t.Fatalf("non-deterministic programs: first nil=%v second nil=%v", first == nil, second == nil)
		}
		if first != nil && (first.Language() != second.Language() || !equalStrings(AnalyzeAnchors(first).RequiredLiterals(), AnalyzeAnchors(second).RequiredLiterals())) {
			t.Fatal("compiled program metadata is not deterministic")
		}
	})
}

func FuzzCompileSnippetNoPanicDeterministic(f *testing.F) {
	for _, snippet := range []string{"target($x)", "$x + $x", "var $x = $_", "\\`quoted\\`", "func ("} {
		f.Add(snippet)
	}
	f.Fuzz(func(t *testing.T, snippet string) {
		if len(snippet) > 64<<10 {
			t.Skip()
		}
		query := "language go\n`" + snippet + "`"
		first, firstErr := Compile([]byte(query), CompileOptions{})
		second, secondErr := Compile([]byte(query), CompileOptions{})
		if errorText(firstErr) != errorText(secondErr) || (first == nil) != (second == nil) {
			t.Fatalf("non-deterministic snippet compilation: %q / %q", errorText(firstErr), errorText(secondErr))
		}
	})
}

func FuzzBindingRollbackNoPanicDeterministic(f *testing.F) {
	program, err := Compile([]byte("language go\nor { and { `$x + $x`, not `$x + $x` }, `target($x)` }"), CompileOptions{})
	if err != nil {
		f.Fatal(err)
	}
	for _, source := range []string{
		"package p\nvar _ = target(one)\n",
		"package p\nvar _ = one + one\n",
		"package p\nvar _ = target(two) + three\n",
	} {
		f.Add(source)
	}
	f.Fuzz(func(t *testing.T, source string) {
		if len(source) > 64<<10 {
			t.Skip()
		}
		assertDeterministicEvaluation(t, program, []byte(source))
	})
}

func FuzzNestedTraversalNoPanicDeterministic(f *testing.F) {
	for _, seed := range []struct {
		depth  uint8
		source string
	}{{0, "package p\nvar _ = target(1)\n"}, {8, "package p\nfunc f(){ target(target(1)) }\n"}, {32, "package p\nfunc {"}} {
		f.Add(seed.depth, seed.source)
	}
	f.Fuzz(func(t *testing.T, depth uint8, source string) {
		if len(source) > 64<<10 {
			t.Skip()
		}
		query := "`target($x)`"
		operators := []string{"contains ", "maybe ", "not ", "within "}
		for index := 0; index < int(depth%33); index++ {
			query = operators[index%len(operators)] + query
		}
		program, err := Compile([]byte("language go\n"+query), CompileOptions{MaxDepth: 64})
		if err != nil {
			return
		}
		assertDeterministicEvaluation(t, program, []byte(source))
	})
}

func assertDeterministicEvaluation(t *testing.T, program *Program, source []byte) {
	t.Helper()
	options := EvaluateOptions{MaxSourceBytes: 64 << 10, MaxMemoryBytes: 4 << 20, MaxCandidates: 10_000, MaxSteps: 100_000, MaxFindings: 1_000}
	input := FileInput{Path: "fuzz/input.go", Content: source, PatternID: "fuzz"}
	first := EvaluateFile(context.Background(), program, input, options)
	second := EvaluateFile(context.Background(), program, input, options)
	firstJSON, firstMarshalErr := json.Marshal(first)
	secondJSON, secondMarshalErr := json.Marshal(second)
	if firstMarshalErr != nil || secondMarshalErr != nil {
		t.Fatalf("marshal errors: %v, %v", firstMarshalErr, secondMarshalErr)
	}
	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatalf("non-deterministic evaluation:\n%s\n%s", firstJSON, secondJSON)
	}
}

func FuzzEvaluateFileNoPanicDeterministic(f *testing.F) {
	program, err := Compile([]byte("language go\n`target($x)`"), CompileOptions{})
	if err != nil {
		f.Fatal(err)
	}
	for _, source := range [][]byte{
		[]byte("package p\nfunc f() { target(1) }\n"),
		[]byte("package p\nfunc {"),
		{0xff, 0xfe, 0xfd},
		[]byte("package p\n" + strings.Repeat("(", 128)),
	} {
		f.Add(source)
	}
	f.Fuzz(func(t *testing.T, source []byte) {
		if len(source) > 64<<10 {
			t.Skip()
		}
		assertDeterministicEvaluation(t, program, source)
	})
}

func FuzzValidateGlobsNoPanicDeterministic(f *testing.F) {
	for _, glob := range []string{"**/*.go", "src/[ab].go", "[bad", "", "../*.go"} {
		f.Add(glob)
	}
	f.Fuzz(func(t *testing.T, glob string) {
		if len(glob) > 8<<10 {
			t.Skip()
		}
		first := errorText(ValidateGlobs([]string{glob}, nil))
		second := errorText(ValidateGlobs([]string{glob}, nil))
		if first != second {
			t.Fatalf("non-deterministic validation: %q != %q", first, second)
		}
	})
}

func TestScanFilesProgramsRejectsUnsafeReadPathBeforeAcquisition(t *testing.T) {
	program := compileFindingPattern(t, "`x`")
	batch := ScanFilesPrograms(context.Background(), fstest.MapFS{"safe.go": {Data: []byte("package p\nvar x = 1\n")}}, []ProgramScan{
		{Program: program, PatternID: "rule"},
	}, []ScanCandidate{{ReadPath: "../safe.go", Path: "safe.go"}}, ScanOptions{})
	if stats := batch.Stats(); stats.FilesRead != 0 || stats.FilesParsed != 0 {
		t.Fatalf("unsafe candidate was acquired: %+v", stats)
	}
	results := batch.Programs()
	if len(results) != 1 || !batchHasDiagnostic(results[0].Result.Diagnostics(), "INTERNAL_ERROR") {
		t.Fatalf("results=%#v", results)
	}
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
