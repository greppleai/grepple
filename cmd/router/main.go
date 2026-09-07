// Package main runs the grepple router: the control plane that discovers,
// assigns, and rebalances shards, ingests GitHub webhooks, authenticates the
// CLI, and fans searches out to the shard backends.
package main

import (
	"os"

	"grepple/internal/command"
	"grepple/internal/router"
)

func main() {
	command.Run(os.Args[1:], router.Run)
}
