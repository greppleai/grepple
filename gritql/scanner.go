package gritql

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/greppleai/grepple/parser"
)

const (
	defaultScanWorkers     = 4
	hardScanWorkers        = 64
	defaultScanFiles       = 100_000
	hardScanFiles          = 1_000_000
	defaultScanTotalBytes  = int64(1 << 30)
	hardScanTotalBytes     = int64(16 << 30)
	sourceMemoryFactor     = int64(4)
	minimumScanReservation = int64(64 << 10)
)

// ScanCandidate identifies one already-scoped filesystem entry and its public
// repository-relative display path. ReadPath is interpreted by the supplied FS.
// A non-nil Content slice supplies bytes acquired by an upstream prefilter and
// prevents a second filesystem read; callers must not mutate it during the scan.
type ScanCandidate struct {
	ReadPath string
	Path     string
	Language string
	Content  []byte
}

// ScanOptions controls candidate filtering, bounded acquisition, and evaluation.
// Zero resource values select deterministic defaults.
type ScanOptions struct {
	EvaluateOptions EvaluateOptions
	PatternID       string
	Message         string
	IncludeGlobs    []string
	ExcludeGlobs    []string
	Workers         int
	MaxFiles        int
	MaxTotalBytes   int64
}

// ScanTruncation records deterministic scanner-level omission separately from
// evaluation diagnostics.
type ScanTruncation struct {
	Reason  string `json:"reason"`
	Limit   int64  `json:"limit"`
	Skipped int    `json:"skipped"`
}

// ScanStats reports deterministic candidate and acquisition counts.
type ScanStats struct {
	Candidates      int   `json:"candidates"`
	Eligible        int   `json:"eligible"`
	Evaluated       int   `json:"evaluated"`
	BytesRead       int64 `json:"bytes_read"`
	SkippedLanguage int   `json:"skipped_language"`
	SkippedGlob     int   `json:"skipped_glob"`
	SkippedBinary   int   `json:"skipped_binary"`
}

// ScanResult is the immutable normalized result of one bounded candidate scan.
type ScanResult struct {
	evaluation  FileEvaluation
	truncations []ScanTruncation
	stats       ScanStats
}

// Findings returns globally sorted, deduplicated structural findings.
func (r ScanResult) Findings() []Finding { return r.evaluation.Findings() }

// Diagnostics returns globally sorted, deduplicated evaluation diagnostics.
func (r ScanResult) Diagnostics() []Diagnostic { return r.evaluation.Diagnostics() }

// Metadata returns the compatibility and effective evaluation limits.
func (r ScanResult) Metadata() EvaluationMetadata { return r.evaluation.Metadata() }

// Truncations returns a copy of scanner-level truncation records.
func (r ScanResult) Truncations() []ScanTruncation {
	return append([]ScanTruncation(nil), r.truncations...)
}

// Stats returns deterministic scanner counters.
func (r ScanResult) Stats() ScanStats { return r.stats }

// MarshalJSON returns findings, diagnostics, metadata, truncations, and scanner statistics.
func (r ScanResult) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Findings    []Finding          `json:"findings"`
		Diagnostics []Diagnostic       `json:"diagnostics"`
		Metadata    EvaluationMetadata `json:"metadata"`
		Truncations []ScanTruncation   `json:"truncations"`
		Stats       ScanStats          `json:"stats"`
	}{r.Findings(), r.Diagnostics(), r.Metadata(), nonNilTruncations(r.truncations), r.stats})
}

type preparedScanCandidate struct {
	readPath string
	path     string
	language string
	content  []byte
}

type scanOutcome struct {
	index          int
	evaluation     FileEvaluation
	bytesRead      int64
	reserved       int64
	binary         bool
	totalTruncated bool
}

type scanJob struct {
	index     int
	candidate preparedScanCandidate
}
type scanState struct {
	options        ScanOptions
	started        time.Time
	evaluations    []FileEvaluation
	truncations    []ScanTruncation
	retained       int64
	stats          ScanStats
	account        *scanMemoryAccount
	bytes          *scanByteAccount
	candidateCount int
	stop           bool
	totalTruncated bool
	totalSkipped   int
}

