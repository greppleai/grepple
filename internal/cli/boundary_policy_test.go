package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBoundariesLoadsRepositoryPolicyAndReportsFacadeBypass(t *testing.T) {
	directory := chdirTemp(t)
	writeGraphSource(t, directory, "internal/engine/run.go", "package engine\nfunc Execute(){}\n")
	writeGraphSource(t, directory, "engine/engine.go", "package engine\nimport internal \"example/internal/engine\"\nfunc Run(){ internal.Execute() }\n")
	writeGraphSource(t, directory, "client/client.go", "package client\nimport internal \"example/internal/engine\"\nfunc Call(){ internal.Execute() }\n")
	policy := `{"schema":"grepple-boundary-policy-v1","facades":[{"name":"engine","facadePaths":["engine"],"implementationPaths":["internal/engine"]}]}`
	policyPath := filepath.Join(directory, defaultBoundaryPolicyPath)
	if err := os.MkdirAll(filepath.Dir(policyPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(policyPath, []byte(policy), 0o600); err != nil {
		t.Fatal(err)
	}
	cold := captureStdout(t, func() {
		if err := Run([]string{"boundaries", "."}); err != nil {
			t.Fatal(err)
		}
	})
	warm := captureStdout(t, func() {
		if err := Run([]string{"boundaries", "."}); err != nil {
			t.Fatal(err)
		}
	})
	if cold != warm || !strings.Contains(cold, "engine:") {
		t.Fatalf("policy cache parity failed:\ncold=%s\nwarm=%s", cold, warm)
	}
	output := captureStdout(t, func() {
		if err := Run([]string{"boundaries", "--json", "--no-cache", "."}); err != nil {
			t.Fatal(err)
		}
	})
	var report boundariesOutput
	if err := json.Unmarshal([]byte(output), &report); err != nil {
		t.Fatal(err)
	}
	if report.Policy != defaultBoundaryPolicyPath || len(report.FacadeBypasses) != 1 || report.FacadeBypasses[0].Caller.Name != "Call" {
		t.Fatalf("policy report=%#v", report)
	}
}

func TestBoundariesRejectsInvalidPolicyWithoutReadingSources(t *testing.T) {
	directory := chdirTemp(t)
	policyPath := filepath.Join(directory, "policy.json")
	if err := os.WriteFile(policyPath, []byte(`{"schema":"future"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	err := Run([]string{"boundaries", "--policy", policyPath, "."})
	if err == nil || !strings.Contains(err.Error(), "unsupported boundary policy schema") {
		t.Fatalf("err=%v", err)
	}
}
