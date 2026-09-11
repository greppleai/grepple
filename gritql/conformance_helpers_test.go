package gritql

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"unicode/utf8"
)

// ConformanceSuites is the fixed set of corpus groups.
var ConformanceSuites = []string{"parser", "compiler", "matcher", "scanner", "api", "integration"}

// Corpus is a strictly decoded gritql-go-v1 conformance corpus.
type Corpus struct {
	Files []FixtureFile
	Cases []Case
}

type FixtureFile struct {
	SchemaVersion int        `json:"schema_version"`
	Suite         string     `json:"suite"`
	Provenance    Provenance `json:"provenance"`
	Cases         []Case     `json:"cases"`
}

type Provenance struct {
	Kind string `json:"kind"`
	Note string `json:"note"`
}

type Case struct {
	Name               string        `json:"name"`
	Description        string        `json:"description"`
	Features           []string      `json:"features"`
	PatternID          string        `json:"pattern_id"`
	Message            string        `json:"message"`
	Pattern            string        `json:"pattern"`
	AdditionalPatterns []PatternSpec `json:"additional_patterns,omitempty"`
	Sources            []Source      `json:"sources"`
	RejectedPaths      []string      `json:"rejected_paths,omitempty"`
	Execution          Execution     `json:"execution"`
	Expected           Expected      `json:"expected"`

	Suite      string     `json:"-"`
	Provenance Provenance `json:"-"`
}

// PatternSpec adds another rule to a batch; the primary pattern remains on Case
// so single-rule fixtures stay compact.
type PatternSpec struct {
	PatternID string `json:"pattern_id"`
	Message   string `json:"message"`
	Pattern   string `json:"pattern"`
}

type Source struct {
	Path        string  `json:"path"`
	Text        *string `json:"text,omitempty"`
	BytesBase64 *string `json:"bytes_base64,omitempty"`
}

type Execution struct {
	Mode              string `json:"mode"`
	Cancel            string `json:"cancel,omitempty"`
	Fault             string `json:"fault,omitempty"`
	RequireParseOnce  bool   `json:"require_parse_once,omitempty"`
	TextCompatibility bool   `json:"text_compatibility,omitempty"`
	Limits            Limits `json:"limits,omitempty"`
}

type Limits struct {
	PatternBytes      *int64 `json:"pattern_bytes,omitempty"`
	SourceBytes       *int64 `json:"source_bytes,omitempty"`
	ParseDepth        *int64 `json:"parse_depth,omitempty"`
	Candidates        *int64 `json:"candidates,omitempty"`
	ASTSteps          *int64 `json:"ast_steps,omitempty"`
	Findings          *int64 `json:"findings,omitempty"`
	FileTimeMillis    *int64 `json:"file_time_ms,omitempty"`
	BatchTimeMillis   *int64 `json:"batch_time_ms,omitempty"`
	MemoryBytes       *int64 `json:"memory_bytes,omitempty"`
	RegexBytes        *int64 `json:"regex_bytes,omitempty"`
	RegexInstructions *int64 `json:"regex_instructions,omitempty"`
}

type Expected struct {
	Findings    []ExpectedFinding    `json:"findings"`
	Diagnostics []ExpectedDiagnostic `json:"diagnostics"`
	Metadata    *Metadata            `json:"metadata,omitempty"`
	Text        *string              `json:"text,omitempty"`
	ParseCounts map[string]int       `json:"parse_counts,omitempty"`
}

type Metadata struct {
	Contract  string `json:"contract"`
	GoGrammar string `json:"go_grammar"`
	Limits    Limits `json:"limits"`
}

type PositionRange struct {
	StartByte   int `json:"start_byte"`
	EndByte     int `json:"end_byte"`
	StartLine   int `json:"start_line"`
	StartColumn int `json:"start_column"`
	EndLine     int `json:"end_line"`
	EndColumn   int `json:"end_column"`
}

type ExpectedFinding struct {
	Path string `json:"path"`
	PositionRange
	PatternID string             `json:"pattern_id"`
	Message   string             `json:"message"`
	Bindings  map[string]Binding `json:"bindings"`
}

