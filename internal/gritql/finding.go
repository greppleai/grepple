package gritql

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path"
	"runtime"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/greppleai/grepple/internal/parser"
)

const (
	// GoGrammar is the canonical Go language/grammar identifier in the v1 contract.
	GoGrammar = "go1.25"
	// TreeSitterGoGrammar identifies the independently pinned parser implementation.
	TreeSitterGoGrammar = "tree-sitter-go@0.25.0"

	defaultMaxSourceBytes = 10 << 20
	hardMaxSourceBytes    = 50 << 20
	defaultBatchTime      = 30 * time.Second
	hardBatchTime         = 300 * time.Second
	defaultMaxMemoryBytes = 128 << 20
	hardMaxMemoryBytes    = 512 << 20
)

// FileInput is one in-memory source file. EvaluateFile does not access the
// filesystem. Path is normalized as a repository-relative path and Content is
// parsed without modification. An empty Language is inferred from Path, then
// falls back to Go for backward compatibility.
type FileInput struct {
	Path      string
	Language  string
	Content   []byte
	PatternID string
	Message   string
}

// DocumentInput supplies identity and rule data for an already parsed document.
type DocumentInput struct {
	Path      string
	PatternID string
	Message   string
}

// EffectiveLimits publishes every effective native GritQL resource bound.
type EffectiveLimits struct {
	PatternBytes      int   `json:"pattern_bytes"`
	SourceBytes       int   `json:"source_bytes"`
	ParseDepth        int   `json:"parse_depth"`
	Candidates        int   `json:"candidates"`
	ASTSteps          int   `json:"ast_steps"`
	Findings          int   `json:"findings"`
	FileTimeMillis    int64 `json:"file_time_ms"`
	BatchTimeMillis   int64 `json:"batch_time_ms"`
	MemoryBytes       int   `json:"memory_bytes"`
	RegexBytes        int   `json:"regex_bytes"`
	RegexInstructions int   `json:"regex_instructions"`
}

// EvaluationMetadata identifies the closed contract and effective limits.
// GoGrammar remains populated for the original Go wire contract.
type EvaluationMetadata struct {
	Contract          string          `json:"contract"`
	Language          string          `json:"language,omitempty"`
	Grammar           string          `json:"grammar,omitempty"`
	GoGrammar         string          `json:"go_grammar,omitempty"`
	Limits            EffectiveLimits `json:"limits"`
	TreeSitterGrammar string          `json:"-"`
}

// Finding is an immutable normalized structural finding. All slice-returning
// accessors return fresh copies.
type Finding struct {
	path          string
	language      string
	compatibility string
	grammar       string
	rng           parser.Range
	text          string
	patternID     string
	message       string
	bindings      []FindingBinding
	bindingJSON   []byte
	canonicalJSON []byte
}

// Path returns the normalized repository-relative source path.
func (f Finding) Path() string { return f.path }

// Language returns the source language identifier.
func (f Finding) Language() string { return f.language }

// Compatibility returns the structural-search compatibility identifier.
func (f Finding) Compatibility() string { return f.compatibility }

// Grammar returns the target-language grammar identifier.
func (f Finding) Grammar() string { return f.grammar }

// Range returns the complete matched source range.
func (f Finding) Range() parser.Range { return f.rng }

// StartByte returns the zero-based inclusive byte offset of the match.
func (f Finding) StartByte() int { return f.rng.StartByte }

// EndByte returns the zero-based exclusive byte offset of the match.
func (f Finding) EndByte() int { return f.rng.EndByte }

// Start returns the match's starting source position.
func (f Finding) Start() parser.Position { return f.rng.Start }

// End returns the match's exclusive ending source position.
func (f Finding) End() parser.Position { return f.rng.End }

// Text returns the exact source bytes covered by the finding as a string.
func (f Finding) Text() string { return f.text }

// PatternID returns the caller-supplied identifier for the matching pattern.
func (f Finding) PatternID() string { return f.patternID }

// Message returns the caller-supplied message associated with the pattern.
func (f Finding) Message() string { return f.message }

