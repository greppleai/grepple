package initcommand

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/directorymeta"
)

func TestInitPrintsAreaProposalsWithoutPersistingThem(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "file.go"), []byte("package demo\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	files, err := directorymeta.FilesForDirectory(root, root, []string{"file.go"})
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	application := cliruntime.NewContext(cliruntime.ContextOptions{Output: &output, ErrorOutput: &bytes.Buffer{}})
	plan := generationPlan{{directory: root, files: files}}
	generate := func(_ context.Context, job generationJob) (directorymeta.Metadata, error) {
		metadata := generatedInitMetadata(job)
		metadata.Files[0].Areas = []string{"outline"}
		metadata.AreaProposals = []directorymeta.AreaProposal{{Path: "file.go", Area: "outline", Action: "add", Evidence: "file.go:1 parser"}}
		return metadata, nil
	}
	if err := runGeneration(context.Background(), application, root, plan, 1, generate); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "area-proposal add outline ./file.go: file.go:1 parser") {
		t.Fatalf("proposal output=%q", output.String())
	}
	content, err := os.ReadFile(filepath.Join(root, directorymeta.FileName))
	if err != nil || !strings.Contains(string(content), "areas:") || strings.Contains(string(content), "area_proposals") {
		t.Fatalf("persisted=%s err=%v", content, err)
	}
}