type Binding struct {
	Kind       string          `json:"kind"`
	Range      *PositionRange  `json:"range,omitempty"`
	Ranges     []PositionRange `json:"ranges,omitempty"`
	Structural json.RawMessage `json:"structural"`
}

type ExpectedDiagnostic struct {
	Code      string         `json:"code"`
	Class     string         `json:"class"`
	Severity  string         `json:"severity"`
	Message   string         `json:"message"`
	PatternID *string        `json:"pattern_id"`
	Path      *string        `json:"path"`
	Range     *PositionRange `json:"range"`
}

// LoadConformance loads all JSON files in the six immediate suite directories.
// It rejects unknown JSON fields, trailing JSON, duplicate names, invalid ranges,
// unsorted output, and incomplete feature coverage.
func LoadConformance(root string) (*Corpus, error) {
	loader := conformanceLoader{
		corpus:          &Corpus{},
		names:           make(map[string]string),
		features:        make(map[string]bool),
		diagnosticCodes: make(map[string]bool),
	}
	for _, suite := range ConformanceSuites {
		if err := loader.loadSuite(root, suite); err != nil {
			return nil, err
		}
	}
	if err := loader.validateRequiredCoverage(); err != nil {
		return nil, err
	}
	return loader.corpus, nil
}

type conformanceLoader struct {
	corpus          *Corpus
	names           map[string]string
	features        map[string]bool
	diagnosticCodes map[string]bool
}

func (l *conformanceLoader) loadSuite(root, suite string) error {
	files, err := suiteFixturePaths(root, suite)
	if err != nil {
		return err
	}
	for _, path := range files {
		if err := l.loadFixture(path, suite); err != nil {
			return err
		}
	}
	return nil
}

func suiteFixturePaths(root, suite string) ([]string, error) {
	dir := filepath.Join(root, suite)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read suite %s: %w", suite, err)
	}
	files := make([]string, 0)
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			files = append(files, filepath.Join(dir, entry.Name()))
		}
	}
	sort.Strings(files)
	if len(files) == 0 {
		return nil, fmt.Errorf("suite %s has no JSON fixtures", suite)
	}
	return files, nil
}

func (l *conformanceLoader) loadFixture(path, suite string) error {
	fixture, err := decodeFixture(path)
	if err != nil {
		return err
	}
	if fixture.SchemaVersion != 1 || fixture.Suite != suite {
		return fmt.Errorf("%s: schema_version must be 1 and suite must be %q", path, suite)
	}
	if fixture.Provenance.Kind == "" || fixture.Provenance.Note == "" {
		return fmt.Errorf("%s: provenance kind and note are required", path)
	}
	if len(fixture.Cases) == 0 {
		return fmt.Errorf("%s: no cases", path)
	}
	for i := range fixture.Cases {
		if err := l.addCase(path, suite, fixture.Provenance, &fixture.Cases[i]); err != nil {
			return err
		}
	}
	l.corpus.Files = append(l.corpus.Files, fixture)
	return nil
}

func (l *conformanceLoader) addCase(path, suite string, provenance Provenance, c *Case) error {
	c.Suite, c.Provenance = suite, provenance
	if old, ok := l.names[c.Name]; ok {
		return fmt.Errorf("%s: duplicate case name %q (first in %s)", path, c.Name, old)
	}
	l.names[c.Name] = path
	for _, feature := range c.Features {
		l.features[feature] = true
	}
	for _, diagnostic := range c.Expected.Diagnostics {
		l.diagnosticCodes[diagnostic.Code] = true
	}
	if err := validateCase(*c); err != nil {
		return fmt.Errorf("%s case %q: %w", path, c.Name, err)
	}
	l.corpus.Cases = append(l.corpus.Cases, *c)
	return nil
}

func (l *conformanceLoader) validateRequiredCoverage() error {
	for _, required := range requiredFeatures {
		if !l.features[required] {
			return fmt.Errorf("corpus lacks required feature %q", required)
		}
	}
	for _, required := range requiredDiagnosticCodes {
		if !l.diagnosticCodes[required] {
			return fmt.Errorf("corpus lacks stable diagnostic code %q", required)
		}
	}
	return nil
}

