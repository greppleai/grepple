package gritql

import (
	"context"
	"encoding/json"
	"runtime"
	"testing"
	"time"

	"github.com/greppleai/grepple/parser"
)

func TestNormalizeEvaluationsAppliesFindingLimitGlobally(t *testing.T) {
	program := compileFindingPattern(t, "`x`")
	options := EvaluateOptions{MaxFindings: 1}
	z := EvaluateFile(context.Background(), program, FileInput{Path: "z.go", Content: []byte("package p\nvar x=2\n"), PatternID: "r", Message: "m"}, options)
	a := EvaluateFile(context.Background(), program, FileInput{Path: "a.go", Content: []byte("package p\nvar x=1\n"), PatternID: "r", Message: "m"}, options)
	if len(a.Findings()) != 1 || len(z.Findings()) != 1 {
		t.Fatal("per-file evaluator applied the globally scoped finding cap")
	}
	result := NormalizeEvaluations(context.Background(), []FileEvaluation{z, a, a}, options)
	findings := result.Findings()
	if len(findings) != 1 || findings[0].Path() != "a.go" {
		t.Fatalf("globally normalized findings=%v", findings)
	}
	diagnostics := result.Diagnostics()
	if len(diagnostics) != 1 || diagnostics[0].Code() != "LIMIT_FINDINGS" {
		t.Fatalf("diagnostics=%v", diagnostics)
	}
	if _, ok := diagnostics[0].Path(); ok {
		t.Fatal("LIMIT_FINDINGS must be pathless")
	}
}

func TestEvaluateDocumentsUsesGlobalCommitPath(t *testing.T) {
	program := compileFindingPattern(t, "`x`")
	makeDocument := func(source string) *parser.Document {
		document, err := parser.ParseDocument("go", source)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(document.Close)
		return document
	}
	result := EvaluateDocuments(context.Background(), program, []ParsedEvaluationInput{
		{Document: makeDocument("package p\nvar x=2\n"), Input: DocumentInput{Path: "z.go", PatternID: "r"}},
		{Document: makeDocument("package p\nvar x=1\n"), Input: DocumentInput{Path: "a.go", PatternID: "r"}},
	}, EvaluateOptions{MaxFindings: 1})
	if findings := result.Findings(); len(findings) != 1 || findings[0].Path() != "a.go" {
		t.Fatalf("findings=%v", findings)
	}
}

func TestMetadataMatchesCanonicalContractAndEveryDefaultLimit(t *testing.T) {
	program := compileFindingPattern(t, "`x`")
	result := NormalizeEvaluations(context.Background(), []FileEvaluation{
		EvaluateFile(context.Background(), program, FileInput{Path: "a.go", Content: []byte("package p\n"), PatternID: "r"}, EvaluateOptions{}),
	}, EvaluateOptions{})
	encoded, err := json.Marshal(result.Metadata())
	if err != nil {
		t.Fatal(err)
	}
	want := `{"contract":"gritql-go-v1","go_grammar":"go1.25","limits":{"pattern_bytes":262144,"source_bytes":10485760,"parse_depth":256,"candidates":250000,"ast_steps":10000000,"findings":10000,"file_time_ms":2000,"batch_time_ms":30000,"memory_bytes":134217728,"regex_bytes":16384,"regex_instructions":100000}}`
	if string(encoded) != want {
		t.Fatalf("metadata=%s\nwant=%s", encoded, want)
	}
	if result.Metadata().TreeSitterGrammar != "tree-sitter-go@0.25.0" {
		t.Fatal("tree-sitter parser pin was not separately reported")
	}
}

func TestDeadlineChecksDiscardBeforeCommit(t *testing.T) {
	program := compileFindingPattern(t, "`x`")
	input := FileInput{Path: "a.go", Content: []byte("package p\nvar x=1\n"), PatternID: "r"}
	file := EvaluateFile(context.Background(), program, input, EvaluateOptions{Deadline: time.Now().Add(-time.Millisecond)})
	if len(file.Findings()) != 0 || len(file.Diagnostics()) != 1 || file.Diagnostics()[0].Code() != "LIMIT_TIME_FILE" {
		t.Fatalf("file deadline result=%v/%v", file.Findings(), file.Diagnostics())
	}
	batch := NormalizeEvaluations(context.Background(), []FileEvaluation{EvaluateFile(context.Background(), program, input, EvaluateOptions{})}, EvaluateOptions{Deadline: time.Now().Add(-time.Millisecond)})
	if len(batch.Findings()) != 0 || len(batch.Diagnostics()) != 1 || batch.Diagnostics()[0].Code() != "LIMIT_TIME_BATCH" {
		t.Fatalf("batch deadline result=%v/%v", batch.Findings(), batch.Diagnostics())
	}
}

