package cli

import (
	"errors"
	"fmt"
	"os"

	cliruntime "github.com/greppleai/grepple/internal/cli/runtime"
	"github.com/greppleai/grepple/linerange"
)

var commandExitState cliruntime.ExitState

type remoteFullLineRangeMissError struct{ err error }

func (err remoteFullLineRangeMissError) Error() string { return err.err.Error() }
func (err remoteFullLineRangeMissError) Unwrap() error { return err.err }

func requestExit(code int) { commandExitState.Request(code) }

func resetRequestedExit() { commandExitState.Reset() }

func requestedExit() int { return commandExitState.Requested() }

// ExitCode returns a requested non-error command status such as no matches.
func ExitCode(err error) (int, bool) { return cliruntime.ExitCode(err) }

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
