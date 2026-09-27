// Package linerange resolves inclusive source-line requests against file length.
package linerange

import (
	"fmt"
	"strings"
)

// Outcome classifies how a requested range intersects a source file.
type Outcome string

const (
	// OutcomeExact means the requested range was returned without clamping.
	OutcomeExact Outcome = "exact"
	// OutcomePartialMiss means the start existed and the end was clamped to EOF.
	OutcomePartialMiss Outcome = "partial-miss"
	// OutcomeFullMiss means the requested start was beyond EOF.
	OutcomeFullMiss Outcome = "full-miss"

	// HeaderOutcome carries Outcome across raw HTTP reads.
	HeaderOutcome = "X-Grepple-Line-Range-Outcome"
	// HeaderWarning carries a successful EOF-clamp warning across raw HTTP reads.
	HeaderWarning = "X-Grepple-Line-Range-Warning"
)

// Result describes an inclusive requested range and the range actually returned.
type Result struct {
	RequestedStart int     `json:"requestedStart"`
	RequestedEnd   int     `json:"requestedEnd"`
	ReturnedStart  int     `json:"returnedStart,omitempty"`
	ReturnedEnd    int     `json:"returnedEnd,omitempty"`
	FileLines      int     `json:"fileLines"`
	Outcome        Outcome `json:"outcome"`
	Warning        string  `json:"warning,omitempty"`
}

// OutsideError reports a range whose first line is already beyond EOF.
type OutsideError struct {
	Result Result
}

func (err *OutsideError) Error() string {
	return fmt.Sprintf("requested line range %d-%d is outside file (1-%d)", err.Result.RequestedStart, err.Result.RequestedEnd, err.Result.FileLines)
}

// Resolve validates and clamps an inclusive range against lineCount.
func Resolve(start, end, lineCount int) (Result, error) {
	result := Result{RequestedStart: start, RequestedEnd: end, FileLines: lineCount, Outcome: OutcomeExact}
	if start < 1 || end < start {
		return result, fmt.Errorf("line range must be positive and ascending")
	}
	if lineCount < 1 {
		return result, fmt.Errorf("file line count must be positive")
	}
	if start > lineCount {
		result.Outcome = OutcomeFullMiss
		return result, &OutsideError{Result: result}
	}
	result.ReturnedStart, result.ReturnedEnd = start, end
	if end > lineCount {
		result.ReturnedEnd = lineCount
		result.Outcome = OutcomePartialMiss
		result.Warning = fmt.Sprintf("EOF: requested line range %d-%d exceeds file length %d; returned %d-%d", start, end, lineCount, start, lineCount)
	}
	return result, nil
}

// SplitLines uses editor-style line counts: a final newline does not create an
// additional empty line, while an empty file still has one addressable line.
func SplitLines(content string) []string {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		return []string{""}
	}
	return lines
}
