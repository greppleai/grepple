// Package main starts the optional foreground local CLI daemon.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/greppleai/grepple/internal/daemon"
)

func main() {
	if len(os.Args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: greppled (one foreground user-level daemon; Ctrl-C to stop)")
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := daemon.Serve(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}
