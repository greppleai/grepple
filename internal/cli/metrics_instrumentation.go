package cli

import (
	"time"

	agentmetrics "github.com/greppleai/grepple/internal/metrics"
)

var observableGreppleCommands = map[string]bool{
	"search": true, "graph": true, "anchors": true, "boundaries": true,
	"examples": true, "artifacts": true, "languages": true, "get": true,
	"tree": true, "repos": true, "refs": true, "ask": true,
	"ai-provider": true, "login": true, "logout": true, "rules": true,
	"grit": true, "extract": true, "architecture": true, "sources": true,
	"version": true, "help": true,
}

// recordGreppleCommand is deliberately best-effort: metrics collection must not
// change a command's output or exit behavior.
func recordGreppleCommand(args []string, started time.Time, success bool) {
	if len(args) > 0 && args[0] == "metrics" {
		return
	}
	directory, err := metricsDirectory()
	if err != nil {
		return
	}
	state, err := readMetricsActiveState(directory)
	if err != nil {
		return
	}
	name := observableCommandName(args)
	data := agentmetrics.CommandData{
		Name: name, Success: success, DurationMS: time.Since(started).Milliseconds(), GreppleMode: name,
	}
	_ = appendMetricsEvent(state.Journal, metricsEventOptions{runID: state.RunID}, agentmetrics.EventCommand, data)
}

func observableCommandName(args []string) string {
	if len(args) > 0 && observableGreppleCommands[args[0]] {
		return args[0]
	}
	return "search"
}