func decodeFixture(path string) (FixtureFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return FixtureFile{}, fmt.Errorf("read %s: %w", path, err)
	}
	var fixture FixtureFile
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&fixture); err != nil {
		return FixtureFile{}, fmt.Errorf("decode %s: %w", path, err)
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return FixtureFile{}, fmt.Errorf("decode %s: trailing JSON", path)
	}
	return fixture, nil
}

func validateCase(c Case) error {
	if err := validateCaseConfiguration(c); err != nil {
		return err
	}
	patternIDs, patterns, err := validateCasePatterns(c)
	if err != nil {
		return err
	}
	if err := validateExpectedContract(c); err != nil {
		return err
	}
	sources, err := decodeCaseSources(c.Sources)
	if err != nil {
		return err
	}
	if err := validateRejectedPaths(c.RejectedPaths); err != nil {
		return err
	}
	if err := validateParseCounts(c, sources); err != nil {
		return err
	}
	if err := validateExpectedFindings(c.Expected.Findings, sources, patternIDs); err != nil {
		return err
	}
	if err := validateFindingOrder(c.Expected.Findings); err != nil {
		return err
	}
	if err := validateExpectedDiagnostics(c.Expected.Diagnostics, sources, patternIDs, patterns); err != nil {
		return err
	}
	if err := validateRejectedPathDiagnostics(c.RejectedPaths, c.Expected.Diagnostics); err != nil {
		return err
	}
	return validateDiagnosticOrder(c.Expected.Diagnostics)
}

func validateCaseConfiguration(c Case) error {
	if c.Name == "" || c.Description == "" || c.PatternID == "" || c.Pattern == "" {
		return fmt.Errorf("name, description, pattern_id, and pattern are required")
	}
	if len(c.Features) == 0 {
		return fmt.Errorf("features must not be empty")
	}
	if c.Execution.Mode != "local" && c.Execution.Mode != "distributed" && c.Execution.Mode != "both" {
		return fmt.Errorf("execution.mode must be local, distributed, or both")
	}
	if c.Execution.Cancel != "" && c.Execution.Cancel != "before-file" && c.Execution.Cancel != "during-file" && c.Execution.Cancel != "before-commit" {
		return fmt.Errorf("execution.cancel has an unknown checkpoint")
	}
	if c.Execution.Fault != "" && c.Execution.Fault != "internal-invariant" {
		return fmt.Errorf("execution.fault has an unknown injection")
	}
	return nil
}

func validateCasePatterns(c Case) (map[string]bool, map[string][]byte, error) {
	patternIDs := map[string]bool{c.PatternID: true}
	patterns := map[string][]byte{c.PatternID: []byte(c.Pattern)}
	for i, pattern := range c.AdditionalPatterns {
		if pattern.PatternID == "" || pattern.Pattern == "" || patternIDs[pattern.PatternID] {
			return nil, nil, fmt.Errorf("additional pattern %d is incomplete or has a duplicate pattern_id", i)
		}
		patternIDs[pattern.PatternID] = true
		patterns[pattern.PatternID] = []byte(pattern.Pattern)
	}
	return patternIDs, patterns, nil
}

func validateExpectedContract(c Case) error {
	if c.Expected.Findings == nil || c.Expected.Diagnostics == nil {
		return fmt.Errorf("expected findings and diagnostics must be arrays, not null")
	}
	if err := validateLimits(c.Execution.Limits, false); err != nil {
		return fmt.Errorf("execution limits: %w", err)
	}
	if (c.Expected.Text != nil) != c.Execution.TextCompatibility {
		return fmt.Errorf("expected text must be present exactly when text_compatibility is enabled")
	}
	if c.Expected.Metadata == nil {
		return nil
	}
	if c.Expected.Metadata.Contract != "gritql-go-v1" || c.Expected.Metadata.GoGrammar == "" {
		return fmt.Errorf("metadata must identify gritql-go-v1 and its Go grammar")
	}
	if err := validateLimits(c.Expected.Metadata.Limits, true); err != nil {
		return fmt.Errorf("metadata limits: %w", err)
	}
	return nil
}

