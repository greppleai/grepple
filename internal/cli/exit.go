package cli

import (
	"errors"
	"fmt"
	"os"
	"sync/atomic"

	"github.com/greppleai/grepple/linerange"
)

var requestedExitCode atomic.Int32

type commandExitError struct{ code int }

type remoteFullLineRangeMissError struct{ err error }

func (err remoteFullLineRangeMissError) Error() string { return err.err.Error() }
func (err remoteFullLineRangeMissError) Unwrap() error { return err.err }

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

func reportLineRangeCommandError(err error) error {
	var local *linerange.OutsideError
	var remote remoteFullLineRangeMissError
	if !errors.As(err, &local) && !errors.As(err, &remote) {
		return err
	}
	fmt.Fprintln(os.Stderr, "error:", err)
	requestExit(1)
	return nil
}
