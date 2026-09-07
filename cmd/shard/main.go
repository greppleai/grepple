// Package main runs the grepple shard: the per-shard binary that keeps a
// checkout base fresh, serves search over it, and (when configured) maintains
// a zoekt trigram index.
package main

import (
	"os"

	"grepple/internal/command"
	"grepple/internal/shard"
)

func main() {
	command.Run(os.Args[1:], shard.Run)
}