func decodeCaseSources(input []Source) (map[string][]byte, error) {
	sources := make(map[string][]byte)
	for _, source := range input {
		normalized, ok := normalizeFixturePath(source.Path)
		if !ok || normalized != source.Path {
			return nil, fmt.Errorf("source path %q is not normalized", source.Path)
		}
		if _, exists := sources[source.Path]; exists {
			return nil, fmt.Errorf("duplicate source path %q", source.Path)
		}
		if (source.Text == nil) == (source.BytesBase64 == nil) {
			return nil, fmt.Errorf("source %q must set exactly one of text or bytes_base64", source.Path)
		}
		data, err := decodeCaseSource(source)
		if err != nil {
			return nil, err
		}
		sources[source.Path] = data
	}
	return sources, nil
}

func decodeCaseSource(source Source) ([]byte, error) {
	if source.Text != nil {
		return []byte(*source.Text), nil
	}
	data, err := base64.StdEncoding.Strict().DecodeString(*source.BytesBase64)
	if err != nil {
		return nil, fmt.Errorf("source %q: invalid base64: %w", source.Path, err)
	}
	return data, nil
}

func validateRejectedPaths(paths []string) error {
	seen := make(map[string]bool)
	for i, path := range paths {
		if _, safe := normalizeFixturePath(path); safe {
			return fmt.Errorf("rejected path %d %q is safe and must be supplied as a source", i, path)
		}
		if seen[path] {
			return fmt.Errorf("duplicate rejected path %q", path)
		}
		seen[path] = true
	}
	return nil
}

func validateParseCounts(c Case, sources map[string][]byte) error {
	for path, count := range c.Expected.ParseCounts {
		normalized, normalizedOK := normalizeFixturePath(path)
		_, sourceOK := sources[path]
		if !normalizedOK || normalized != path || !sourceOK || count < 0 {
			return fmt.Errorf("parse count for %q is not normalized, has no source, or is negative", path)
		}
		if c.Execution.RequireParseOnce && count != 1 {
			return fmt.Errorf("parse-once case has count %d for %q", count, path)
		}
	}
	if c.Execution.RequireParseOnce && len(c.Expected.ParseCounts) == 0 {
		return fmt.Errorf("parse-once case must declare expected parse counts")
	}
	return nil
}

func validateExpectedFindings(findings []ExpectedFinding, sources map[string][]byte, patternIDs map[string]bool) error {
	for i, finding := range findings {
		if err := validateExpectedFinding(i, finding, sources, patternIDs); err != nil {
			return err
		}
	}
	return nil
}

func validateExpectedFinding(index int, finding ExpectedFinding, sources map[string][]byte, patternIDs map[string]bool) error {
	normalized, normalizedOK := normalizeFixturePath(finding.Path)
	data, sourceOK := sources[finding.Path]
	if !normalizedOK || normalized != finding.Path || !sourceOK {
		return fmt.Errorf("finding %d refers to unknown or non-normalized source %q", index, finding.Path)
	}
	if err := validateRange(finding.PositionRange, data); err != nil {
		return fmt.Errorf("finding %d: %w", index, err)
	}
	if !patternIDs[finding.PatternID] || finding.Bindings == nil {
		return fmt.Errorf("finding %d has unknown pattern_id or null bindings", index)
	}
	for name, binding := range finding.Bindings {
		if err := validateExpectedBinding(index, name, binding, finding.PositionRange, data); err != nil {
			return err
		}
	}
	return nil
}

func validateExpectedBinding(findingIndex int, name string, binding Binding, findingRange PositionRange, data []byte) error {
	if !validBindingName(name) || binding.Kind != "node" && binding.Kind != "list" || len(binding.Structural) == 0 || !json.Valid(binding.Structural) {
		return fmt.Errorf("finding %d has invalid binding %q", findingIndex, name)
	}
	if binding.Range == nil {
		return fmt.Errorf("binding %q must have an overall range", name)
	}
	if binding.Kind == "node" && binding.Ranges != nil {
		return fmt.Errorf("node binding %q must have no element ranges", name)
	}
	if binding.Kind == "list" && binding.Ranges == nil {
		return fmt.Errorf("list binding %q must have an element ranges array", name)
	}
	if err := validateBindingRanges(name, binding, findingRange, data); err != nil {
		return err
	}
	if err := validateStructuralBinding(binding); err != nil {
		return fmt.Errorf("binding %q: %w", name, err)
	}
	return nil
}