func TestBackslashAndColonPathsArePlatformCorrect(t *testing.T) {
	got, ok := normalizeEvaluationPath(`dir\main.go`)
	if !ok {
		t.Fatal("path rejected")
	}
	if runtime.GOOS == "windows" {
		if got != "dir/main.go" {
			t.Fatalf("windows path=%q", got)
		}
		if _, ok := normalizeEvaluationPath("c:main.go"); ok {
			t.Fatal("Windows drive prefix accepted")
		}
	} else {
		if got != `dir\main.go` {
			t.Fatalf("POSIX backslash was treated as a separator: %q", got)
		}
		if got, ok := normalizeEvaluationPath("c:main.go"); !ok || got != "c:main.go" {
			t.Fatalf("POSIX colon path=%q, %v", got, ok)
		}
	}
}

func TestNormalizeEvaluationsCapsEachPatternAndResortsDiagnostics(t *testing.T) {
	program := compileFindingPattern(t, "`x`")
	var files []FileEvaluation
	for _, input := range []FileInput{
		{Path: "b.go", Content: []byte("package p\nvar x=1\n"), PatternID: "z"},
		{Path: "a.go", Content: []byte("package p\nvar x=1\n"), PatternID: "a"},
		{Path: "d.go", Content: []byte("package p\nvar x=1\n"), PatternID: "z"},
		{Path: "c.go", Content: []byte("package p\nvar x=1\n"), PatternID: "a"},
	} {
		files = append(files, EvaluateFile(context.Background(), program, input, EvaluateOptions{}))
	}
	result := NormalizeEvaluations(context.Background(), files, EvaluateOptions{MaxFindings: 1})
	findings := result.Findings()
	if len(findings) != 2 || findings[0].Path() != "a.go" || findings[1].Path() != "b.go" {
		t.Fatalf("per-pattern retained findings=%v", findings)
	}
	diagnostics := result.Diagnostics()
	if len(diagnostics) != 2 {
		t.Fatalf("diagnostics=%v", diagnostics)
	}
	for i, want := range []string{"a", "z"} {
		got, ok := diagnostics[i].PatternID()
		if !ok || got != want || diagnostics[i].Code() != "LIMIT_FINDINGS" {
			t.Fatalf("diagnostic %d=%v", i, diagnostics[i])
		}
		if _, ok := diagnostics[i].Path(); ok {
			t.Fatal("LIMIT_FINDINGS must be pathless")
		}
	}
}

func TestEmptyEvaluateDocumentsUsesProgramCompileMetadata(t *testing.T) {
	program, err := Compile([]byte("language go\n`x`"), CompileOptions{MaxPatternBytes: 1234, MaxRegexBytes: 2345, MaxRegexInstructions: 3456, MaxDepth: 17})
	if err != nil {
		t.Fatal(err)
	}
	result := EvaluateDocuments(context.Background(), program, nil, EvaluateOptions{MaxDepth: 99})
	limits := result.Metadata().Limits
	if limits.PatternBytes != 1234 || limits.RegexBytes != 2345 || limits.RegexInstructions != 3456 || limits.ParseDepth != 17 {
		t.Fatalf("empty batch metadata=%+v", limits)
	}
}