// Bindings returns a copy of the finding's name-sorted metavariable bindings.
func (f Finding) Bindings() []FindingBinding {
	out := make([]FindingBinding, len(f.bindings))
	copy(out, f.bindings)
	return out
}

// MarshalJSON returns the finding's canonical deterministic JSON representation.
func (f Finding) MarshalJSON() ([]byte, error) {
	if f.canonicalJSON != nil {
		return append([]byte(nil), f.canonicalJSON...), nil
	}
	return json.Marshal(findingJSONValue(f))
}

// FindingBinding is one immutable, name-sorted metavariable binding.
type FindingBinding struct {
	name       string
	kind       BindingKind
	rng        parser.Range
	ranges     []parser.Range
	structural []StructuralNode
}

// Name returns the metavariable name without its leading dollar sign.
func (b FindingBinding) Name() string { return b.name }

// Kind identifies whether the binding contains one node or a list of nodes.
func (b FindingBinding) Kind() BindingKind { return b.kind }

// Range returns the full source span of the binding.
func (b FindingBinding) Range() parser.Range { return b.rng }

// Ranges returns a copy of the element-aligned source ranges.
func (b FindingBinding) Ranges() []parser.Range {
	out := make([]parser.Range, len(b.ranges))
	copy(out, b.ranges)
	return out
}

// Structural returns a copy of the normalized, element-aligned structural values.
func (b FindingBinding) Structural() []StructuralNode {
	out := make([]StructuralNode, len(b.structural))
	copy(out, b.structural)
	return out
}

// MarshalJSON returns the binding's normalized node-or-list JSON representation.
func (b FindingBinding) MarshalJSON() ([]byte, error) { return marshalBinding(b), nil }

// Node returns the single normalized value of a node binding.
func (b FindingBinding) Node() (StructuralNode, bool) {
	if b.kind != BindingNode || len(b.structural) != 1 {
		return StructuralNode{}, false
	}
	return b.structural[0], true
}

// Elements returns normalized list elements in source order.
func (b FindingBinding) Elements() []StructuralNode {
	if b.kind != BindingList {
		return []StructuralNode{}
	}
	return b.Structural()
}

// Diagnostic is an immutable typed single-file evaluation diagnostic.
type Diagnostic struct {
	code, class, severity, message string
	patternID, path                *string
	rng                            *parser.Range
}

// Code returns the stable machine-readable diagnostic code.
func (d Diagnostic) Code() string { return d.code }

// Class returns the diagnostic category.
func (d Diagnostic) Class() string { return d.class }

// Severity returns the diagnostic severity.
func (d Diagnostic) Severity() string { return d.severity }

// Message returns the human-readable diagnostic detail.
func (d Diagnostic) Message() string { return d.message }

// PatternID returns the associated pattern identifier when one is available.
func (d Diagnostic) PatternID() (string, bool) {
	if d.patternID == nil {
		return "", false
	}
	return *d.patternID, true
}

// Path returns the associated normalized source path when one is available.
func (d Diagnostic) Path() (string, bool) {
	if d.path == nil {
		return "", false
	}
	return *d.path, true
}

// Range returns the associated source range when one is available.
func (d Diagnostic) Range() (parser.Range, bool) {
	if d.rng == nil {
		return parser.Range{}, false
	}
	return *d.rng, true
}

// MarshalJSON returns the diagnostic's stable wire representation with nullable context fields.
func (d Diagnostic) MarshalJSON() ([]byte, error) {
	var r *rangeJSON
	if d.rng != nil {
		value := rangeJSONValue(*d.rng)
		r = &value
	}
	return json.Marshal(diagnosticJSON{d.code, d.class, d.severity, d.message, d.patternID, d.path, r})
}

// FileEvaluation is a transactional result. A failed file has no findings.
type FileEvaluation struct {
	findings    []Finding
	diagnostics []Diagnostic
	metadata    EvaluationMetadata
}

// Findings returns a copy of the successfully committed findings.
func (r FileEvaluation) Findings() []Finding {
	out := make([]Finding, len(r.findings))
	copy(out, r.findings)
	return out
}

// Diagnostics returns a copy of the evaluation diagnostics.
func (r FileEvaluation) Diagnostics() []Diagnostic {
	out := make([]Diagnostic, len(r.diagnostics))
	copy(out, r.diagnostics)
	return out
}

