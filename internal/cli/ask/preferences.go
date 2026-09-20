package ask

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

type userPreferences struct {
	Ask struct {
		Model string `json:"model,omitempty"`
		Logs  struct {
			Enabled         *bool  `json:"enabled,omitempty"`
			RetentionPeriod string `json:"retentionPeriod,omitempty"`
		} `json:"logs,omitempty"`
	} `json:"ask,omitempty"`
}

// Preferences are the configured defaults for the ask command.
type Preferences struct {
	Model        string
	LogsEnabled  bool
	LogRetention time.Duration
}

// LoadPreferences reads ask defaults from the user Grepple configuration.
func LoadPreferences() (Preferences, error) {
	configured := Preferences{LogsEnabled: true, LogRetention: 7 * 24 * time.Hour}
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
	var preferences userPreferences
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
		configured.LogRetention, err = ParseLogRetention(retention)
		if err != nil {
			return configured, fmt.Errorf("invalid user configuration %s ask.logs.retentionPeriod: %w", path, err)
		}
	}
	return configured, nil
}

// ParseLogRetention parses the duration accepted by ask.logs.retentionPeriod.
func ParseLogRetention(value string) (time.Duration, error) {
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
