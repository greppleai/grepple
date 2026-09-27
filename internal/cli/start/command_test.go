package start

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/directorymeta"
)

func TestStartAreaUsesOnlyCurrentMetadataAndSeparatesTests(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	if err := os.Mkdir(filepath.Join(root, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"source.go", "source_test.go", "stale.go"} {
		if err := os.WriteFile(filepath.Join(root, "pkg", name), []byte("package pkg\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	files, err := directorymeta.FilesForDirectory(root, filepath.Join(root, "pkg"), []string{"pkg/source.go", "pkg/source_test.go", "pkg/stale.go"})
	if err != nil {
		t.Fatal(err)
	}
	for i := range files {
		files[i].Description, files[i].Areas = "Outline source.", []string{"outline"}
		files[i].Kind = "production"
		if strings.HasSuffix(files[i].Path, "_test.go") {
			files[i].Kind = "test"
		}
	}
	if err := directorymeta.Write(filepath.Join(root, "pkg"), directorymeta.Metadata{Description: "Package.", Responsibilities: []string{"Own source."}, Files: files}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "pkg", "stale.go"), []byte("package pkg\n// newer\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	application := cliruntime.NewContext(cliruntime.ContextOptions{Output: &output, ErrorOutput: &bytes.Buffer{}})
	report, err := Build(application, &Args{Area: "outline"})
	if err != nil || len(report.References) != 2 || len(report.Review) != 1 || report.Review[0].Path != "pkg/stale.go" {
		t.Fatalf("report=%+v err=%v", report, err)
	}
	if err := Execute(application, &Args{Area: "outline"}); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"source pkg/source.go", "test pkg/source_test.go", "review pkg/stale.go"} {
		if !strings.Contains(output.String(), value) {
			t.Fatalf("missing %q in %q", value, output.String())
		}
	}
	if _, err := Build(application, &Args{Area: "../bad"}); err == nil {
		t.Fatal("invalid area accepted")
	}
}
