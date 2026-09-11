package gritql

import (
	"bytes"
	"context"
	"io/fs"
	"sort"
	"time"
	"unicode/utf8"

	"github.com/greppleai/grepple/parser"
)

// ProgramScan identifies one immutable program in a shared file scan.
type ProgramScan struct {
	Program   *Program
	PatternID string
	Message   string
}

// ProgramScanResult is one program's normalized result from a shared scan.
type ProgramScanResult struct {
	PatternID string
	Result    ScanResult
}

// BatchScanStats reports work shared across programs.
type BatchScanStats struct {
	FilesRead   int
	FilesParsed int
}

// BatchScanResult contains per-program results in input order and shared work statistics.
type BatchScanResult struct {
	programs []ProgramScanResult
	stats    BatchScanStats
}

// Programs returns a copy of the per-program scan results.
func (r BatchScanResult) Programs() []ProgramScanResult {
	return append([]ProgramScanResult(nil), r.programs...)
}

// Stats returns shared acquisition and parse counts.
func (r BatchScanResult) Stats() BatchScanStats { return r.stats }

// ScanFilesPrograms reads and parses each eligible file once, then evaluates every
// same-language program against the shared immutable document. Mixed-language rules
// share acquisition while retaining per-language parsing and program-local identity.
func ScanFilesPrograms(ctx context.Context, filesystem fs.FS, programs []ProgramScan, candidates []ScanCandidate, options ScanOptions) BatchScanResult {
	if ctx == nil {
		ctx = context.Background()
	}
	options = normalizeScanOptions(options)
	states, prepared := prepareProgramScan(programs, candidates, options)
	batch := BatchScanResult{}
	if scanFilesystemUnavailable(filesystem, prepared) {
		for index := range states {
			states[index].evaluations = append(states[index].evaluations, scannerFailure(states[index].options, "source filesystem is unavailable", nil))
		}
		return finishProgramScan(ctx, programs, states, batch)
	}
	bytesAccount := newScanByteAccount(options.MaxTotalBytes)
	readIndex := 0
	for index, candidate := range prepared {
		if stopProgramBatch(ctx, programs, states) {
			break
		}
		if !activeProgramMatchesCandidate(programs, states, candidate) {
			continue
		}
		read := bytesAccount.read(ctx, filesystem, candidate.readPath, candidate.content, readIndex, options)
		readIndex++
		batch.stats.FilesRead++
		if read.limit == scanLimitTotal {
			for stateIndex := range states {
				skipped := matchingCandidateCount(programs[stateIndex].Program, prepared[index:])
				states[stateIndex].totalTruncated = skipped > 0
				states[stateIndex].totalSkipped = skipped
			}
			break
		}
		commitProgramRead(ctx, programs, states, candidate, read, &batch)
	}
	return finishProgramScan(ctx, programs, states, batch)
}

type preparedLanguageScan struct {
	candidates  []preparedScanCandidate
	early       []FileEvaluation
	truncations []ScanTruncation
	stats       ScanStats
}

func prepareProgramScan(programs []ProgramScan, candidates []ScanCandidate, options ScanOptions) ([]scanState, []preparedScanCandidate) {
	states := make([]scanState, len(programs))
	preparedByPath := make(map[string]preparedScanCandidate)
	byLanguage := make(map[string]preparedLanguageScan)
	for index, program := range programs {
		programOptions := options
		programOptions.PatternID = program.PatternID
		programOptions.Message = program.Message
		state := scanState{options: programOptions, started: time.Now()}
		if program.Program == nil || program.Program.root == nil {
			state.evaluations = append(state.evaluations, scannerDiagnosticEvaluation(program.Program, programOptions, evaluationFailure("INTERNAL_ERROR", "internal", "invalid compiled program", nil), nil))
			state.stop = true
			states[index] = state
			continue
		}
		language := program.Program.Language()
		languageScan, prepared := byLanguage[language]
		if !prepared {
			languageScan.candidates, languageScan.early, languageScan.truncations = prepareScanCandidates(language, candidates, programOptions, &languageScan.stats)
			byLanguage[language] = languageScan
			for _, candidate := range languageScan.candidates {
				preparedByPath[candidate.path] = candidate
			}
		}
		state.stats = languageScan.stats
		state.evaluations = cloneProgramScanFailures(languageScan.early, program.Program, programOptions)
		state.truncations = append([]ScanTruncation(nil), languageScan.truncations...)
		state.candidateCount = len(languageScan.candidates)
		states[index] = state
	}
	prepared := make([]preparedScanCandidate, 0, len(preparedByPath))
	for _, candidate := range preparedByPath {
		prepared = append(prepared, candidate)
	}
	sort.Slice(prepared, func(i, j int) bool {
		if prepared[i].path != prepared[j].path {
			return prepared[i].path < prepared[j].path
		}
		return prepared[i].readPath < prepared[j].readPath
	})
	return states, prepared
}
func cloneProgramScanFailures(source []FileEvaluation, program *Program, options ScanOptions) []FileEvaluation {
	var cloned []FileEvaluation
	for _, evaluation := range source {
		for _, diagnostic := range evaluation.diagnostics {
			failure := evaluationFailure(diagnostic.code, diagnostic.class, diagnostic.message, nil)
			cloned = append(cloned, scannerDiagnosticEvaluation(program, options, failure, diagnostic.path))
		}
	}
	return cloned
}

