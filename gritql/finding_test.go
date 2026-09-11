package gritql

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/greppleai/grepple/parser"
)

func compileFindingPattern(t *testing.T, body string) *Program {
	t.Helper()
	program, err := Compile([]byte("language go\n"+body), CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return program
}

func TestEvaluateFileNormalizesUTF8CRLFRangesTextAndBindings(t *testing.T) {
	t.Parallel()
	source := []byte("package p\r\nvar _ = f(茶)\r\n")
	program := compileFindingPattern(t, "`f($z)` where { $z <: `$a` }")
	result := EvaluateFile(context.Background(), program, FileInput{Path: "dir/main.go", Content: source, PatternID: "rule", Message: "message"}, EvaluateOptions{})
	if diagnostics := result.Diagnostics(); len(diagnostics) != 0 {
		t.Fatalf("diagnostics=%v", diagnostics)
	}
	findings := result.Findings()
	if len(findings) != 1 {
		t.Fatalf("findings=%d", len(findings))
	}
	finding := findings[0]
	if finding.Path() != "dir/main.go" || finding.Text() != "f(茶)" {
		t.Fatalf("finding path/text=%q/%q", finding.Path(), finding.Text())
	}
	r := finding.Range()
	if r.Start.Line != 2 || r.Start.Column != 9 || r.End.Line != 2 || r.End.Column != 13 {
		t.Fatalf("range=%+v", r)
	}
	bindings := finding.Bindings()
	if len(bindings) != 2 || bindings[0].Name() != "a" || bindings[1].Name() != "z" {
		t.Fatalf("binding order=%v", bindings)
	}
	if bindings[1].Range().EndByte-bindings[1].Range().StartByte != len("茶") {
		t.Fatalf("UTF-8 binding range=%+v", bindings[1].Range())
	}
	copyBindings := finding.Bindings()
	copyBindings[0] = FindingBinding{}
	if finding.Bindings()[0].Name() != "a" {
		t.Fatal("binding accessor exposed mutable storage")
	}
	metadata := result.Metadata()
	if metadata.Contract != Compatibility || metadata.GoGrammar != GoGrammar || metadata.TreeSitterGrammar != TreeSitterGoGrammar {
		t.Fatalf("metadata=%+v", metadata)
	}
}

func TestEvaluateFileDeduplicatesExactRecordsAndCompositionRange(t *testing.T) {
	t.Parallel()
	source := []byte("package p\nfunc f(){ pair(a,b) }\n")
	program := compileFindingPattern(t, "or { and { contains `a`, contains `b` }, and { contains `a`, contains `b` } }")
	result := EvaluateFile(context.Background(), program, FileInput{Path: "main.go", Content: source, PatternID: "p", Message: "m"}, EvaluateOptions{})
	findings := result.Findings()
	if len(findings) != 1 {
		t.Fatalf("deduplicated findings=%d: %v", len(findings), findings)
	}
	if findings[0].Text() != "a,b" {
		t.Fatalf("composition text=%q range=%+v", findings[0].Text(), findings[0].Range())
	}
}

func TestEvaluateFileParseFailureIsOneRangedDiagnosticAndNoFindings(t *testing.T) {
	t.Parallel()
	program := compileFindingPattern(t, "`x`")
	result := EvaluateFile(context.Background(), program, FileInput{Path: "main.go", Content: []byte("package p\nfunc f(\n"), PatternID: "p"}, EvaluateOptions{})
	if len(result.Findings()) != 0 {
		t.Fatal("parse failure returned findings")
	}
	diagnostics := result.Diagnostics()
	if len(diagnostics) != 1 || diagnostics[0].Code() != "SOURCE_PARSE" || diagnostics[0].Class() != "source" {
		t.Fatalf("diagnostics=%v", diagnostics)
	}
	if _, ok := diagnostics[0].Range(); !ok {
		t.Fatal("source parse diagnostic has no exact range")
	}
}

func TestEvaluateFileCancellationAndLimitsAreTransactional(t *testing.T) {
	t.Parallel()
	program := compileFindingPattern(t, "`x`")
	input := FileInput{Path: "main.go", Content: []byte("package p\nvar _ = x\n"), PatternID: "p"}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cancelled := EvaluateFile(ctx, program, input, EvaluateOptions{})
	if len(cancelled.Findings()) != 0 || len(cancelled.Diagnostics()) != 1 || cancelled.Diagnostics()[0].Code() != "EVALUATION_CANCELLED" {
		t.Fatalf("cancelled=%v/%v", cancelled.Findings(), cancelled.Diagnostics())
	}
	limited := EvaluateFile(context.Background(), program, input, EvaluateOptions{MaxCandidates: 1})
	if len(limited.Findings()) != 0 || len(limited.Diagnostics()) != 1 || limited.Diagnostics()[0].Code() != "LIMIT_CANDIDATES" {
		t.Fatalf("limited=%v/%v", limited.Findings(), limited.Diagnostics())
	}
}

func TestEvaluateFileConcurrentSerializationIsByteIdentical(t *testing.T) {
	program := compileFindingPattern(t, "`pair($z, $a)`")
	input := FileInput{Path: "main.go", Content: []byte("package p\nfunc f(){ pair(left,right) }\n"), PatternID: "p", Message: "m"}
	const runs = 32
	outputs := make([]string, runs)
	var wg sync.WaitGroup
	for i := range outputs {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			result := EvaluateFile(context.Background(), program, input, EvaluateOptions{})
			encoded, err := json.Marshal(result)
			if err != nil {
				t.Errorf("marshal: %v", err)
				return
			}
			outputs[index] = string(encoded)
		}(i)
	}
	wg.Wait()
	for i := 1; i < len(outputs); i++ {
		if outputs[i] != outputs[0] {
			t.Fatalf("run %d differs\n%s\n%s", i, outputs[0], outputs[i])
		}
	}
}

func TestFindingOrderingUsesCompleteTieBreakers(t *testing.T) {
	baseRange := parser.Range{StartByte: 1, EndByte: 2, Start: parser.Position{Line: 1, Column: 2}, End: parser.Position{Line: 1, Column: 3}}
	makeFinding := func(pattern, message, binding string) Finding {
		f := Finding{path: "a.go", rng: baseRange, patternID: pattern, message: message, bindings: []FindingBinding{{name: binding, kind: BindingList, rng: baseRange}}}
		f.bindingJSON = marshalBindings(f.bindings)
		f.canonicalJSON, _ = json.Marshal(findingJSONValue(f))
		return f
	}
	if compareNormalizedFindings(makeFinding("a", "z", "a"), makeFinding("b", "a", "a")) >= 0 {
		t.Fatal("pattern ID tie breaker ignored")
	}
	if compareNormalizedFindings(makeFinding("a", "a", "a"), makeFinding("a", "b", "a")) >= 0 {
		t.Fatal("message tie breaker ignored")
	}
	if compareNormalizedFindings(makeFinding("a", "a", "a"), makeFinding("a", "a", "b")) >= 0 {
		t.Fatal("binding tie breaker ignored")
	}
}
