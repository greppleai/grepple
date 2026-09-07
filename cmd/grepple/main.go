// Package main is the grepple CLI entry point: a single query is routed to
// the local shard (spawning one if needed), searched locally, or fanned out
// through a remote router depending on the flags and config.
package main

import (
	"os"
	"os/signal"
	"syscall"

	"grepple/internal/command"
	"grepple/internal/grepplecli"
)

func main() {
	// Convert a downstream pipe closing into EPIPE so the CLI can return cleanly.
	signal.Ignore(syscall.SIGPIPE)
	command.Run(os.Args[1:], grepplecli.Run)
}
