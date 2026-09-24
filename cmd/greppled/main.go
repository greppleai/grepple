// Package main starts the optional foreground local architecture worker.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/greppleai/grepple/internal/archdaemon"
	"github.com/greppleai/grepple/internal/storagepaths"
	"github.com/greppleai/grepple/parser"
)

func main() {
	if len(os.Args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: greppled (run from the repository directory; Ctrl-C to stop)")
		os.Exit(2)
	}
	if _, configured := os.LookupEnv(parser.NavigationCacheDirectoryEnv); !configured {
		if cwd, err := os.Getwd(); err == nil {
			_ = os.Setenv(parser.NavigationCacheDirectoryEnv, filepath.Join(storagepaths.Cache(cwd), "navigation"))
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := archdaemon.Serve(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}
