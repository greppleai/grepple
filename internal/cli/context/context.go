// Package context implements context-guard lifecycle commands.
package context

import (
	"fmt"
	"io"
	"os"
	"strings"

	cliruntime "github.com/greppleai/grepple/internal/cli/runtime"
)

// Dependencies supplies process-owned context-guard operations.
type Dependencies struct {
	Stdout     io.Writer
	Invalidate func(reason string) error
}

// Run executes the context command.
func Run(args []string, dependencies Dependencies) error {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		return cliruntime.NewOutput(dependencies.stdout()).WriteString("Manage session-agnostic structural-segment context deduplication.\nUsage:\n  grepple context invalidate [--reason REASON]\n")
	}
	if len(args) == 0 || args[0] != "invalidate" {
		return fmt.Errorf("usage: grepple context invalidate [--reason REASON]")
	}
	reason, err := parseInvalidationReason(args[1:])
	if err != nil {
		return err
	}
	if dependencies.Invalidate == nil {
		return fmt.Errorf("context invalidation is unavailable")
	}
	return dependencies.Invalidate(reason)
}

func (dependencies Dependencies) stdout() io.Writer {
	if dependencies.Stdout != nil {
		return dependencies.Stdout
	}
	return os.Stdout
}

func parseInvalidationReason(args []string) (string, error) {
	reason := "manual"
	for index := 0; index < len(args); index++ {
		switch {
		case args[index] == "--reason" && index+1 < len(args):
			reason = strings.TrimSpace(args[index+1])
			index++
		case strings.HasPrefix(args[index], "--reason="):
			reason = strings.TrimSpace(strings.TrimPrefix(args[index], "--reason="))
		default:
			return "", fmt.Errorf("unknown context invalidate argument %q", args[index])
		}
	}
	if reason == "" {
		return "", fmt.Errorf("--reason requires a value")
	}
	return reason, nil
}