// ScanFiles evaluates already-scoped candidates through a bounded worker pool.
// Candidate completion order never affects committed ordering or normalized output.
func ScanFiles(ctx context.Context, filesystem fs.FS, program *Program, candidates []ScanCandidate, options ScanOptions) ScanResult {
	if ctx == nil {
		ctx = context.Background()
	}
	options = normalizeScanOptions(options)
	state := scanState{
		options: options, started: time.Now(),
		account: newScanMemoryAccount(int64(options.EvaluateOptions.MaxMemoryBytes)),
		bytes:   newScanByteAccount(options.MaxTotalBytes),
	}
	if program == nil || program.root == nil {
		state.evaluations = append(state.evaluations, scannerDiagnosticEvaluation(program, options, evaluationFailure("INTERNAL_ERROR", "internal", "invalid compiled program", nil), nil))
		return finishScan(ctx, program, state, nil)
	}
	prepared, early, truncations := prepareScanCandidates(program.Language(), candidates, options, &state.stats)
	state.evaluations = append(state.evaluations, early...)
	if scanFilesystemUnavailable(filesystem, prepared) {
		state.evaluations = append(state.evaluations, scannerFailure(options, "source filesystem is unavailable", nil))
		return finishScan(ctx, program, state, truncations)
	}
	state.scan(ctx, filesystem, program, prepared)
	return finishScan(ctx, program, state, truncations)
}
func scanFilesystemUnavailable(filesystem fs.FS, candidates []preparedScanCandidate) bool {
	return filesystem == nil && (len(candidates) == 0 || scanCandidatesRequireFS(candidates))
}

func scanCandidatesRequireFS(candidates []preparedScanCandidate) bool {
	for _, candidate := range candidates {
		if candidate.content == nil {
			return true
		}
	}
	return false
}

func (s *scanState) scan(ctx context.Context, filesystem fs.FS, program *Program, candidates []preparedScanCandidate) {
	if len(candidates) == 0 {
		return
	}
	s.candidateCount = len(candidates)
	jobs := make(chan scanJob)
	outcomes := make(chan scanOutcome, s.options.Workers)
	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	for range s.options.Workers {
		go scanWorker(workerCtx, filesystem, program, s.options, s.account, s.bytes, jobs, outcomes)
	}
	go feedScanJobs(workerCtx, candidates, jobs)
	consumeScanOutcomes(ctx, s, program, outcomes)
}

func feedScanJobs(ctx context.Context, candidates []preparedScanCandidate, jobs chan<- scanJob) {
	defer close(jobs)
	for index, candidate := range candidates {
		select {
		case jobs <- scanJob{index, candidate}:
		case <-ctx.Done():
			return
		}
	}
}

func consumeScanOutcomes(ctx context.Context, s *scanState, program *Program, outcomes <-chan scanOutcome) {
	pending := make(map[int]scanOutcome, s.options.Workers)
	next := 0
	for next < s.candidateCount && !s.stop {
		if commitScanFailure(ctx, s, program) {
			break
		}
		select {
		case outcome := <-outcomes:
			pending[outcome.index] = outcome
			next = commitPendingScanOutcomes(s, program, pending, next)
		case <-ctx.Done():
			s.evaluations = append(s.evaluations, scannerDiagnosticEvaluation(program, s.options, evaluationFailure("EVALUATION_CANCELLED", "cancelled", "evaluation cancelled", ctx.Err()), nil))
			s.stop = true
		case <-time.After(scanWaitDuration(s.options.EvaluateOptions, s.started)):
			s.evaluations = append(s.evaluations, scannerDiagnosticEvaluation(program, s.options, evaluationFailure("LIMIT_TIME_BATCH", "resource", "batch evaluation deadline exceeded", nil), nil))
			s.stop = true
		}
	}
}

func commitScanFailure(ctx context.Context, s *scanState, program *Program) bool {
	failure := scanBatchFailure(ctx, s.options.EvaluateOptions, s.started)
	if failure == nil {
		return false
	}
	s.evaluations = append(s.evaluations, scannerDiagnosticEvaluation(program, s.options, failure, nil))
	s.stop = true
	return true
}

func commitPendingScanOutcomes(s *scanState, program *Program, pending map[int]scanOutcome, next int) int {
	for {
		current, ok := pending[next]
		if !ok {
			return next
		}
		delete(pending, next)
		s.commit(program, current)
		next++
		if s.stop {
			return next
		}
	}
}

