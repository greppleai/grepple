package verify

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/directorymeta"
	"github.com/greppleai/grepple/internal/filedigest"
	sourcedomain "github.com/greppleai/grepple/internal/sources"
)

func TestVerifyV1ProductionPolicyAndExplicitFileBypass(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	for _, name := range []string{"prod.go", "test.go", "ignored.go"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("package demo\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("ignored.go\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	checksum, err := filedigest.SHA256Hex(filepath.Join(root, "prod.go"))
	if err != nil {
		t.Fatal(err)
	}
	if err := directorymeta.WriteDirectoryV1(root, root, directorymeta.DirectoryV1{Files: []directorymeta.FileV1{{Name: "prod.go", Checksum: checksum, Description: "Production source.", Purpose: "Implementation"}}}); err != nil {
		t.Fatal(err)
	}
	policy := sourcedomain.Options{WorkingDirectory: root, IgnoreRoot: root, ProductionOnly: true}
	application := cliruntime.Environment{Output: &bytes.Buffer{}, RepositoryContext: cliruntime.RepositoryServices{WorkingDirectoryFunc: func() string { return root }, ScopeOptionsFunc: func() (sourcedomain.Options, error) { return policy, nil }}}
	report, err := Build(application, nil)
	if err != nil || report.Valid != 1 || len(report.Stale) != 0 || len(report.Missing) != 0 {
		t.Fatalf("production-only=%+v err=%v", report, err)
	}
	report, err = Build(application, []string{"test.go"})
	if err != nil || len(report.Stale) != 1 || report.Stale[0] != "." || len(report.Issues["."]) == 0 {
		t.Fatalf("explicit test not verified=%+v err=%v", report, err)
	}
}
