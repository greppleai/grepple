// Package get implements indexed source-file retrieval.
package get

import (
	"errors"
	"fmt"

	"github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/linerange"
)

func reportRangeError(application cliruntime.Context, err error, remoteFullMiss bool) error {
	var outside *linerange.OutsideError
	if !remoteFullMiss && !errors.As(err, &outside) {
		return err
	}
	fmt.Fprintln(application.Stderr(), "error:", err)
	application.RequestExit(1)
	return nil
}
