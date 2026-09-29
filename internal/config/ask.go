package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type askFile struct {
	Ask struct {
		Model string `json:"model,omitempty"`
		Logs  struct {
			Enabled         *bool  `json:"enabled,omitempty"`
			RetentionPeriod string `json:"retentionPeriod,omitempty"`
		} `json:"logs,omitempty"`
	} `json:"ask,omitempty"`
}

// AskPreferences are shared model and logging defaults consumed by Ask and Init.
type AskPreferences struct {
	Model        string
	LogsEnabled  bool
	LogRetention time.Duration
}

// loadAskPreferences reads legacy ask defaults from ~/.grepple/grepple.json.
func loadAskPreferences() (AskPreferences, error) {
	configured := AskPreferences{LogsEnabled: true, LogRetention: 7 * 24 * time.Hour}
	home, err := os.UserHomeDir()
	if err != nil {
		return configured, err
	}
	path := filepath.Join(home, ".grepple", "grepple.json")
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return configured, nil
	}
	if err != nil {
		return configured, fmt.Errorf("read user configuration %s: %w", path, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	var preferences askFile
	if err := decoder.Decode(&preferences); err != nil {
		return configured, fmt.Errorf("invalid user configuration %s: %w", path, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return configured, fmt.Errorf("invalid user configuration %s: trailing JSON content", path)
	}
	configured.Model = strings.TrimSpace(preferences.Ask.Model)
	if preferences.Ask.Logs.Enabled != nil {
		configured.LogsEnabled = *preferences.Ask.Logs.Enabled
	}
	if retention := strings.TrimSpace(preferences.Ask.Logs.RetentionPeriod); retention != "" {
		configured.LogRetention, err = parseLogRetention(retention)
		if err != nil {
			return configured, fmt.Errorf("invalid user configuration %s ask.logs.retentionPeriod: %w", path, err)
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