func validateBindingRanges(name string, binding Binding, findingRange PositionRange, data []byte) error {
	if err := validateRange(*binding.Range, data); err != nil {
		return fmt.Errorf("binding %q: %w", name, err)
	}
	if !rangeContains(findingRange, *binding.Range) {
		return fmt.Errorf("binding %q range lies outside finding", name)
	}
	previousEnd := binding.Range.StartByte
	for _, elementRange := range binding.Ranges {
		if err := validateRange(elementRange, data); err != nil {
			return fmt.Errorf("binding %q: %w", name, err)
		}
		if elementRange.StartByte < previousEnd || !rangeContains(*binding.Range, elementRange) {
			return fmt.Errorf("binding %q element ranges overlap, are unordered, or lie outside its span", name)
		}
		previousEnd = elementRange.EndByte
	}
	if binding.Kind != "list" {
		return nil
	}
	if len(binding.Ranges) == 0 && binding.Range.StartByte != binding.Range.EndByte {
		return fmt.Errorf("empty list binding %q must have a zero-width slot range", name)
	}
	if len(binding.Ranges) > 0 && (binding.Range.StartByte != binding.Ranges[0].StartByte || binding.Range.EndByte != binding.Ranges[len(binding.Ranges)-1].EndByte) {
		return fmt.Errorf("list binding %q span must cover exactly its first through last elements", name)
	}
	return nil
}

func validateFindingOrder(findings []ExpectedFinding) error {
	if !sort.SliceIsSorted(findings, func(i, j int) bool {
		return compareFindings(findings[i], findings[j]) < 0
	}) && len(findings) > 1 {
		return fmt.Errorf("expected findings are not in contractual order")
	}
	for i := 1; i < len(findings); i++ {
		if compareFindings(findings[i-1], findings[i]) == 0 {
			return fmt.Errorf("expected findings contain a duplicate")
		}
	}
	return nil
}

func validateExpectedDiagnostics(diagnostics []ExpectedDiagnostic, sources map[string][]byte, patternIDs map[string]bool, patterns map[string][]byte) error {
	for i, diagnostic := range diagnostics {
		if err := validateExpectedDiagnostic(i, diagnostic, sources, patternIDs, patterns); err != nil {
			return err
		}
	}
	return nil
}

func validateExpectedDiagnostic(index int, diagnostic ExpectedDiagnostic, sources map[string][]byte, patternIDs map[string]bool, patterns map[string][]byte) error {
	class, known := diagnosticClasses[diagnostic.Code]
	if !known || diagnostic.Class != class || diagnostic.Severity != "error" || diagnostic.Message == "" {
		return fmt.Errorf("diagnostic %d has unknown code or invalid required fields", index)
	}
	if diagnostic.PatternID != nil && !patternIDs[*diagnostic.PatternID] {
		return fmt.Errorf("diagnostic %d refers to unknown pattern_id", index)
	}
	if isPatternDiagnostic(diagnostic.Code) {
		return validatePatternDiagnostic(index, diagnostic, patterns)
	}
	return validateSourceDiagnostic(index, diagnostic, sources)
}

func validatePatternDiagnostic(index int, diagnostic ExpectedDiagnostic, patterns map[string][]byte) error {
	if diagnostic.PatternID == nil || diagnostic.Path != nil || diagnostic.Range == nil {
		return fmt.Errorf("diagnostic %d pattern diagnostics require pattern_id and range and must have null path", index)
	}
	patternSource := patterns[*diagnostic.PatternID]
	if err := validateRange(*diagnostic.Range, patternSource); err != nil {
		return fmt.Errorf("diagnostic %d pattern range: %w", index, err)
	}
	if diagnostic.Range.StartByte != 0 || diagnostic.Range.EndByte != len(patternSource) {
		return fmt.Errorf("diagnostic %d pattern range must cover the whole submitted pattern", index)
	}
	return nil
}

