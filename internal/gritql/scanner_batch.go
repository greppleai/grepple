package gritql

import (
	"bytes"
	"context"
	"io/fs"
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
// supplied program against the shared immutable document. All programs share the
// scope and acquisition limits in options; finding identity remains program-local.
func ScanFilesPrograms(ctx context.Context, filesystem fs.FS, programs []ProgramScan, candidates []ScanCandidate, options ScanOptions) BatchScanResult {
	if ctx == nil {
		ctx = context.Background()
	}
	options = normalizeScanOptions(options)
	states, prepared, truncations := prepareProgramScan(programs, candidates, options)
	batch := BatchScanResult{}
	if filesystem == nil {
		for index := range states {
			states[index].evaluations = append(states[index].evaluations, scannerFailure(states[index].options, "source filesystem is unavailable", nil))
		}
		return finishProgramScan(ctx, programs, states, truncations, batch)
	}
	bytesAccount := newScanByteAccount(options.MaxTotalBytes)
	for index, candidate := range prepared {
		if stopProgramBatch(ctx, programs, states) {
			break
		}
		read := bytesAccount.read(ctx, filesystem, candidate.readPath, index, options)
		batch.stats.FilesRead++
		if read.limit == scanLimitTotal {
			for stateIndex := range states {
				states[stateIndex].totalTruncated = true
				states[stateIndex].totalSkipped = len(prepared) - index
			}
			break
		}
		commitProgramRead(ctx, programs, states, candidate, read, &batch)
	}
	return finishProgramScan(ctx, programs, states, truncations, batch)
}

func prepareProgramScan(programs []ProgramScan, candidates []ScanCandidate, options ScanOptions) ([]scanState, []preparedScanCandidate, []ScanTruncation) {
	states := make([]scanState, len(programs))
	var prepared []preparedScanCandidate
	var truncations []ScanTruncation
	for index, program := range programs {
		programOptions := options
		programOptions.PatternID = program.PatternID
		programOptions.Message = program.Message
		states[index] = scanState{options: programOptions, started: time.Now()}
		if index == 0 {
			prepared, states[index].evaluations, truncations = prepareScanCandidates(candidates, programOptions, &states[index].stats)
			continue
		}
		states[index].stats = states[0].stats
		states[index].evaluations = cloneScannerEvaluations(states[0].evaluations, program.Program, programOptions)
	}
	for index := range states {
		states[index].candidateCount = len(prepared)
	}
	return states, prepared, truncations
}

func cloneScannerEvaluations(source []FileEvaluation, program *Program, options ScanOptions) []FileEvaluation {
	cloned := make([]FileEvaluation, 0, len(source))
	for _, evaluation := range source {
		if len(evaluation.diagnostics) == 0 {
			continue
		}
		diagnostic := evaluation.diagnostics[0]
		failure := evaluationFailure(diagnostic.code, diagnostic.class, diagnostic.message, nil)
		cloned = append(cloned, scannerDiagnosticEvaluation(program, options, failure, diagnostic.path))
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
	for index := range states {
		if !states[index].stop {
			states[index].stats.BytesRead += int64(len(read.content))
		}
	}
	if read.err != nil {
		appendProgramReadFailure(programs, states, candidate, read)
		return
	}
	if bytes.IndexByte(read.content, 0) >= 0 {
		for index := range states {
			if !states[index].stop {
				states[index].stats.SkippedBinary++
			}
		}
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
		if states[index].stop {
			continue
		}
		evaluation := EvaluateDocumentFindings(ctx, program.Program, document, DocumentInput{Path: candidate.path, PatternID: program.PatternID, Message: program.Message}, states[index].options.EvaluateOptions)
		commitProgramEvaluation(program.Program, &states[index], evaluation)
	}
}

func appendProgramReadFailure(programs []ProgramScan, states []scanState, candidate preparedScanCandidate, read scanReadResult) {
	code, class, message := "INTERNAL_ERROR", "internal", "source file could not be read"
	if read.limit == scanLimitMemory {
		code, class, message = "LIMIT_MEMORY", "resource", "accounted memory limit exceeded"
	} else if read.limit == scanLimitSource {
		code, class, message = "LIMIT_SOURCE_BYTES", "resource", "source exceeds effective byte limit"
	}
	for index := range states {
		if !states[index].stop {
			states[index].evaluations = append(states[index].evaluations, scannerDiagnosticEvaluation(programs[index].Program, states[index].options, evaluationFailure(code, class, message, read.err), &candidate.path))
		}
	}
}

func appendProgramSourceFailure(programs []ProgramScan, states []scanState, candidate preparedScanCandidate, code, message string) {
	for index := range states {
		if !states[index].stop {
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

func finishProgramScan(ctx context.Context, programs []ProgramScan, states []scanState, truncations []ScanTruncation, batch BatchScanResult) BatchScanResult {
	batch.programs = make([]ProgramScanResult, len(programs))
	for index, program := range programs {
		batch.programs[index] = ProgramScanResult{PatternID: program.PatternID, Result: finishScan(ctx, program.Program, states[index], append([]ScanTruncation(nil), truncations...))}
	}
	return batch
}