// Metadata returns the compatibility, grammar, and effective-limit metadata.
func (r FileEvaluation) Metadata() EvaluationMetadata { return r.metadata }

// MarshalJSON returns deterministic evaluation JSON with non-null finding and diagnostic arrays.
func (r FileEvaluation) MarshalJSON() ([]byte, error) {
	findings := r.findings
	if findings == nil {
		findings = []Finding{}
	}
	diagnostics := r.diagnostics
	if diagnostics == nil {
		diagnostics = []Diagnostic{}
	}
	return json.Marshal(struct {
		Findings    []Finding          `json:"findings"`
		Diagnostics []Diagnostic       `json:"diagnostics"`
		Metadata    EvaluationMetadata `json:"metadata"`
	}{findings, diagnostics, r.metadata})
}

func evaluationMetadata(options EvaluateOptions) EvaluationMetadata {
	o := normalizeEvaluateOptions(options)
	return EvaluationMetadata{
		Contract: Compatibility, GoGrammar: GoGrammar, TreeSitterGrammar: TreeSitterGoGrammar,
		Limits: EffectiveLimits{
			PatternBytes: defaultMaxPatternBytes, SourceBytes: o.MaxSourceBytes, ParseDepth: o.MaxDepth,
			Candidates: o.MaxCandidates, ASTSteps: o.MaxSteps, Findings: o.MaxFindings,
			FileTimeMillis: o.MaxElapsed.Milliseconds(), BatchTimeMillis: o.MaxBatchElapsed.Milliseconds(),
			MemoryBytes: o.MaxMemoryBytes, RegexBytes: defaultMaxRegexBytes, RegexInstructions: defaultMaxRegexInstructions,
		},
	}
}

func evaluationMetadataForProgram(options EvaluateOptions, program *Program) EvaluationMetadata {
	metadata := evaluationMetadata(options)
	if program != nil {
		metadata.Contract = program.Compatibility()
		metadata.Limits.PatternBytes = program.compileLimits.MaxPatternBytes
		metadata.Limits.RegexBytes = program.compileLimits.MaxRegexBytes
		metadata.Limits.RegexInstructions = program.compileLimits.MaxRegexInstructions
		if program.compileLimits.MaxDepth < metadata.Limits.ParseDepth {
			metadata.Limits.ParseDepth = program.compileLimits.MaxDepth
		}
		if adapter, ok := targetLanguageByID(program.Language()); ok {
			metadata.Language = adapter.metadataLanguage
			metadata.Grammar = adapter.metadataGrammar
			metadata.GoGrammar = adapter.goGrammar
			metadata.TreeSitterGrammar = adapter.treeSitter
		}
	}
	return metadata
}
func fileInputLanguage(input FileInput) string {
	if input.Language != "" {
		return input.Language
	}
	if language := parser.LanguageFor(input.Path); language != "" {
		if _, supported := targetLanguageByID(language); supported {
			return language
		}
	}
	return defaultTargetLanguage
}

func withFileDeadline(started time.Time, options EvaluateOptions) EvaluateOptions {
	normalized := normalizeEvaluateOptions(options)
	options.Deadline = evaluationDeadline(started, normalized.MaxElapsed, options.Deadline)
	return options
}

