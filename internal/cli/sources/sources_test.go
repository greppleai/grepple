package sources

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/directorymeta"
	"github.com/greppleai/grepple/internal/filedigest"
)

func TestSourcesExplainReportsConfigExclusionsAndClassifications(t *testing.T) {
	root := t.TempDir()
	writeSourceFixture(t, root, "main.go", "package sample\nfunc Run() {}\n")
	writeSourceFixture(t, root, "main_test.go", "package sample\nfunc TestRun() {}\n")
	writeSourceFixture(t, root, "sandbox/ignored.go", "package sandbox\n")
	configPath := filepath.Join(root, "grepple.json")
	if err := os.WriteFile(configPath, []byte(`{"ignore":{"paths":["sandbox/**"]}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	writeSourceKinds(t, root, map[string]string{"main.go": "production", "main_test.go": "test", "grepple.json": "production"})
	dependencies := Dependencies{Environment: func() (Environment, error) {
		return Environment{Root: root, ConfigPath: configPath, IgnorePaths: []string{"sandbox/**"}, ProductionOnly: true}, nil
	}, WorkingDirectory: func() string { return root }}
	var output bytes.Buffer
	dependencies.Stdout = &output
	if err := run([]string{"explain", "--json"}, dependencies); err != nil {
		t.Fatal(err)
	}
	var report Report
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if !report.Config.Loaded || report.Config.Path != "grepple.json" || !strings.HasPrefix(report.Config.Digest, "sha256:") {
		t.Fatalf("config=%+v", report.Config)
	}
	if !report.ProductionOnly || report.SelectedFiles != 2 || report.ExcludedFiles != 2 {
		t.Fatalf("report=%+v", report)
	}
	if FormatCounts(report.Exclusions) != "config-ignore:1,non-production:1" {
		t.Fatalf("exclusions=%+v", report.Exclusions)
	}
}
func writeSourceKinds(t *testing.T, root string, kinds map[string]string) {
	t.Helper()
	files := make([]directorymeta.File, 0, len(kinds))
	for name, kind := range kinds {
		digest, err := filedigest.SHA256Hex(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, directorymeta.File{Path: name, Description: "Source fixture.", Kind: kind, Checksum: digest})
	}
	if err := directorymeta.Write(root, directorymeta.Metadata{Description: "Source fixtures.", Responsibilities: []string{"Test source reporting."}, Files: files}); err != nil {
		t.Fatal(err)
	}
}

func writeSourceFixture(t *testing.T, root, relative, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
