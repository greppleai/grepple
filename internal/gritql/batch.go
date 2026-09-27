package gritql

import (
	"bytes"
	"context"
	"sort"
	"time"

	"github.com/greppleai/grepple/internal/parser"
)

// ParsedEvaluationInput is one already-parsed source and its finding identity.
// The caller retains ownership of Document and may close it after this call.
type ParsedEvaluationInput struct {
	Document *parser.Document
	Input    DocumentInput
}

// EvaluateDocuments evaluates pre-parsed files in normalized path order and
// commits them through NormalizeEvaluations. It is the scanner-ready batch path;
// each file remains transactional and the finding cap is applied per pattern.
func EvaluateDocuments(ctx context.Context, program *Program, inputs []ParsedEvaluationInput, options EvaluateOptions) FileEvaluation {
	if ctx == nil {
		ctx = context.Background()
	}
	started := time.Now()
	ordered := append([]ParsedEvaluationInput(nil), inputs...)
	sort.SliceStable(ordered, func(i, j int) bool {
		left, leftOK := normalizeEvaluationPath(ordered[i].Input.Path)
		right, rightOK := normalizeEvaluationPath(ordered[j].Input.Path)
		if leftOK != rightOK {
			return !leftOK
		}
		if !leftOK {
			left, right = ordered[i].Input.Path, ordered[j].Input.Path
		}
		return bytes.Compare([]byte(left), []byte(right)) < 0
	})
	results := make([]FileEvaluation, 0, len(ordered))
	for _, input := range ordered {
		if failure := batchFailure(ctx, started, options); failure != nil {
			return failedBatch(evaluationMetadataForProgram(options, program), results, failure)
		}
		results = append(results, EvaluateDocumentFindings(ctx, program, input.Document, input.Input, options))
	}
	return normalizeEvaluationsAt(ctx, results, options, started, program)
}

// NormalizeEvaluations globally sorts, deduplicates, and caps findings from
// transactional per-file results. It is safe to call for one result too.
func NormalizeEvaluations(ctx context.Context, results []FileEvaluation, options EvaluateOptions) FileEvaluation {
	return normalizeEvaluationsAt(ctx, results, options, time.Now(), nil)
}

func normalizeEvaluationsAt(ctx context.Context, results []FileEvaluation, options EvaluateOptions, started time.Time, program *Program) FileEvaluation {
	if ctx == nil {
		ctx = context.Background()
	}
	result := FileEvaluation{metadata: evaluationMetadataForProgram(options, program)}
	mergeEvaluationResults(&result, results, program != nil)
	if failure := normalizationCheckpointFailure(ctx, normalizationInFlight, started, options); failure != nil {
		return failedBatch(result.metadata, results, failure)
	}
	if failure := diagnosedBatchFailure(result.diagnostics); failure != nil {
		return failedBatch(result.metadata, results, failure)
	}
	result.findings = sortAndDeduplicateFindings(result.findings)
	result.findings, result.diagnostics = capFindingsByPattern(result.findings, result.diagnostics, normalizeEvaluateOptions(options).MaxFindings)
	sortAndDeduplicateDiagnostics(&result.diagnostics)

	// Cancellation and the batch deadline invalidate every finding, but do not
	// erase diagnostics that were transactionally determined before the failure.
	if failure := normalizationCheckpointFailure(ctx, normalizationPrecommit, started, options); failure != nil {
		return failedBatch(result.metadata, []FileEvaluation{result}, failure)
	}
	return result
}

func mergeEvaluationResults(result *FileEvaluation, files []FileEvaluation, compileMetadataSet bool) {
	for _, file := range files {
		if !compileMetadataSet && file.metadata.Contract != "" {
			result.metadata.Contract = file.metadata.Contract
			result.metadata.Language = file.metadata.Language
			result.metadata.Grammar = file.metadata.Grammar
			result.metadata.GoGrammar = file.metadata.GoGrammar
			result.metadata.TreeSitterGrammar = file.metadata.TreeSitterGrammar
			result.metadata.Limits.PatternBytes = file.metadata.Limits.PatternBytes
			result.metadata.Limits.RegexBytes = file.metadata.Limits.RegexBytes
			result.metadata.Limits.RegexInstructions = file.metadata.Limits.RegexInstructions
			if file.metadata.Limits.ParseDepth < result.metadata.Limits.ParseDepth {
				result.metadata.Limits.ParseDepth = file.metadata.Limits.ParseDepth
			}
			compileMetadataSet = true
		}
		result.findings = append(result.findings, file.findings...)
		result.diagnostics = append(result.diagnostics, file.diagnostics...)
	}
}

func sortAndDeduplicateFindings(findings []Finding) []Finding {
	sort.Slice(findings, func(i, j int) bool {
		return compareNormalizedFindings(findings[i], findings[j]) < 0
	})
	deduplicated := findings[:0]
	for _, finding := range findings {
		if len(deduplicated) == 0 || !bytes.Equal(deduplicated[len(deduplicated)-1].canonicalJSON, finding.canonicalJSON) {
			deduplicated = append(deduplicated, finding)
		}
	}
	return deduplicated
}