func (s *scanState) commit(program *Program, outcome scanOutcome) {
	if outcome.totalTruncated {
		s.account.discard(outcome.index, outcome.reserved)
		s.stop = true
		s.totalTruncated = true
		s.totalSkipped = s.candidateCount - outcome.index
		return
	}
	s.stats.BytesRead += outcome.bytesRead
	if outcome.binary {
		s.account.discard(outcome.index, outcome.reserved)
		s.stats.SkippedBinary++
		return
	}
	if hasEvaluationDiagnostic(outcome.evaluation, "LIMIT_MEMORY") {
		s.account.discard(outcome.index, outcome.reserved)
		s.evaluations = append(s.evaluations, outcome.evaluation)
		s.stop = true
		return
	}
	estimate := estimateEvaluationMemory(outcome.evaluation)
	if s.retained+estimate > int64(s.options.EvaluateOptions.MaxMemoryBytes) {
		s.account.discard(outcome.index, outcome.reserved)
		pathValue := diagnosticPath(outcome.evaluation)
		s.evaluations = append(s.evaluations, scannerDiagnosticEvaluation(program, s.options, evaluationFailure("LIMIT_MEMORY", "resource", "accounted memory limit exceeded", nil), pathValue))
		s.stop = true
		return
	}
	s.retained += estimate
	s.account.retain(outcome.index, outcome.reserved)
	s.stats.Evaluated++
	s.evaluations = append(s.evaluations, outcome.evaluation)
}

func scanWorker(ctx context.Context, filesystem fs.FS, program *Program, options ScanOptions, account *scanMemoryAccount, byteAccount *scanByteAccount, jobs <-chan scanJob, outcomes chan<- scanOutcome) {
	for job := range jobs {
		reservation := scanSourceReservation(options)
		if !account.acquire(ctx, reservation) {
			if !account.replace(ctx, job.index, 0, 0) {
				return
			}
			outcome := scanMemoryFailure(program, options, job.index, job.candidate.path)
			if !sendScanOutcome(ctx, outcomes, outcome) {
				return
			}
			continue
		}
		outcome := evaluateScanCandidate(ctx, filesystem, program, options, byteAccount, job.index, job.candidate)
		resultReservation := estimateEvaluationMemory(outcome.evaluation)
		if !account.replace(ctx, job.index, reservation, resultReservation) {
			outcome = scanMemoryFailure(program, options, job.index, job.candidate.path)
			resultReservation = 0
		}
		outcome.reserved = resultReservation
		if !sendScanOutcome(ctx, outcomes, outcome) {
			account.release(resultReservation)
			return
		}
	}
}

func sendScanOutcome(ctx context.Context, outcomes chan<- scanOutcome, outcome scanOutcome) bool {
	select {
	case outcomes <- outcome:
		return true
	case <-ctx.Done():
		return false
	}
}

func scanMemoryFailure(program *Program, options ScanOptions, index int, path string) scanOutcome {
	return scanOutcome{index: index, evaluation: scannerDiagnosticEvaluation(program, options, evaluationFailure("LIMIT_MEMORY", "resource", "accounted memory limit exceeded", nil), &path)}
}

func evaluateScanCandidate(ctx context.Context, filesystem fs.FS, program *Program, options ScanOptions, byteAccount *scanByteAccount, index int, candidate preparedScanCandidate) scanOutcome {
	outcome := scanOutcome{index: index}
	started := time.Now()
	read := byteAccount.read(ctx, filesystem, candidate.readPath, candidate.content, index, options)
	outcome.bytesRead = int64(len(read.content))
	if read.limit == scanLimitTotal {
		outcome.totalTruncated = true
		return outcome
	}
	if read.err != nil {
		code, class, message := "INTERNAL_ERROR", "internal", "source file could not be read"
		switch read.limit {
		case scanLimitMemory:
			code, class, message = "LIMIT_MEMORY", "resource", "accounted memory limit exceeded"
		case scanLimitSource:
			code, class, message = "LIMIT_SOURCE_BYTES", "resource", "source exceeds effective byte limit"
		}
		outcome.evaluation = scannerDiagnosticEvaluation(program, options, evaluationFailure(code, class, message, read.err), &candidate.path)
		return outcome
	}
	if bytes.IndexByte(read.content, 0) >= 0 {
		outcome.binary = true
		return outcome
	}
	evaluateOptions := options.EvaluateOptions
	evaluateOptions.Deadline = evaluationDeadline(started, evaluateOptions.MaxElapsed, evaluateOptions.Deadline)
	outcome.evaluation = EvaluateFile(ctx, program, FileInput{Path: candidate.path, Language: candidate.language, Content: read.content, PatternID: options.PatternID, Message: options.Message}, evaluateOptions)
	return outcome
}

