package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSourcesExplainReportsConfigExclusionsAndClassifications(t *testing.T) {
	root := t.TempDir()
	writeGraphSource(t, root, "main.go", "package sample\nfunc Run() {}\n")
	writeGraphSource(t, root, "main_test.go", "package sample\nfunc TestRun() {}\n")
	writeGraphSource(t, root, "sandbox/ignored.go", "package sandbox\n")
	if err := os.WriteFile(filepath.Join(root, "grepple.json"), []byte(`{"ignore":{"paths":["sandbox/**"]}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	chdirForConfigTest(t, root)

	output := captureStdout(t, func() {
		if err := Run([]string{"sources", "explain", "--json", "--production-only", "--no-spill"}); err != nil {
			t.Fatal(err)
		}
	})
	var report sourceScopeReport
	if err := json.Unmarshal([]byte(output), &report); err != nil {
		t.Fatal(err)
	}
	if !report.Config.Loaded || report.Config.Path != "grepple.json" || !strings.HasPrefix(report.Config.Digest, "sha256:") {
		t.Fatalf("config=%+v", report.Config)
	}
	if !report.ProductionOnly || report.SelectedFiles != 2 || report.ExcludedFiles != 2 {
		t.Fatalf("report=%+v", report)
	}
	if formatSourceScopeCounts(report.Exclusions) != "config-ignore:1,non-production:1" {
		t.Fatalf("exclusions=%+v", report.Exclusions)
	}
	spooled := captureStdout(t, func() {
		if err := Run([]string{"sources", "explain", "--json", "--production-only", "--spill-threshold-bytes", "1000000"}); err != nil {
			t.Fatal(err)
		}
	})
	if output != spooled {
		t.Fatalf("source report changed with output spool state\nno-spill=%s\nspooled=%s", output, spooled)
	}
}

func TestRepositoryScopeFlagsBypassOnlyRequestedBehavior(t *testing.T) {
	root := t.TempDir()
	writeGraphSource(t, root, "main.go", "package sample\n")
	writeGraphSource(t, root, "sandbox/ignored.go", "package sandbox\n")
	if err := os.WriteFile(filepath.Join(root, "grepple.json"), []byte(`{"ignore":{"paths":["sandbox/**"]}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	chdirForConfigTest(t, root)

	withoutIgnore := captureStdout(t, func() {
		if err := Run([]string{"sources", "explain", "--json", "--no-config-ignore", "--no-spill"}); err != nil {
			t.Fatal(err)
		}
	})
	var report sourceScopeReport
	if err := json.Unmarshal([]byte(withoutIgnore), &report); err != nil {
		t.Fatal(err)
	}
	if !report.Config.Loaded || report.Config.IgnoreEnabled || report.ExcludedFiles != 0 {
		t.Fatalf("no-config-ignore report=%+v", report)
	}

	withoutConfig := captureStdout(t, func() {
		if err := Run([]string{"sources", "explain", "--json", "--no-repo-config", "--no-spill", root}); err != nil {
			t.Fatal(err)
		}
	})
	if err := json.Unmarshal([]byte(withoutConfig), &report); err != nil {
		t.Fatal(err)
	}
	if report.Config.Loaded {
		t.Fatalf("no-repo-config config=%+v", report.Config)
	}

	bypass := captureStderr(t, func() {
		_ = Run([]string{"--files", filepath.Join(root, "sandbox", "ignored.go"), "--production-only", "--no-spill"})
	})
	if !strings.Contains(bypass, "explicitly named file bypasses grepple.json ignore.paths") {
		t.Fatalf("bypass stderr=%q", bypass)
	}

	production := captureStdout(t, func() {
		if err := Run([]string{"--files", "--production-only", "--limit", "0", ".", "--no-spill"}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(production, "main.go") || strings.Contains(production, "main_test.go") || strings.Contains(production, "sandbox/ignored.go") {
		t.Fatalf("production paths=%q", production)
	}
}
