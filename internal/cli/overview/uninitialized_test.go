package overview

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/directorymeta"
	"github.com/greppleai/grepple/internal/sources"
)

func TestOverviewUninitializedCheckoutSkipsSourcePolicyAndCounts(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0700); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(root, "pkg")
	if err := os.Mkdir(child, 0700); err != nil {
		t.Fatal(err)
	}
	for _, working := range []string{root, child} {
		application := cliruntime.Environment{RepositoryContext: cliruntime.RepositoryServices{
			WorkingDirectoryFunc: func() string { return working },
			ScopeOptionsFunc: func() (sources.Options, error) {
				t.Fatal("metadata absence must be checked before source discovery/policy")
				return sources.Options{}, nil
			},
		}}
		report, err := Build(application)
		if err != nil || report.MetadataStatus != directorymeta.StatusMissing || report.CountsComputed || report.Files.Selected != 0 {
			t.Fatalf("report=%+v err=%v", report, err)
		}
		if text := report.Text(); !strings.Contains(text, "metadata missing") || !strings.Contains(text, "counts not computed") || strings.Contains(text, "0/0") {
			t.Fatalf("misleading coverage: %s", text)
		}
		encoded, err := json.Marshal(report)
		if err != nil || !strings.Contains(string(encoded), `"countsComputed":false`) || !strings.Contains(string(encoded), `"metadataStatus":"missing"`) {
			t.Fatalf("JSON=%s err=%v", encoded, err)
		}
	}
}
