package initcommand

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/directorymeta"
	"github.com/greppleai/grepple/internal/filedigest"
	sourcedomain "github.com/greppleai/grepple/internal/sources"
)

func TestInitV1PlanHonorsProductionIgnoreAndExplicitFilePolicy(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	var entries []directorymeta.FileV1
	for _, item := range []struct{ name, purpose string }{
		{"prod.go", "Implementation"}, {"test.go", "Testing"}, {"excluded.go", "Implementation"}, {"ignored.go", "Implementation"},
	} {
		writeInitSource(t, root, item.name, "package demo\n// "+item.name+"\n")
		digest, err := filedigest.SHA256Hex(filepath.Join(root, item.name))
		if err != nil {
			t.Fatal(err)
		}
		entries = append(entries, directorymeta.FileV1{Name: item.name, Checksum: digest, Description: "Reviewed source.", Purpose: item.purpose})
	}
	writeInitSource(t, root, ".gitignore", "ignored.go\n")
	if err := directorymeta.WriteDirectoryV1(root, root, directorymeta.DirectoryV1{Files: entries}); err != nil {
		t.Fatal(err)
	}
	var notices bytes.Buffer
	policy := sourcedomain.Options{WorkingDirectory: root, IgnoreRoot: root, IgnorePaths: []string{"excluded.go"}, ProductionOnly: true, Notices: &notices}
	application := cliruntime.Environment{Output: &bytes.Buffer{}, RepositoryContext: cliruntime.RepositoryServices{WorkingDirectoryFunc: func() string { return root }, ScopeOptionsFunc: func() (sourcedomain.Options, error) { return policy, nil }}}
	plan, err := planGenerationV1(context.Background(), application, nil, true, "")
	if err != nil || len(plan) != 1 || len(plan[0].files) != 1 || plan[0].files[0].Name != "prod.go" {
		t.Fatalf("production plan=%+v err=%v", plan, err)
	}
	if !plan.needsGeneration() {
		t.Fatal("--force skipped current metadata")
	}
	scoped, err := planGenerationV1(context.Background(), application, []string{"test.go"}, true, "")
	if err != nil || len(scoped) != 1 || len(scoped[0].files) != 1 || scoped[0].files[0].Name != "test.go" {
		t.Fatalf("explicit file not honored: %+v err=%v", scoped, err)
	}
	if notices.Len() == 0 {
		t.Fatal("explicit production-only bypass lacked notice")
	}
	generate := func(_ context.Context, job generationJobV1) (directorymeta.DirectoryV1, error) {
		entry := generatedV1(job)
		for i := range entry.Files {
			if entry.Files[i].Name == "test.go" {
				entry.Files[i].Purpose = "Testing"
			}
		}
		return entry, nil
	}
	if err := runGenerationV1(context.Background(), application, root, plan, 1, generate); err != nil {
		t.Fatal(err)
	}
	got, err := directorymeta.ReadDocumentV1(root)
	if err != nil || len(got.Directory["."].Files) != 4 {
		t.Fatalf("lost current excluded annotations: %+v err=%v", got, err)
	}
	beforeNotices := notices.String()
	if err := runGenerationV1(context.Background(), application, root, scoped, 1, generate); err != nil {
		t.Fatalf("explicit file recheck failed: %v", err)
	}
	if notices.String() != beforeNotices {
		t.Fatalf("worker duplicated explicit bypass notice: %q", notices.String())
	}
	got, err = directorymeta.ReadDocumentV1(root)
	if err != nil || len(got.Directory["."].Files) != 4 || got.Directory["."].Files[3].Name != "test.go" {
		t.Fatalf("explicit refresh discarded unselected annotations: %+v err=%v", got, err)
	}
	if err := os.Remove(filepath.Join(root, "ignored.go")); err != nil {
		t.Fatal(err)
	}
	writeInitSource(t, root, "excluded.go", "package demo\n// changed while excluded\n")
	plan, err = planGenerationV1(context.Background(), application, nil, true, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := runGenerationV1(context.Background(), application, root, plan, 1, generate); err != nil {
		t.Fatal(err)
	}
	got, err = directorymeta.ReadDocumentV1(root)
	if err != nil || len(got.Directory["."].Files) != 2 || got.Directory["."].Files[0].Name != "prod.go" || got.Directory["."].Files[1].Name != "test.go" {
		t.Fatalf("stale excluded annotations retained: %+v err=%v", got, err)
	}
}
