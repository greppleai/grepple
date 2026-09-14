// Package main is the process entry point for the Grepple CLI.
package main

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/greppleai/grepple/internal/cli"
	"github.com/greppleai/grepple/parser"
)

func main() {
	// CLI workflows share content-addressed parser navigation facts. An explicitly
	// empty environment variable disables the cache for library-like operation.
	if _, configured := os.LookupEnv(parser.NavigationCacheDirectoryEnv); !configured {
		_ = os.Setenv(parser.NavigationCacheDirectoryEnv, filepath.Join(".grepple", "cache", "navigation"))
	}

	// Convert a downstream pipe closing into EPIPE so the CLI can return cleanly.
	signal.Ignore(syscall.SIGPIPE)
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