type scanLimitKind uint8

const (
	scanLimitNone scanLimitKind = iota
	scanLimitSource
	scanLimitMemory
	scanLimitTotal
)

type scanReadResult struct {
	content []byte
	err     error
	limit   scanLimitKind
}

type scanByteAccount struct {
	mu      sync.Mutex
	notify  chan struct{}
	next    int
	total   int64
	maximum int64
	stopped bool
}

func newScanByteAccount(maximum int64) *scanByteAccount {
	return &scanByteAccount{maximum: maximum, notify: make(chan struct{})}
}

func (a *scanByteAccount) read(ctx context.Context, filesystem fs.FS, path string, content []byte, index int, options ScanOptions) scanReadResult {
	for {
		a.mu.Lock()
		if a.stopped {
			a.mu.Unlock()
			return scanReadResult{limit: scanLimitTotal, err: errSourceTooLarge}
		}
		if index == a.next {
			result := a.readNext(ctx, filesystem, path, content, options)
			a.mu.Unlock()
			return result
		}
		notify := a.notify
		a.mu.Unlock()
		select {
		case <-notify:
		case <-ctx.Done():
			return scanReadResult{err: ctx.Err()}
		}
	}
}

func (a *scanByteAccount) readNext(ctx context.Context, filesystem fs.FS, path string, provided []byte, options ScanOptions) scanReadResult {
	if err := ctx.Err(); err != nil {
		return scanReadResult{err: err}
	}
	remaining := a.maximum - a.total
	limit, kind := scanReadLimit(options, remaining)
	content, err := boundedCandidateContent(filesystem, path, provided, limit)
	result := scanReadResult{content: content, err: err}
	if err == errSourceTooLarge {
		result.limit = kind
		if kind == scanLimitTotal {
			a.stopped = true
			a.signalLocked()
			return result
		}
	}
	a.total += int64(len(content))
	a.next++
	a.signalLocked()
	return result
}

func (a *scanByteAccount) signalLocked() {
	close(a.notify)
	a.notify = make(chan struct{})
}

func scanReadLimit(options ScanOptions, remaining int64) (int, scanLimitKind) {
	sourceLimit := options.EvaluateOptions.MaxSourceBytes
	memoryLimit := options.EvaluateOptions.MaxMemoryBytes / int(sourceMemoryFactor)
	if remaining <= int64(sourceLimit) && remaining <= int64(memoryLimit) {
		return int(max(remaining, 0)), scanLimitTotal
	}
	if memoryLimit <= sourceLimit {
		return memoryLimit, scanLimitMemory
	}
	return sourceLimit, scanLimitSource
}

var errSourceTooLarge = &EvaluationError{Code: "LIMIT_SOURCE_BYTES", Class: "resource", Message: "source exceeds effective byte limit"}

func boundedCandidateContent(filesystem fs.FS, path string, provided []byte, maxBytes int) ([]byte, error) {
	if provided == nil {
		return readBoundedFile(filesystem, path, maxBytes)
	}
	if len(provided) > maxBytes {
		return provided[:maxBytes+1], errSourceTooLarge
	}
	return provided, nil
}

func readBoundedFile(filesystem fs.FS, path string, maxBytes int) ([]byte, error) {
	file, err := filesystem.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, int64(maxBytes)+1))
	if err != nil {
		return content, err
	}
	if len(content) > maxBytes {
		return content, errSourceTooLarge
	}
	return content, nil
}

