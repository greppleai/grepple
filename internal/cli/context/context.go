// Package context implements context-guard lifecycle commands.
package context

import (
	"fmt"
	"strings"

	cliruntime "github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/render"
)

// InvalidateArgs contains context invalidation arguments.
type InvalidateArgs struct {
	Reason string `arg:"--reason" placeholder:"REASON" help:"reason recorded for invalidation"`
}

// Args contains context command subcommands.
type Args struct {
	Invalidate *InvalidateArgs `arg:"subcommand:invalidate"`
}

type command struct{ context cliruntime.Context }

// New constructs the context command.
func New(context cliruntime.Context) cliruntime.Command { return &command{context: context} }

// Run executes the context command.
func (command *command) Run(args []string) error {
	application := command.context
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		return cliruntime.NewOutput(application.Stdout()).WriteString("Manage session-agnostic structural-segment context deduplication.\nUsage:\n  grepple context invalidate [--reason REASON]\n")
	}
	if len(args) == 0 || args[0] != "invalidate" {
		return fmt.Errorf("usage: grepple context invalidate [--reason REASON]")
	}
	reason, err := parseInvalidationReason(args[1:])
	if err != nil {
		return err
	}
	return Execute(command.context, &Args{Invalidate: &InvalidateArgs{Reason: reason}})
}

// Execute applies an application-parsed context command.
func Execute(_ cliruntime.Context, values *Args) error {
	if values == nil || values.Invalidate == nil {
		return fmt.Errorf("usage: grepple context invalidate [--reason REASON]")
	}
	reason := strings.TrimSpace(values.Invalidate.Reason)
	if reason == "" {
		reason = "manual"
	}
	return render.InvalidateContext(reason)
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