func TestNormalizationCancellationAndTimeoutRetainDeterminedDiagnostics(t *testing.T) {
	program := compileFindingPattern(t, "`x`")
	invalid := EvaluateFile(context.Background(), program, FileInput{Path: "z.go", Content: []byte("package p\nfunc {"), PatternID: "r"}, EvaluateOptions{})
	valid := EvaluateFile(context.Background(), program, FileInput{Path: "a.go", Content: []byte("package p\nvar x=1\n"), PatternID: "r"}, EvaluateOptions{})

	tests := []normalizationFailureCase{
		{"in-flight cancellation", normalizationInFlight, nil, true, "EVALUATION_CANCELLED"},
		{"precommit cancellation", normalizationPrecommit, nil, true, "EVALUATION_CANCELLED"},
		{"in-flight deadline", normalizationInFlight, evaluationFailure("LIMIT_TIME_BATCH", "resource", "batch evaluation deadline exceeded", nil), false, "LIMIT_TIME_BATCH"},
		{"precommit deadline", normalizationPrecommit, evaluationFailure("LIMIT_TIME_BATCH", "resource", "batch evaluation deadline exceeded", nil), false, "LIMIT_TIME_BATCH"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assertNormalizationFailure(t, tc, valid, invalid)
		})
	}
}

type normalizationFailureCase struct {
	name       string
	checkpoint normalizationCheckpoint
	failure    *EvaluationError
	cancel     bool
	wantCode   string
}

func assertNormalizationFailure(t *testing.T, tc normalizationFailureCase, valid, invalid FileEvaluation) {
	t.Helper()
	base, cancel := context.WithCancel(context.Background())
	defer cancel()
	trigger := func(got normalizationCheckpoint) *EvaluationError {
		return tc.failureAt(got, cancel)
	}
	ctx := context.WithValue(base, normalizationCheckpointKey{}, normalizationCheckpointHook(trigger))
	result := NormalizeEvaluations(ctx, []FileEvaluation{valid, invalid, invalid}, EvaluateOptions{})
	if len(result.Findings()) != 0 {
		t.Fatalf("findings committed: %v", result.Findings())
	}
	diagnostics := result.Diagnostics()
	if len(diagnostics) != 2 || diagnostics[0].Code() != tc.wantCode || diagnostics[1].Code() != "SOURCE_PARSE" {
		t.Fatalf("sorted/deduplicated diagnostics=%v", diagnostics)
	}
	if _, ok := diagnostics[0].Path(); ok {
		t.Fatal("batch failure diagnostic must be pathless")
	}
}

func (tc normalizationFailureCase) failureAt(got normalizationCheckpoint, cancel context.CancelFunc) *EvaluationError {
	if got != tc.checkpoint {
		return nil
	}
	if tc.cancel {
		cancel()
		return nil
	}
	return tc.failure
}

func TestNormalizeEvaluationsPromotesFileCancellationToBatchCancellation(t *testing.T) {
	program := compileFindingPattern(t, "`x`")
	valid := EvaluateFile(context.Background(), program, FileInput{Path: "a.go", Content: []byte("package p\nvar x=1\n"), PatternID: "r"}, EvaluateOptions{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cancelled := EvaluateFile(ctx, program, FileInput{Path: "b.go", Content: []byte("package p\nvar x=2\n"), PatternID: "r"}, EvaluateOptions{})
	result := NormalizeEvaluations(context.Background(), []FileEvaluation{valid, cancelled}, EvaluateOptions{})
	if len(result.Findings()) != 0 || len(result.Diagnostics()) != 1 || result.Diagnostics()[0].Code() != "EVALUATION_CANCELLED" {
		t.Fatalf("batch cancellation=%v/%v", result.Findings(), result.Diagnostics())
	}
	if _, ok := result.Diagnostics()[0].Path(); ok {
		t.Fatal("promoted cancellation must be pathless")
	}
}

func TestDisabledWallClockTimeoutsDoNotExpire(t *testing.T) {
	options := normalizeEvaluateOptions(EvaluateOptions{DisableFileTimeout: true, DisableBatchTimeout: true})
	if options.MaxElapsed != 0 || options.MaxBatchElapsed != 0 {
		t.Fatalf("disabled durations=%v/%v", options.MaxElapsed, options.MaxBatchElapsed)
	}
	if failure := batchFailure(context.Background(), time.Now().Add(-time.Hour), options); failure != nil {
		t.Fatalf("disabled batch timeout failed: %v", failure)
	}
	budget := newEvaluationBudget(context.Background(), options)
	if !budget.deadline.IsZero() || !budget.take() {
		t.Fatalf("disabled file timeout budget=%#v", budget)
	}
}
