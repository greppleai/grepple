package verify

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/directorymeta"
)

func TestVerifyAreasReportsStaleAndInvalidMembershipLeads(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	path := filepath.Join(root, "file.go")
	if err := os.WriteFile(path, []byte("package demo\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	files, err := directorymeta.FilesForDirectory(root, root, []string{"file.go"})
	if err != nil {
		t.Fatal(err)
	}
	files[0].Kind, files[0].Description, files[0].Areas = "production", "Source.", []string{"outline"}
	if err := directorymeta.Write(root, directorymeta.Metadata{Description: "Root.", Responsibilities: []string{"Own code."}, Files: files}); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	var code int
	application := cliruntime.NewContext(cliruntime.ContextOptions{Output: &output, ErrorOutput: &bytes.Buffer{}, Exit: func(value int) { code = value }})
	report, err := BuildWithAreas(application, nil, true)
	if err != nil || report.Valid != 1 || len(report.AreaIssues) != 0 {
		t.Fatalf("initial report=%+v err=%v", report, err)
	}
	if err := os.WriteFile(path, []byte("package demo\n// changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	report, err = BuildWithAreas(application, nil, true)
	if err != nil || len(report.AreaIssues) != 1 || report.AreaIssues[0].Status != directorymeta.StatusStale {
		t.Fatalf("stale report=%+v err=%v", report, err)
	}
	if err := Execute(application, &Args{Areas: true}); err != nil || code != 1 || !strings.Contains(output.String(), "A stale outline file.go") {
		t.Fatalf("output=%q code=%d err=%v", output.String(), code, err)
	}
	files[0].Areas = []string{"Bad"}
	if err := directorymeta.Write(root, directorymeta.Metadata{Description: "Root.", Responsibilities: []string{"Own code."}, Files: files}); err != nil {
		t.Fatal(err)
	}
	report, err = BuildWithAreas(application, nil, true)
	if err != nil || len(report.AreaIssues) != 1 || report.AreaIssues[0].Status != directorymeta.StatusInvalid {
		t.Fatalf("invalid report=%+v err=%v", report, err)
	}
}