// EvaluateFile parses and evaluates one in-memory file. Expected source,
// cancellation, and resource failures are returned as typed diagnostics rather
// than Go errors. The result is deterministic and contains no partial findings.
func EvaluateFile(ctx context.Context, program *Program, input FileInput, options EvaluateOptions) (result FileEvaluation) {
	started := time.Now()
	language := fileInputLanguage(input)
	options = withFileDeadline(started, options)
	result.metadata = evaluationMetadataForProgram(options, program)
	defer func() {
		finalizeFileEvaluation(ctx, &result, recover(), options, input)
	}()
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		result.diagnostics = []Diagnostic{newDiagnostic("EVALUATION_CANCELLED", "cancelled", "evaluation cancelled", stringPtr(input.PatternID), nil, nil)}
		return result
	}
	normalizedPath, ok := normalizeEvaluationPath(input.Path)
	if !ok {
		result.diagnostics = []Diagnostic{newDiagnostic("PATH_INVALID", "source", "source path is not a safe repository-relative path", stringPtr(input.PatternID), nil, nil)}
		return result
	}
	if program == nil || program.root == nil {
		result.diagnostics = []Diagnostic{newDiagnostic("INTERNAL_ERROR", "internal", "invalid compiled program", stringPtr(input.PatternID), &normalizedPath, nil)}
		return result
	}
	if _, supported := targetLanguageByID(language); !supported || program.Language() != language {
		result.diagnostics = []Diagnostic{newDiagnostic("SOURCE_PARSE", "source", "source language must match the compiled program", stringPtr(input.PatternID), &normalizedPath, nil)}
		return result
	}
	maxSourceBytes := normalizeEvaluateOptions(options).MaxSourceBytes
	if len(input.Content) > maxSourceBytes {
		result.diagnostics = []Diagnostic{newDiagnostic("LIMIT_SOURCE_BYTES", "resource", "source exceeds effective byte limit", stringPtr(input.PatternID), &normalizedPath, nil)}
		return result
	}
	if !utf8.Valid(input.Content) {
		result.diagnostics = []Diagnostic{newDiagnostic("SOURCE_INVALID_UTF8", "source", "source is not valid UTF-8", stringPtr(input.PatternID), &normalizedPath, nil)}
		return result
	}
	document, err := parser.ParseDocument(language, string(input.Content))
	if err != nil {
		result.diagnostics = []Diagnostic{newDiagnostic("INTERNAL_ERROR", "internal", "source parser failed", stringPtr(input.PatternID), &normalizedPath, nil)}
		return result
	}
	defer document.Close()
	return evaluateNormalizedDocument(ctx, program, document, DocumentInput{Path: normalizedPath, PatternID: input.PatternID, Message: input.Message}, options)
}

func finalizeFileEvaluation(ctx context.Context, result *FileEvaluation, recovered any, options EvaluateOptions, input FileInput) {
	if recovered != nil {
		result.findings = nil
		result.diagnostics = []Diagnostic{newDiagnostic("INTERNAL_ERROR", "internal", "unexpected evaluator failure", stringPtr(input.PatternID), nil, nil)}
		return
	}
	failure := precommitFailure(ctx, options)
	if failure == nil {
		return
	}
	result.findings = nil
	var diagnosticPath *string
	if normalized, ok := normalizeEvaluationPath(input.Path); ok {
		diagnosticPath = &normalized
	}
	result.diagnostics = []Diagnostic{newDiagnostic(failure.Code, failure.Class, failure.Message, stringPtr(input.PatternID), diagnosticPath, nil)}
}

// EvaluateDocumentFindings normalizes findings for one already parsed document.
// Ownership of document remains with the caller.
func EvaluateDocumentFindings(ctx context.Context, program *Program, document *parser.Document, input DocumentInput, options EvaluateOptions) (result FileEvaluation) {
	started := time.Now()
	options = withFileDeadline(started, options)
	result.metadata = evaluationMetadataForProgram(options, program)
	defer func() {
		if recover() != nil {
			result.findings = nil
			result.diagnostics = []Diagnostic{newDiagnostic("INTERNAL_ERROR", "internal", "unexpected evaluator failure", stringPtr(input.PatternID), nil, nil)}
			return
		}
		if failure := precommitFailure(ctx, options); failure != nil {
			result.findings = nil
			var diagnosticPath *string
			if normalized, ok := normalizeEvaluationPath(input.Path); ok {
				diagnosticPath = &normalized
			}
			result.diagnostics = []Diagnostic{newDiagnostic(failure.Code, failure.Class, failure.Message, stringPtr(input.PatternID), diagnosticPath, nil)}
		}
	}()
	return evaluateNormalizedDocument(ctx, program, document, input, options)
}

