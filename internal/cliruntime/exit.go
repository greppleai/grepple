package cliruntime

import "sync/atomic"

// ExitState records a command-requested process status without treating it as an execution failure.
type ExitState struct {
	requested atomic.Int32
}

// Request records a positive exit status.
func (state *ExitState) Request(code int) {
	if code > 0 {
		state.requested.Store(int32(code))
	}
}

// Reset clears the requested exit status.
func (state *ExitState) Reset() { state.requested.Store(0) }

// Requested returns the currently requested exit status.
func (state *ExitState) Requested() int { return int(state.requested.Load()) }

// ExitError carries an intentional command exit status to the process boundary.
type ExitError struct{ code int }

func (err ExitError) Error() string { return "command exit status" }

// NewExitError creates an intentional command exit status.
func NewExitError(code int) error { return ExitError{code: code} }

// ExitCode extracts an intentional command exit status.
func ExitCode(err error) (int, bool) {
	status, ok := err.(ExitError)
	return status.code, ok
}
