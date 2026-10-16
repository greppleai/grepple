// Package main is the process entry point for the Grepple CLI.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/greppleai/grepple/internal/cli"
	"github.com/greppleai/grepple/internal/grepguard"
)

func main() {
	// Convert a downstream pipe closing into EPIPE so the CLI can return cleanly.
	signal.Ignore(syscall.SIGPIPE)
	if len(os.Args) == 2 && os.Args[1] == "claude-grep-guard" {
		// Hook input is untrusted; malformed or oversized payloads fail open.
		input, err := io.ReadAll(io.LimitReader(os.Stdin, 1<<20))
		if err == nil {
			_, _ = os.Stdout.Write(grepguard.HandleClaudeHook(input))
		}
		return
	}
	if err := cli.Run(os.Args[1:]); err != nil {
		if code, ok := cli.ExitCode(err); ok {
			os.Exit(code)
		}
		if errors.Is(err, syscall.EPIPE) {
			return
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}
