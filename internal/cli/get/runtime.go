// Package get implements indexed source-file retrieval.
package get

import (
	"errors"
	"fmt"

	"github.com/greppleai/grepple/internal/cliruntime"
	rendercommand "github.com/greppleai/grepple/internal/render"
	"github.com/greppleai/grepple/linerange"
)

func reportRangeError(application cliruntime.Context, err error, remoteFullMiss bool) error {
	var outside *linerange.OutsideError
	if !remoteFullMiss && !errors.As(err, &outside) {
		return err
	}
	rendercommand.RecordLineRangeError(err, application.Configuration().ContextGuardEnabled())
	fmt.Fprintln(application.Stderr(), "error:", err)
	application.RequestExit(1)
	return nil
}