func validateSourceDiagnostic(index int, diagnostic ExpectedDiagnostic, sources map[string][]byte) error {
	if diagnostic.Path == nil {
		return nil
	}
	normalized, normalizedOK := normalizeFixturePath(*diagnostic.Path)
	data, sourceOK := sources[*diagnostic.Path]
	if !normalizedOK || normalized != *diagnostic.Path || !sourceOK {
		return fmt.Errorf("diagnostic %d refers to unknown or non-normalized source", index)
	}
	if diagnostic.Range == nil || !utf8.Valid(data) {
		return nil
	}
	if err := validateRange(*diagnostic.Range, data); err != nil {
		return fmt.Errorf("diagnostic %d: %w", index, err)
	}
	return nil
}

func validateRejectedPathDiagnostics(rejectedPaths []string, diagnostics []ExpectedDiagnostic) error {
	if len(rejectedPaths) == 0 {
		return nil
	}
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == "PATH_INVALID" {
			return nil
		}
	}
	return fmt.Errorf("rejected_paths input requires a PATH_INVALID diagnostic")
}

func validateDiagnosticOrder(diagnostics []ExpectedDiagnostic) error {
	if !sort.SliceIsSorted(diagnostics, func(i, j int) bool {
		return compareDiagnostics(diagnostics[i], diagnostics[j]) < 0
	}) && len(diagnostics) > 1 {
		return fmt.Errorf("expected diagnostics are not in contractual order")
	}
	return nil
}

func isPatternDiagnostic(code string) bool {
	return strings.HasPrefix(code, "PATTERN_") || code == "LIMIT_PATTERN_BYTES"
}

func validBindingName(name string) bool {
	if name == "" || !isASCIILetter(name[0]) {
		return false
	}
	for i := 1; i < len(name); i++ {
		if !isASCIILetter(name[i]) && (name[i] < '0' || name[i] > '9') && name[i] != '_' {
			return false
		}
	}
	return true
}

func isASCIILetter(b byte) bool {
	return b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z'
}

func rangeContains(outer, inner PositionRange) bool {
	return outer.StartByte <= inner.StartByte && inner.EndByte <= outer.EndByte
}

func validateStructuralBinding(binding Binding) error {
	var value any
	if err := json.Unmarshal(binding.Structural, &value); err != nil {
		return fmt.Errorf("invalid structural JSON: %w", err)
	}
	if binding.Kind == "node" {
		if _, ok := value.(map[string]any); !ok {
			return fmt.Errorf("node structural value must be one normalized node")
		}
		return validateStructuralValue(value)
	}
	elements, ok := value.([]any)
	if !ok {
		return fmt.Errorf("list structural value must be an array")
	}
	if len(elements) != len(binding.Ranges) {
		return fmt.Errorf("list structural element and range counts differ")
	}
	for _, element := range elements {
		if err := validateStructuralValue(element); err != nil {
			return err
		}
	}
	return nil
}

func validateStructuralValue(value any) error {
	object, ok := value.(map[string]any)
	if !ok || len(object) != 2 {
		return fmt.Errorf("structural value must be a normalized two-field node or token")
	}
	if kind, node := object["node_kind"].(string); node && kind != "" {
		children, childrenOK := object["children"].([]any)
		if !childrenOK {
			return fmt.Errorf("normalized node must have an ordered children array")
		}
		for _, child := range children {
			if err := validateStructuralValue(child); err != nil {
				return err
			}
		}
		return nil
	}
	kind, token := object["token_kind"].(string)
	_, lexeme := object["lexeme"].(string)
	if !token || kind == "" || !lexeme {
		return fmt.Errorf("normalized token must have token_kind and lexeme strings")
	}
	return nil
}
func validateLimits(l Limits, requireAll bool) error {
	values := []struct {
		name  string
		value *int64
		max   int64
	}{
		{"pattern_bytes", l.PatternBytes, 1 << 20},
		{"source_bytes", l.SourceBytes, 50 << 20},
		{"parse_depth", l.ParseDepth, 1024},
		{"candidates", l.Candidates, 1_000_000},
		{"ast_steps", l.ASTSteps, 100_000_000},
		{"findings", l.Findings, 100_000},
		{"file_time_ms", l.FileTimeMillis, 10_000},
		{"batch_time_ms", l.BatchTimeMillis, 300_000},
		{"memory_bytes", l.MemoryBytes, 512 << 20},
		{"regex_bytes", l.RegexBytes, 64 << 10},
		{"regex_instructions", l.RegexInstructions, 500_000},
	}
	for _, item := range values {
		if item.value == nil {
			if requireAll {
				return fmt.Errorf("%s is required", item.name)
			}
			continue
		}
		if *item.value < 0 || *item.value > item.max {
			return fmt.Errorf("%s must be between zero and hard maximum %d", item.name, item.max)
		}
	}
	return nil
}