func prepareScanCandidates(programLanguage string, candidates []ScanCandidate, options ScanOptions, stats *ScanStats) ([]preparedScanCandidate, []FileEvaluation, []ScanTruncation) {
	stats.Candidates = len(candidates)
	if !validScanGlobs(options.IncludeGlobs) || !validScanGlobs(options.ExcludeGlobs) {
		failure := scannerDiagnosticEvaluation(nil, options, evaluationFailure("INTERNAL_ERROR", "internal", "scan glob is invalid", nil), nil)
		return nil, []FileEvaluation{failure}, nil
	}
	var prepared []preparedScanCandidate
	var early []FileEvaluation
	for _, candidate := range candidates {
		entry, failure, disposition := prepareScanCandidate(programLanguage, candidate, options)
		switch disposition {
		case scanCandidatePrepared:
			prepared = append(prepared, entry)
		case scanCandidateInvalid:
			early = append(early, failure)
		case scanCandidateSkippedLanguage:
			stats.SkippedLanguage++
		case scanCandidateSkippedGlob:
			stats.SkippedGlob++
		}
	}
	sort.Slice(prepared, func(i, j int) bool {
		if prepared[i].path != prepared[j].path {
			return prepared[i].path < prepared[j].path
		}
		if prepared[i].readPath != prepared[j].readPath {
			return prepared[i].readPath < prepared[j].readPath
		}
		return prepared[i].content != nil && prepared[j].content == nil
	})
	prepared = deduplicatePreparedCandidates(prepared)
	stats.Eligible = len(prepared)
	if len(prepared) <= options.MaxFiles {
		return prepared, early, nil
	}
	skipped := len(prepared) - options.MaxFiles
	return prepared[:options.MaxFiles], early, []ScanTruncation{{Reason: "max_files", Limit: int64(options.MaxFiles), Skipped: skipped}}
}

type scanCandidateDisposition uint8

const (
	scanCandidatePrepared scanCandidateDisposition = iota
	scanCandidateInvalid
	scanCandidateSkippedLanguage
	scanCandidateSkippedGlob
)

func prepareScanCandidate(programLanguage string, candidate ScanCandidate, options ScanOptions) (preparedScanCandidate, FileEvaluation, scanCandidateDisposition) {
	normalized, ok := normalizeEvaluationPath(candidate.Path)
	if !ok {
		failure := scannerDiagnosticEvaluation(nil, options, evaluationFailure("PATH_INVALID", "source", "source path is not a safe repository-relative path", nil), nil)
		return preparedScanCandidate{}, failure, scanCandidateInvalid
	}
	language := candidate.Language
	if language == "" {
		language = parser.LanguageFor(normalized)
	}
	if language == "" || language != programLanguage {
		return preparedScanCandidate{}, FileEvaluation{}, scanCandidateSkippedLanguage
	}
	if !scanPathMatchesAny(normalized, options.IncludeGlobs) || scanPathMatchesAnyNonEmpty(normalized, options.ExcludeGlobs) {
		return preparedScanCandidate{}, FileEvaluation{}, scanCandidateSkippedGlob
	}
	if !fs.ValidPath(candidate.ReadPath) || candidate.ReadPath == "." {
		failure := scannerDiagnosticEvaluation(nil, options, evaluationFailure("INTERNAL_ERROR", "internal", "source file path is invalid", nil), &normalized)
		return preparedScanCandidate{}, failure, scanCandidateInvalid
	}
	return preparedScanCandidate{readPath: candidate.ReadPath, path: normalized, language: language, content: candidate.Content}, FileEvaluation{}, scanCandidatePrepared
}

func deduplicatePreparedCandidates(candidates []preparedScanCandidate) []preparedScanCandidate {
	out := candidates[:0]
	for _, candidate := range candidates {
		if len(out) == 0 || out[len(out)-1].path != candidate.path {
			out = append(out, candidate)
		}
	}
	return out
}

func finishScan(ctx context.Context, program *Program, state scanState, truncations []ScanTruncation) ScanResult {
	if state.totalTruncated {
		truncations = append(truncations, ScanTruncation{Reason: "max_total_bytes", Limit: state.options.MaxTotalBytes, Skipped: state.totalSkipped})
	}
	evaluation := normalizeEvaluationsAt(ctx, state.evaluations, state.options.EvaluateOptions, state.started, program)
	if len(state.evaluations) == 0 {
		evaluation.metadata = evaluationMetadataForProgram(state.options.EvaluateOptions, program)
	}
	return ScanResult{evaluation: evaluation, truncations: truncations, stats: state.stats}
}

func scannerFailure(options ScanOptions, message string, path *string) FileEvaluation {
	return scannerDiagnosticEvaluation(nil, options, evaluationFailure("INTERNAL_ERROR", "internal", message, nil), path)
}

func scannerDiagnosticEvaluation(program *Program, options ScanOptions, failure *EvaluationError, path *string) FileEvaluation {
	return FileEvaluation{diagnostics: []Diagnostic{newDiagnostic(failure.Code, failure.Class, failure.Message, stringPtr(options.PatternID), path, nil)}, metadata: evaluationMetadataForProgram(options.EvaluateOptions, program)}
}

