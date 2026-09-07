// Package command provides the shared process boundary for grepple binaries.
package command

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// Run executes a command and translates domain errors into process exit codes.
func Run(args []string, execute func([]string) error) {
	if err := execute(args); err != nil {
		if errors.Is(err, syscall.EPIPE) {
			return
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}