func evaluateNormalizedDocument(ctx context.Context, program *Program, document *parser.Document, input DocumentInput, options EvaluateOptions) FileEvaluation {
	language := ""
	if document != nil {
		language = document.Language()
	}
	result := FileEvaluation{metadata: evaluationMetadataForProgram(options, program)}
	if ctx == nil {
		ctx = context.Background()
	}
	normalizedPath, diagnostics := validateNormalizedDocumentInput(ctx, program, document, language, input, options)
	if diagnostics != nil {
		result.diagnostics = diagnostics
		return result
	}
	if diagnostics := sourceParseDiagnostics(document, input.PatternID, normalizedPath); diagnostics != nil {
		result.diagnostics = diagnostics
		return result
	}
	matches, err := Evaluate(ctx, program, document, options)
	if err != nil {
		result.diagnostics = documentEvaluationErrorDiagnostics(err, input.PatternID, normalizedPath)
		return result
	}
	if err := ctx.Err(); err != nil {
		result.diagnostics = []Diagnostic{newDiagnostic("EVALUATION_CANCELLED", "cancelled", "evaluation cancelled", stringPtr(input.PatternID), &normalizedPath, nil)}
		return result
	}
	source := document.Source()
	findings := make([]Finding, 0, len(matches))
	for _, match := range matches {
		findings = append(findings, normalizeFinding(normalizedPath, language, input.PatternID, input.Message, source, match))
	}
	// Per-file evaluation is transactional but intentionally does not sort,
	// deduplicate, or cap globally scoped findings. NormalizeEvaluations is the
	// single commit path used by batch/scanner callers.
	result.findings = findings
	if failure := precommitFailure(ctx, options); failure != nil {
		result.findings = nil
		result.diagnostics = []Diagnostic{newDiagnostic(failure.Code, failure.Class, failure.Message, stringPtr(input.PatternID), &normalizedPath, nil)}
	}
	return result
}

func validateNormalizedDocumentInput(ctx context.Context, program *Program, document *parser.Document, language string, input DocumentInput, options EvaluateOptions) (string, []Diagnostic) {
	normalizedPath, ok := normalizeEvaluationPath(input.Path)
	if !ok {
		return "", []Diagnostic{newDiagnostic("PATH_INVALID", "source", "source path is not a safe repository-relative path", stringPtr(input.PatternID), nil, nil)}
	}
	if err := ctx.Err(); err != nil {
		return "", []Diagnostic{newDiagnostic("EVALUATION_CANCELLED", "cancelled", "evaluation cancelled", stringPtr(input.PatternID), &normalizedPath, nil)}
	}
	if document == nil || language == "" {
		return "", []Diagnostic{newDiagnostic("INTERNAL_ERROR", "internal", "invalid or closed source document", stringPtr(input.PatternID), &normalizedPath, nil)}
	}
	if program == nil || program.root == nil {
		return "", []Diagnostic{newDiagnostic("INTERNAL_ERROR", "internal", "invalid compiled program", stringPtr(input.PatternID), &normalizedPath, nil)}
	}
	if _, supported := targetLanguageByID(language); !supported || program.Language() != language {
		return "", []Diagnostic{newDiagnostic("SOURCE_PARSE", "source", "source language must match the compiled program", stringPtr(input.PatternID), &normalizedPath, nil)}
	}
	maxSourceBytes := normalizeEvaluateOptions(options).MaxSourceBytes
	if len(document.Source()) > maxSourceBytes {
		return "", []Diagnostic{newDiagnostic("LIMIT_SOURCE_BYTES", "resource", "source exceeds effective byte limit", stringPtr(input.PatternID), &normalizedPath, nil)}
	}
	return normalizedPath, nil
}

func sourceParseDiagnostics(document *parser.Document, patternID, normalizedPath string) []Diagnostic {
	parseDiagnostics := document.ParseDiagnostics()
	if len(parseDiagnostics) == 0 {
		return nil
	}
	sort.SliceStable(parseDiagnostics, func(i, j int) bool {
		if parseDiagnostics[i].Range.StartByte != parseDiagnostics[j].Range.StartByte {
			return parseDiagnostics[i].Range.StartByte < parseDiagnostics[j].Range.StartByte
		}
		return bytes.Compare([]byte(parseDiagnostics[i].Message), []byte(parseDiagnostics[j].Message)) < 0
	})
	first := parseDiagnostics[0]
	return []Diagnostic{newDiagnostic("SOURCE_PARSE", "source", "source parse failure: "+first.Message, stringPtr(patternID), &normalizedPath, &first.Range)}
}