func diagnosticPath(evaluation FileEvaluation) *string {
	for _, diagnostic := range evaluation.diagnostics {
		if diagnostic.path != nil {
			return stringPtr(*diagnostic.path)
		}
	}
	for _, finding := range evaluation.findings {
		return stringPtr(finding.path)
	}
	return nil
}

func estimateEvaluationMemory(evaluation FileEvaluation) int64 {
	var total int64
	for _, finding := range evaluation.findings {
		total += int64(len(finding.canonicalJSON) + len(finding.bindingJSON) + len(finding.text) + len(finding.path) + len(finding.patternID) + len(finding.message))
	}
	for _, diagnostic := range evaluation.diagnostics {
		encoded, _ := diagnostic.MarshalJSON()
		total += int64(len(encoded))
	}
	return total
}

func accountedSourceMemory(size int64) int64 {
	if size > int64(^uint64(0)>>1)/sourceMemoryFactor {
		return int64(^uint64(0) >> 1)
	}
	return size * sourceMemoryFactor
}

type scanMemoryAccount struct {
	mu       sync.Mutex
	notify   chan struct{}
	capacity int64
	current  int64
	retained int64
	next     int
}

func newScanMemoryAccount(capacity int64) *scanMemoryAccount {
	return &scanMemoryAccount{capacity: capacity, notify: make(chan struct{})}
}

func (a *scanMemoryAccount) acquire(ctx context.Context, amount int64) bool {
	for {
		a.mu.Lock()
		if a.retained+amount > a.capacity {
			a.mu.Unlock()
			return false
		}
		if a.current+amount <= a.capacity {
			a.current += amount
			a.mu.Unlock()
			return true
		}
		notify := a.notify
		a.mu.Unlock()
		select {
		case <-notify:
		case <-ctx.Done():
			return false
		}
	}
}

func (a *scanMemoryAccount) replace(ctx context.Context, index int, oldAmount, newAmount int64) bool {
	for {
		a.mu.Lock()
		if index == a.next {
			a.current -= oldAmount
			if newAmount > oldAmount || a.retained+newAmount > a.capacity {
				a.signalLocked()
				a.mu.Unlock()
				return false
			}
			a.current += newAmount
			a.signalLocked()
			a.mu.Unlock()
			return true
		}
		notify := a.notify
		a.mu.Unlock()
		select {
		case <-notify:
		case <-ctx.Done():
			a.release(oldAmount)
			return false
		}
	}
}

func (a *scanMemoryAccount) retain(index int, amount int64) {
	a.mu.Lock()
	if index == a.next {
		a.retained += amount
		a.next++
	}
	a.signalLocked()
	a.mu.Unlock()
}

func (a *scanMemoryAccount) discard(index int, amount int64) {
	a.mu.Lock()
	if index == a.next {
		a.current -= amount
		a.next++
	}
	a.signalLocked()
	a.mu.Unlock()
}

func (a *scanMemoryAccount) release(amount int64) {
	if amount == 0 {
		return
	}
	a.mu.Lock()
	a.current -= amount
	a.signalLocked()
	a.mu.Unlock()
}

func (a *scanMemoryAccount) signalLocked() {
	close(a.notify)
	a.notify = make(chan struct{})
}

func scanSourceReservation(options ScanOptions) int64 {
	readLimit := min(options.EvaluateOptions.MaxSourceBytes, options.EvaluateOptions.MaxMemoryBytes/int(sourceMemoryFactor))
	reservation := max(accountedSourceMemory(int64(readLimit)), minimumScanReservation)
	return min(reservation, int64(options.EvaluateOptions.MaxMemoryBytes))
}

func hasEvaluationDiagnostic(evaluation FileEvaluation, code string) bool {
	for _, diagnostic := range evaluation.diagnostics {
		if diagnostic.code == code {
			return true
		}
	}
	return false
}

func normalizeScanOptions(options ScanOptions) ScanOptions {
	options.EvaluateOptions = normalizeEvaluateOptions(options.EvaluateOptions)
	options.Workers = boundedOption(options.Workers, defaultScanWorkers, hardScanWorkers)
	perWorker := scanSourceReservation(options)
	memoryWorkers := int64(options.EvaluateOptions.MaxMemoryBytes) / perWorker
	if memoryWorkers < 1 {
		memoryWorkers = 1
	}
	if int64(options.Workers) > memoryWorkers {
		options.Workers = int(memoryWorkers)
	}
	options.MaxFiles = boundedOption(options.MaxFiles, defaultScanFiles, hardScanFiles)
	if options.MaxTotalBytes <= 0 {
		options.MaxTotalBytes = defaultScanTotalBytes
	} else if options.MaxTotalBytes > hardScanTotalBytes {
		options.MaxTotalBytes = hardScanTotalBytes
	}
	return options
}