func stopProgramBatch(ctx context.Context, programs []ProgramScan, states []scanState) bool {
	active := false
	for index := range states {
		if states[index].stop {
			continue
		}
		if failure := scanBatchFailure(ctx, states[index].options.EvaluateOptions, states[index].started); failure != nil {
			states[index].evaluations = append(states[index].evaluations, scannerDiagnosticEvaluation(programs[index].Program, states[index].options, failure, nil))
			states[index].stop = true
			continue
		}
		active = true
	}
	return !active
}

func commitProgramRead(ctx context.Context, programs []ProgramScan, states []scanState, candidate preparedScanCandidate, read scanReadResult, batch *BatchScanResult) {
	accountProgramRead(programs, states, candidate, len(read.content))
	if read.err != nil {
		appendProgramReadFailure(programs, states, candidate, read)
		return
	}
	if bytes.IndexByte(read.content, 0) >= 0 {
		accountProgramBinary(programs, states, candidate)
		return
	}
	if !utf8.Valid(read.content) {
		appendProgramSourceFailure(programs, states, candidate, "SOURCE_INVALID_UTF8", "source is not valid UTF-8")
		return
	}
	document, err := parser.ParseDocument(candidate.language, string(read.content))
	if err != nil {
		appendProgramSourceFailure(programs, states, candidate, "INTERNAL_ERROR", "source parser failed")
		return
	}
	batch.stats.FilesParsed++
	defer document.Close()
	for index, program := range programs {
		if states[index].stop || !programMatchesCandidate(program.Program, candidate) {
			continue
		}
		evaluation := EvaluateDocumentFindings(ctx, program.Program, document, DocumentInput{Path: candidate.path, PatternID: program.PatternID, Message: program.Message}, states[index].options.EvaluateOptions)
		commitProgramEvaluation(program.Program, &states[index], evaluation)
	}
}
func programMatchesCandidate(program *Program, candidate preparedScanCandidate) bool {
	return program != nil && program.Language() == candidate.language
}

func activeProgramMatchesCandidate(programs []ProgramScan, states []scanState, candidate preparedScanCandidate) bool {
	for index := range states {
		if !states[index].stop && programMatchesCandidate(programs[index].Program, candidate) {
			return true
		}
	}
	return false
}

func matchingCandidateCount(program *Program, candidates []preparedScanCandidate) int {
	count := 0
	for _, candidate := range candidates {
		if programMatchesCandidate(program, candidate) {
			count++
		}
	}
	return count
}

func appendProgramReadFailure(programs []ProgramScan, states []scanState, candidate preparedScanCandidate, read scanReadResult) {
	code, class, message := "INTERNAL_ERROR", "internal", "source file could not be read"
	if read.limit == scanLimitMemory {
		code, class, message = "LIMIT_MEMORY", "resource", "accounted memory limit exceeded"
	} else if read.limit == scanLimitSource {
		code, class, message = "LIMIT_SOURCE_BYTES", "resource", "source exceeds effective byte limit"
	}
	for index := range states {
		if !states[index].stop && programMatchesCandidate(programs[index].Program, candidate) {
			states[index].evaluations = append(states[index].evaluations, scannerDiagnosticEvaluation(programs[index].Program, states[index].options, evaluationFailure(code, class, message, read.err), &candidate.path))
		}
	}
}
func accountProgramRead(programs []ProgramScan, states []scanState, candidate preparedScanCandidate, size int) {
	for index := range states {
		if !states[index].stop && programMatchesCandidate(programs[index].Program, candidate) {
			states[index].stats.BytesRead += int64(size)
		}
	}
}

func accountProgramBinary(programs []ProgramScan, states []scanState, candidate preparedScanCandidate) {
	for index := range states {
		if !states[index].stop && programMatchesCandidate(programs[index].Program, candidate) {
			states[index].stats.SkippedBinary++
		}
	}
}

func appendProgramSourceFailure(programs []ProgramScan, states []scanState, candidate preparedScanCandidate, code, message string) {
	for index := range states {
		if !states[index].stop && programMatchesCandidate(programs[index].Program, candidate) {
			states[index].evaluations = append(states[index].evaluations, scannerDiagnosticEvaluation(programs[index].Program, states[index].options, evaluationFailure(code, "source", message, nil), &candidate.path))
		}
	}
}

func commitProgramEvaluation(program *Program, state *scanState, evaluation FileEvaluation) {
	estimate := estimateEvaluationMemory(evaluation)
	if state.retained+estimate > int64(state.options.EvaluateOptions.MaxMemoryBytes) {
		path := diagnosticPath(evaluation)
		state.evaluations = append(state.evaluations, scannerDiagnosticEvaluation(program, state.options, evaluationFailure("LIMIT_MEMORY", "resource", "accounted memory limit exceeded", nil), path))
		state.stop = true
		return
	}
	state.retained += estimate
	state.stats.Evaluated++
	state.evaluations = append(state.evaluations, evaluation)
}

func finishProgramScan(ctx context.Context, programs []ProgramScan, states []scanState, batch BatchScanResult) BatchScanResult {
	batch.programs = make([]ProgramScanResult, len(programs))
	for index, program := range programs {
		programTruncations := append([]ScanTruncation(nil), states[index].truncations...)
		batch.programs[index] = ProgramScanResult{PatternID: program.PatternID, Result: finishScan(ctx, program.Program, states[index], programTruncations)}
	}
	return batch
}
