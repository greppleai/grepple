package config

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// AskSettings stores user-owned Ask and Init defaults in settings.json.
type AskSettings struct {
	Model string  `json:"model,omitempty"`
	Logs  AskLogs `json:"logs,omitempty"`
}

// AskLogs stores logging overrides; absent Enabled defaults to true.
type AskLogs struct {
	Enabled         *bool  `json:"enabled,omitempty"`
	RetentionPeriod string `json:"retentionPeriod,omitempty"`
}

// AskPreferences are shared model and logging defaults consumed by Ask and Init.
type AskPreferences struct {
	Model        string
	LogsEnabled  bool
	LogRetention time.Duration
}

// resolveAskPreferences validates settings and applies Ask defaults.
func resolveAskPreferences(preferences AskSettings) (AskPreferences, error) {
	configured := AskPreferences{Model: strings.TrimSpace(preferences.Model), LogsEnabled: true, LogRetention: 7 * 24 * time.Hour}
	if preferences.Logs.Enabled != nil {
		configured.LogsEnabled = *preferences.Logs.Enabled
	}
	if retention := strings.TrimSpace(preferences.Logs.RetentionPeriod); retention != "" {
		var err error
		configured.LogRetention, err = parseLogRetention(retention)
		if err != nil {
			return configured, fmt.Errorf("ask.logs.retentionPeriod: %w", err)
		}
	}
	return configured, nil
}

// parseLogRetention parses ask.logs.retentionPeriod.
func parseLogRetention(value string) (time.Duration, error) {
	value = strings.TrimSpace(strings.ToLower(value))
	if strings.HasSuffix(value, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(value, "d"))
		maximumDays := int64((1<<63 - 1) / int64(24*time.Hour))
		if err != nil || days < 1 || int64(days) > maximumDays {
			return 0, fmt.Errorf("must be a positive duration such as 7d or 168h")
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		return 0, fmt.Errorf("must be a positive duration such as 7d or 168h")
	}
	return duration, nil
}