func documentEvaluationErrorDiagnostics(err error, patternID, normalizedPath string) []Diagnostic {
	var evaluationErr *EvaluationError
	if errors.As(err, &evaluationErr) {
		return []Diagnostic{newDiagnostic(evaluationErr.Code, evaluationErr.Class, evaluationErr.Message, stringPtr(patternID), &normalizedPath, nil)}
	}
	return []Diagnostic{newDiagnostic("INTERNAL_ERROR", "internal", "unexpected evaluator failure", stringPtr(patternID), &normalizedPath, nil)}
}

func normalizeFinding(pathValue, language, patternID, message, source string, match EvaluationMatch) Finding {
	adapter, _ := targetLanguageByID(language)
	f := Finding{path: pathValue, language: language, compatibility: Compatibility, grammar: adapter.grammar, rng: match.Range(), patternID: patternID, message: message}
	if f.rng.StartByte >= 0 && f.rng.EndByte >= f.rng.StartByte && f.rng.EndByte <= len(source) {
		f.text = strings.Clone(source[f.rng.StartByte:f.rng.EndByte])
	}
	for _, value := range match.Bindings().All() {
		variable := value.Variable()
		name := strings.TrimPrefix(variable.Name, "$")
		f.bindings = append(f.bindings, FindingBinding{name: name, kind: value.Kind(), rng: value.Range(), ranges: value.Ranges(), structural: value.Structural()})
	}
	sort.Slice(f.bindings, func(i, j int) bool { return bytes.Compare([]byte(f.bindings[i].name), []byte(f.bindings[j].name)) < 0 })
	f.bindingJSON = marshalBindings(f.bindings)
	f.canonicalJSON, _ = json.Marshal(findingJSONValue(f))
	return f
}

func compareNormalizedFindings(a, b Finding) int {
	if n := bytes.Compare([]byte(a.path), []byte(b.path)); n != 0 {
		return n
	}
	if a.rng.StartByte != b.rng.StartByte {
		if a.rng.StartByte < b.rng.StartByte {
			return -1
		}
		return 1
	}
	if a.rng.EndByte != b.rng.EndByte {
		if a.rng.EndByte < b.rng.EndByte {
			return -1
		}
		return 1
	}
	if n := bytes.Compare([]byte(a.patternID), []byte(b.patternID)); n != 0 {
		return n
	}
	if n := bytes.Compare([]byte(a.message), []byte(b.message)); n != 0 {
		return n
	}
	if n := bytes.Compare(a.bindingJSON, b.bindingJSON); n != 0 {
		return n
	}
	return bytes.Compare(a.canonicalJSON, b.canonicalJSON)
}

func normalizeEvaluationPath(input string) (string, bool) {
	// Slash is always the repository separator. A backslash is a separator only
	// on Windows; on POSIX it is a valid filename byte and must be retained.
	if runtime.GOOS == "windows" {
		input = strings.ReplaceAll(input, `\`, "/")
	}
	if input == "" || strings.ContainsRune(input, 0) || strings.HasPrefix(input, "/") || runtime.GOOS == "windows" && len(input) >= 2 && input[1] == ':' {
		return "", false
	}
	cleaned := path.Clean(input)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", false
	}
	return cleaned, true
}

func precommitFailure(ctx context.Context, options EvaluateOptions) *EvaluationError {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return evaluationFailure("EVALUATION_CANCELLED", "cancelled", "evaluation cancelled", err)
	}
	if !options.Deadline.IsZero() && !time.Now().Before(options.Deadline) {
		return evaluationFailure("LIMIT_TIME_FILE", "resource", "per-file evaluation deadline exceeded", nil)
	}
	return nil
}

func stringPtr(value string) *string { valueCopy := value; return &valueCopy }
func newDiagnostic(code, class, message string, patternID, path *string, rng *parser.Range) Diagnostic {
	return Diagnostic{code: code, class: class, severity: "error", message: message, patternID: patternID, path: path, rng: rng}
}
