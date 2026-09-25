// Package main starts the optional foreground local architecture worker.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/greppleai/grepple/internal/archdaemon"
)

func main() {
	if len(os.Args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: greppled (one foreground user-level cache; Ctrl-C to stop)")
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := archdaemon.Serve(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}