func scanBatchFailure(ctx context.Context, options EvaluateOptions, started time.Time) *EvaluationError {
	if err := ctx.Err(); err != nil {
		return evaluationFailure("EVALUATION_CANCELLED", "cancelled", "evaluation cancelled", err)
	}
	deadline := evaluationDeadline(started, options.MaxBatchElapsed, options.Deadline)
	if deadlineExceeded(deadline) {
		return evaluationFailure("LIMIT_TIME_BATCH", "resource", "batch evaluation deadline exceeded", nil)
	}
	return nil
}

func scanWaitDuration(options EvaluateOptions, started time.Time) time.Duration {
	deadline := evaluationDeadline(started, options.MaxBatchElapsed, options.Deadline)
	if deadline.IsZero() {
		return time.Duration(1<<63 - 1)
	}
	remaining := time.Until(deadline)
	if remaining <= 0 {
		return time.Nanosecond
	}
	return remaining
}

// ValidateGlobs rejects malformed include or exclude scan patterns.
func ValidateGlobs(include, exclude []string) error {
	if !validScanGlobs(include) || !validScanGlobs(exclude) {
		return fmt.Errorf("structural glob is invalid")
	}
	return nil
}

func validScanGlobs(patterns []string) bool {
	for _, pattern := range patterns {
		for _, segment := range strings.Split(filepath.ToSlash(pattern), "/") {
			if segment == "**" {
				continue
			}
			if _, err := filepath.Match(segment, ""); err != nil {
				return false
			}
		}
	}
	return true
}

func scanPathMatchesAny(path string, patterns []string) bool {
	return len(patterns) == 0 || scanPathMatchesAnyNonEmpty(path, patterns)
}

func scanPathMatchesAnyNonEmpty(name string, patterns []string) bool {
	for _, pattern := range patterns {
		if scanGlobMatch(pattern, name) {
			return true
		}
	}
	return false
}

func scanGlobMatch(pattern, name string) bool {
	pattern = filepath.ToSlash(pattern)
	name = filepath.ToSlash(name)
	if !strings.Contains(pattern, "**") {
		matched, err := filepath.Match(filepath.FromSlash(pattern), filepath.FromSlash(name))
		return err == nil && matched
	}
	return scanMatchSegments(strings.Split(pattern, "/"), strings.Split(name, "/"))
}

type scanSegmentState struct {
	pattern int
	name    int
}

type scanSegmentMatcher struct {
	pattern []string
	name    []string
	seen    map[scanSegmentState]bool
	values  map[scanSegmentState]bool
}

func scanMatchSegments(pattern, name []string) bool {
	matcher := scanSegmentMatcher{pattern: pattern, name: name, seen: make(map[scanSegmentState]bool), values: make(map[scanSegmentState]bool)}
	return matcher.match(0, 0)
}

func (m *scanSegmentMatcher) match(patternIndex, nameIndex int) bool {
	state := scanSegmentState{patternIndex, nameIndex}
	if m.seen[state] {
		return m.values[state]
	}
	m.seen[state] = true
	matched := m.matchUncached(patternIndex, nameIndex)
	m.values[state] = matched
	return matched
}

func (m *scanSegmentMatcher) matchUncached(patternIndex, nameIndex int) bool {
	if patternIndex == len(m.pattern) {
		return nameIndex == len(m.name)
	}
	if m.pattern[patternIndex] == "**" {
		return m.matchDoubleStar(patternIndex, nameIndex)
	}
	if nameIndex == len(m.name) {
		return false
	}
	matched, err := filepath.Match(m.pattern[patternIndex], m.name[nameIndex])
	return err == nil && matched && m.match(patternIndex+1, nameIndex+1)
}

func (m *scanSegmentMatcher) matchDoubleStar(patternIndex, nameIndex int) bool {
	if m.match(patternIndex+1, nameIndex) {
		return true
	}
	return nameIndex < len(m.name) && m.match(patternIndex, nameIndex+1)
}

func nonNilTruncations(values []ScanTruncation) []ScanTruncation {
	if values == nil {
		return []ScanTruncation{}
	}
	return values
}
