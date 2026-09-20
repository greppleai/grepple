package cli

import (
	"time"

	"github.com/greppleai/grepple/internal/agent"
)

type askLog = agent.Log

type askLogOptions struct {
	enabled   bool
	retention time.Duration
}

func newAskLog() (*askLog, error) { return agent.NewLog() }

func newAskLogWithOptions(options askLogOptions) (*askLog, error) {
	return agent.NewLogWithOptions(agent.LogOptions{Enabled: options.enabled, Retention: options.retention})
}

type askTelemetry = agent.Telemetry

const askLogDirectoryEnv = "GREPPLE_ASK_LOG_DIR"

type askLogEvent struct {
	Schema string `json:"schema"`
	Type   string `json:"type"`
}

type researchCacheStatus = agent.CacheStatus

func newAskTelemetry(started time.Time) *askTelemetry { return agent.NewTelemetry(started) }