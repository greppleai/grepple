package cli

import "sync/atomic"

var requestedExitCode atomic.Int32

type commandExitError struct{ code int }

func (err commandExitError) Error() string { return "command exit status" }

func requestExit(code int) {
	if code > 0 {
		requestedExitCode.Store(int32(code))
	}
}

func resetRequestedExit() { requestedExitCode.Store(0) }

func requestedExit() int { return int(requestedExitCode.Load()) }

// ExitCode returns a requested non-error command status such as no matches.
func ExitCode(err error) (int, bool) {
	status, ok := err.(commandExitError)
	return status.code, ok
}