func capFindingsByPattern(findings []Finding, diagnostics []Diagnostic, maxFindings int) ([]Finding, []Diagnostic) {
	// The cap is rule-local. Because the input stream is already in contractual
	// global order, filtering it preserves that order across interleaved rules.
	counts := make(map[string]int)
	truncated := make(map[string]bool)
	retained := findings[:0]
	for _, finding := range findings {
		if counts[finding.patternID] < maxFindings {
			retained = append(retained, finding)
			counts[finding.patternID]++
		} else {
			truncated[finding.patternID] = true
		}
	}
	for patternID := range truncated {
		diagnostics = append(diagnostics, newDiagnostic("LIMIT_FINDINGS", "resource", "rule finding limit reached", stringPtr(patternID), nil, nil))
	}
	return retained, diagnostics
}

func diagnosedBatchFailure(diagnostics []Diagnostic) *EvaluationError {
	for _, diagnostic := range diagnostics {
		if diagnostic.code == "EVALUATION_CANCELLED" {
			return evaluationFailure(diagnostic.code, diagnostic.class, diagnostic.message, nil)
		}
	}
	for _, diagnostic := range diagnostics {
		if diagnostic.code == "LIMIT_TIME_BATCH" {
			return evaluationFailure(diagnostic.code, diagnostic.class, diagnostic.message, nil)
		}
	}
	return nil
}

func batchFailure(ctx context.Context, started time.Time, options EvaluateOptions) *EvaluationError {
	if err := ctx.Err(); err != nil {
		return evaluationFailure("EVALUATION_CANCELLED", "cancelled", "evaluation cancelled", err)
	}
	o := normalizeEvaluateOptions(options)
	deadline := evaluationDeadline(started, o.MaxBatchElapsed, options.Deadline)
	if deadlineExceeded(deadline) {
		return evaluationFailure("LIMIT_TIME_BATCH", "resource", "batch evaluation deadline exceeded", nil)
	}
	return nil
}

func failedBatch(metadata EvaluationMetadata, results []FileEvaluation, failure *EvaluationError) FileEvaluation {
	diagnostics := make([]Diagnostic, 0)
	for _, result := range results {
		for _, diagnostic := range result.diagnostics {
			if diagnostic.code != "EVALUATION_CANCELLED" && diagnostic.code != "LIMIT_TIME_BATCH" {
				diagnostics = append(diagnostics, diagnostic)
			}
		}
	}
	diagnostics = append(diagnostics, newDiagnostic(failure.Code, failure.Class, failure.Message, nil, nil, nil))
	sortAndDeduplicateDiagnostics(&diagnostics)
	return FileEvaluation{metadata: metadata, diagnostics: diagnostics}
}

func sortAndDeduplicateDiagnostics(diagnostics *[]Diagnostic) {
	sort.SliceStable(*diagnostics, func(i, j int) bool {
		return compareNormalizedDiagnostics((*diagnostics)[i], (*diagnostics)[j]) < 0
	})
	out := (*diagnostics)[:0]
	for _, diagnostic := range *diagnostics {
		if len(out) == 0 || !equalDiagnostic(out[len(out)-1], diagnostic) {
			out = append(out, diagnostic)
		}
	}
	*diagnostics = out
}

func equalDiagnostic(a, b Diagnostic) bool {
	if a.code != b.code || a.class != b.class || a.severity != b.severity || a.message != b.message ||
		compareDiagnosticOptionalString(a.patternID, b.patternID) != 0 || compareDiagnosticOptionalString(a.path, b.path) != 0 {
		return false
	}
	if a.rng == nil || b.rng == nil {
		return a.rng == nil && b.rng == nil
	}
	return *a.rng == *b.rng
}

type normalizationCheckpoint uint8

const (
	normalizationInFlight normalizationCheckpoint = iota + 1
	normalizationPrecommit
)

type normalizationCheckpointKey struct{}

// normalizationCheckpointHook is deliberately package-private. Tests use it via

// a context value to deterministically stop normalization without timing sleeps.
type normalizationCheckpointHook func(normalizationCheckpoint) *EvaluationError

func normalizationCheckpointFailure(ctx context.Context, checkpoint normalizationCheckpoint, started time.Time, options EvaluateOptions) *EvaluationError {
	if hook, ok := ctx.Value(normalizationCheckpointKey{}).(normalizationCheckpointHook); ok {
		if failure := hook(checkpoint); failure != nil {
			return failure
		}
	}
	return batchFailure(ctx, started, options)
}

func compareNormalizedDiagnostics(a, b Diagnostic) int {
	if n := compareDiagnosticOptionalString(a.path, b.path); n != 0 {
		return n
	}
	if a.rng == nil || b.rng == nil {
		if a.rng == nil && b.rng != nil {
			return -1
		}
		if a.rng != nil && b.rng == nil {
			return 1
		}
	} else if a.rng.StartByte != b.rng.StartByte {
		if a.rng.StartByte < b.rng.StartByte {
			return -1
		}
		return 1
	}
	if n := bytes.Compare([]byte(a.code), []byte(b.code)); n != 0 {
		return n
	}
	if n := compareDiagnosticOptionalString(a.patternID, b.patternID); n != 0 {
		return n
	}
	return bytes.Compare([]byte(a.message), []byte(b.message))
}

func compareDiagnosticOptionalString(a, b *string) int {
	if a == nil && b != nil {
		return -1
	}
	if a != nil && b == nil {
		return 1
	}
	if a == nil {
		return 0
	}
	return bytes.Compare([]byte(*a), []byte(*b))
}
