package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sourcescommand "github.com/greppleai/grepple/internal/cli/sources"
	"github.com/greppleai/grepple/internal/directorymeta"
	"github.com/greppleai/grepple/internal/filedigest"
)

func TestRepositoryScopeFlagsBypassOnlyRequestedBehavior(t *testing.T) {
	root := t.TempDir()
	writeGraphSource(t, root, "main.go", "package sample\n")
	writeGraphSource(t, root, "sandbox/ignored.go", "package sandbox\n")
	digest, err := filedigest.SHA256Hex(filepath.Join(root, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	if err := directorymeta.Write(root, directorymeta.Metadata{Description: "Scope fixture.", Responsibilities: []string{"Test invocation scope."}, Files: []directorymeta.File{{Path: "main.go", Description: "Production fixture.", Kind: "production", Checksum: digest}}}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "grepple.json"), []byte(`{"ignore":{"paths":["sandbox/**"]}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	withoutIgnore := captureStdout(t, func() {
		if err := Run([]string{"sources", "explain", "--json", "--no-config-ignore", "--no-spill"}); err != nil {
			t.Fatal(err)
		}
	})
	var report sourcescommand.Report
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
