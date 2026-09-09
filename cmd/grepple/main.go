// Package main is the process entry point for the Grepple CLI.
package main

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/greppleai/grepple/internal/cli"
)

func main() {
	// Convert a downstream pipe closing into EPIPE so the CLI can return cleanly.
	signal.Ignore(syscall.SIGPIPE)
	if err := cli.Run(os.Args[1:]); err != nil {
		if errors.Is(err, syscall.EPIPE) {
			return
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}
