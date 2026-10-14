package config

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTelemetrySettingsRemainUserOwnedAndStrict(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	t.Setenv("GREPPLE_SETTINGS", path)
	for _, test := range []struct {
		body  string
		valid bool
	}{
		{`{}`, true},
		{`{"telemetry":{"enabled":true,"trace_endpoint":"http://127.0.0.1:4318/v1/traces","trace_sample_ratio":0}}`, true},
		{`{"telemetry":{"enabled":true,"trace_endpoint":"http://[::1]:4318/v1/traces"}}`, true},
		{`{"telemetry":{"enabled":true,"trace_endpoint":"https://collector.example/v1/traces"}}`, true},
		{`{"telemetry":{"enabled":true}}`, false},
		{`{"telemetry":{"enabled":true,"trace_endpoint":"http://collector.example/v1/traces"}}`, false},
		{`{"telemetry":{"enabled":true,"trace_endpoint":"http://user:secret@localhost:4318/v1/traces"}}`, false},
		{`{"telemetry":{"enabled":true,"trace_endpoint":"http://localhost:4318/v1/traces?secret=1"}}`, false},
		{`{"telemetry":{"enabled":true,"trace_endpoint":"http://localhost:4318/v1/traces#secret"}}`, false},
		{`{"telemetry":{"enabled":true,"trace_endpoint":"http://localhost:4318/v1/logs"}}`, false},
		{`{"telemetry":{"enabled":true,"trace_endpoint":"http://localhost:4318/v1/traces","trace_sample_ratio":2}}`, false},
		{`{"telemetry":{"enabled":true,"trace_endpoint":"http://localhost:4318/v1/traces","headers":{"token":"secret"}}}`, false},
	} {
		if err := os.WriteFile(path, []byte(test.body), 0o600); err != nil {
			t.Fatal(err)
		}
		settings, err := LoadConfig("", true)
		if (err == nil) != test.valid {
			t.Fatalf("settings valid=%v for %s: %v", test.valid, test.body, err)
		}
		if test.valid && settings.User.Telemetry != nil && settings.User.Telemetry.SampleRatio() != 0 && settings.User.Telemetry.SampleRatio() != 1 {
			t.Fatal("incorrect default sample ratio")
		}
	}
	if err := (&Telemetry{Enabled: true, TraceEndpoint: "https://collector.example/v1/traces", TraceSampleRatio: ratioPtr(math.NaN())}).Validate(); err == nil {
		t.Fatal("NaN ratio accepted")
	}
	if err := (&Telemetry{Enabled: true, TraceEndpoint: "https://collector.example/v1/traces", TraceSampleRatio: ratioPtr(math.Inf(1))}).Validate(); err == nil {
		t.Fatal("infinite ratio accepted")
	}
	if err := os.WriteFile(path, []byte(`{"telemetry":{"enabled":true,"trace_endpoint":"http://localhost:4318/v1/traces"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	settings, err := LoadConfig("", true)
	if err != nil || settings.User.Telemetry == nil || settings.User.Telemetry.SampleRatio() != 1 {
		t.Fatalf("--no-repo-config lost user telemetry: %v", err)
	}
	if strings.Contains(settings.String(), "4318") {
		t.Fatal("config string exposed endpoint")
	}
}

func ratioPtr(value float64) *float64 { return &value }
