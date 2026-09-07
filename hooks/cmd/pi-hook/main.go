// Package main runs the repository's Go-based pi hooks.
package main

import (
	"fmt"
	"io"
	"os"

	"grepple/hooks/internal/pihooks"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: pi-hook <grep-guard|lint-check>")
		os.Exit(2)
	}
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		return
	}
	var output []byte
	switch os.Args[1] {
	case "grep-guard":
		output = pihooks.HandleHook(input)
	case "lint-check":
		hookRoot, err := os.Getwd()
		if err != nil {
			return
		}
		output = pihooks.HandleLintHook(input, hookRoot)
	default:
		fmt.Fprintf(os.Stderr, "unknown pi hook %q\n", os.Args[1])
		os.Exit(2)
	}
	if len(output) > 0 {
		_, _ = os.Stdout.Write(output)
	}
}