func normalizeFixturePath(input string) (string, bool) {
	input = strings.ReplaceAll(input, `\`, "/")
	if input == "" || strings.ContainsRune(input, 0) || strings.HasPrefix(input, "/") || runtime.GOOS == "windows" && len(input) >= 2 && input[1] == ':' {
		return "", false
	}
	parts := make([]string, 0)
	for _, part := range strings.Split(input, "/") {
		switch part {
		case "", ".":
			continue
		case "..":
			if len(parts) == 0 {
				return "", false
			}
			parts = parts[:len(parts)-1]
		default:
			parts = append(parts, part)
		}
	}
	if len(parts) == 0 {
		return "", false
	}
	return strings.Join(parts, "/"), true
}

func validateRange(r PositionRange, data []byte) error {
	if !utf8.Valid(data) {
		return fmt.Errorf("cannot validate a range against invalid UTF-8")
	}
	if r.StartByte < 0 || r.EndByte < r.StartByte || r.EndByte > len(data) {
		return fmt.Errorf("invalid byte range [%d,%d) for %d bytes", r.StartByte, r.EndByte, len(data))
	}
	sl, sc, err := lineColumn(data, r.StartByte)
	if err != nil {
		return err
	}
	el, ec, err := lineColumn(data, r.EndByte)
	if err != nil {
		return err
	}
	if sl != r.StartLine || sc != r.StartColumn || el != r.EndLine || ec != r.EndColumn {
		return fmt.Errorf("range coordinates do not match bytes: got %d:%d-%d:%d, want %d:%d-%d:%d", r.StartLine, r.StartColumn, r.EndLine, r.EndColumn, sl, sc, el, ec)
	}
	return nil
}

func lineColumn(data []byte, offset int) (int, int, error) {
	if offset < 0 || offset > len(data) || !utf8.Valid(data) {
		return 0, 0, fmt.Errorf("invalid UTF-8 or offset")
	}
	line, column := 1, 1
	for i := 0; i < offset; {
		if data[i] == '\r' && i+1 < len(data) && data[i+1] == '\n' {
			if i+1 == offset {
				return 0, 0, fmt.Errorf("offset splits CRLF")
			}
			i += 2
			line++
			column = 1
			continue
		}
		r, size := utf8.DecodeRune(data[i:])
		if i+size > offset {
			return 0, 0, fmt.Errorf("offset splits UTF-8 scalar")
		}
		i += size
		if r == '\n' {
			line++
			column = 1
		} else {
			column++
		}
	}
	return line, column, nil
}

func compareFindings(a, b ExpectedFinding) int {
	if n := strings.Compare(a.Path, b.Path); n != 0 {
		return n
	}
	if n := compareInt(a.StartByte, b.StartByte); n != 0 {
		return n
	}
	if n := compareInt(a.EndByte, b.EndByte); n != 0 {
		return n
	}
	if n := strings.Compare(a.PatternID, b.PatternID); n != 0 {
		return n
	}
	if n := strings.Compare(a.Message, b.Message); n != 0 {
		return n
	}
	aBindings, _ := json.Marshal(a.Bindings)
	bBindings, _ := json.Marshal(b.Bindings)
	if n := bytes.Compare(aBindings, bBindings); n != 0 {
		return n
	}
	aComplete, _ := json.Marshal(a)
	bComplete, _ := json.Marshal(b)
	return bytes.Compare(aComplete, bComplete)
}

func compareDiagnostics(a, b ExpectedDiagnostic) int {
	if n := compareOptionalString(a.Path, b.Path); n != 0 {
		return n
	}
	if n := compareOptionalRangeStart(a.Range, b.Range); n != 0 {
		return n
	}
	if n := strings.Compare(a.Code, b.Code); n != 0 {
		return n
	}
	if n := compareOptionalString(a.PatternID, b.PatternID); n != 0 {
		return n
	}
	return strings.Compare(a.Message, b.Message)
}

func compareInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

func compareOptionalString(a, b *string) int {
	if a == nil {
		if b == nil {
			return 0
		}
		return -1
	}
	if b == nil {
		return 1
	}
	return strings.Compare(*a, *b)
}

func compareOptionalRangeStart(a, b *PositionRange) int {
	if a == nil {
		if b == nil {
			return 0
		}
		return -1
	}
	if b == nil {
		return 1
	}
	return compareInt(a.StartByte, b.StartByte)
}

var requiredFeatures = []string{
	"positive", "negative", "malformed", "comments", "utf8-ranges", "repeated-bindings",
	"branch-rollback", "branch-isolation", "nested-contains-within", "ordering", "dedup",
	"local-distributed-parity", "text-compatibility", "parse-once-batching", "cancellation",
	"limit-pattern-bytes", "limit-source-bytes", "limit-parse-depth", "limit-candidates", "limit-ast-steps",
	"limit-findings", "limit-file-time", "limit-batch-time", "limit-memory", "limit-regex-bytes",
	"limit-regex-instructions", "unsupported-rewrite", "unsupported-definitions", "unsupported-imports",
	"unsupported-multifile", "unsupported-constructors", "unsupported-language-qualifier",
	"unsupported-as", "unsupported-functions", "unsupported-equality", "unsupported-token",
	"unsupported-behavior",
}

var diagnosticClasses = map[string]string{
	"PATTERN_PARSE":             "pattern",
	"PATTERN_UNSUPPORTED":       "unsupported",
	"PATTERN_INVALID_CONTEXT":   "pattern",
	"PATTERN_INVALID_SNIPPET":   "pattern",
	"PATTERN_AMBIGUOUS_SNIPPET": "pattern",
	"PATTERN_INVALID_REGEX":     "pattern",
	"SOURCE_INVALID_UTF8":       "source",
	"SOURCE_PARSE":              "source",
	"PATH_INVALID":              "source",
	"LIMIT_PATTERN_BYTES":       "resource",
	"LIMIT_SOURCE_BYTES":        "resource",
	"LIMIT_PARSE_DEPTH":         "resource",
	"LIMIT_CANDIDATES":          "resource",
	"LIMIT_AST_STEPS":           "resource",
	"LIMIT_FINDINGS":            "resource",
	"LIMIT_TIME_FILE":           "resource",
	"LIMIT_TIME_BATCH":          "resource",
	"LIMIT_MEMORY":              "resource",
	"EVALUATION_CANCELLED":      "cancelled",
	"INTERNAL_ERROR":            "internal",
}

var requiredDiagnosticCodes = []string{
	"PATTERN_PARSE", "PATTERN_UNSUPPORTED", "PATTERN_INVALID_CONTEXT",
	"PATTERN_INVALID_REGEX", "SOURCE_INVALID_UTF8", "SOURCE_PARSE", "PATH_INVALID",
	"LIMIT_PATTERN_BYTES", "LIMIT_SOURCE_BYTES", "LIMIT_PARSE_DEPTH", "LIMIT_CANDIDATES", "LIMIT_AST_STEPS",
	"LIMIT_FINDINGS", "LIMIT_TIME_FILE", "LIMIT_TIME_BATCH", "LIMIT_MEMORY",
	"EVALUATION_CANCELLED", "INTERNAL_ERROR",
}
