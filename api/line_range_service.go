package api

import (
	"errors"

	"github.com/greppleai/grepple/internal/linerange"
)

// LineRangeOutcome describes how an inclusive source range intersects a file.
type LineRangeOutcome string

// LineRangeExact, LineRangePartialMiss, and LineRangeFullMiss classify range outcomes.
const (
	LineRangeExact       LineRangeOutcome = LineRangeOutcome(linerange.OutcomeExact)
	LineRangePartialMiss LineRangeOutcome = LineRangeOutcome(linerange.OutcomePartialMiss)
	LineRangeFullMiss    LineRangeOutcome = LineRangeOutcome(linerange.OutcomeFullMiss)

	// HeaderLineRangeOutcome and HeaderLineRangeWarning carry raw-file range status.
	HeaderLineRangeOutcome = linerange.HeaderOutcome
	HeaderLineRangeWarning = linerange.HeaderWarning
)

// SourceLineRange is the validated, possibly EOF-clamped source range.
type SourceLineRange struct {
	RequestedStart, RequestedEnd int
	ReturnedStart, ReturnedEnd   int
	FileLines                    int
	Outcome                      LineRangeOutcome
	Warning                      string
}

// ResolveSourceLineRange validates and clamps a requested inclusive range.
func ResolveSourceLineRange(start, end, lineCount int) (SourceLineRange, error) {
	result, err := linerange.Resolve(start, end, lineCount)
	return SourceLineRange{
		RequestedStart: result.RequestedStart, RequestedEnd: result.RequestedEnd,
		ReturnedStart: result.ReturnedStart, ReturnedEnd: result.ReturnedEnd,
		FileLines: result.FileLines, Outcome: LineRangeOutcome(result.Outcome), Warning: result.Warning,
	}, err
}

// IsOutsideSourceLineRange identifies an authoritative first-line-beyond-EOF error.
func IsOutsideSourceLineRange(err error) bool {
	var outside *linerange.OutsideError
	return errors.As(err, &outside)
}
