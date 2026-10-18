package initcommand

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/greppleai/grepple/internal/directorymeta"
)

func generatedV1(job generationJobV1) directorymeta.DirectoryV1 {
	files := append([]directorymeta.FileV1(nil), job.files...)
	if len(files) == 0 {
		files = []directorymeta.FileV1{}
	}
	for i := range files {
		files[i].Description = "Source-grounded annotation."
		files[i].Purpose = "Implementation"
		files[i].DomainAreas = []string{"Checkout"}
	}
	return directorymeta.DirectoryV1{Files: files}
}

func TestInitV1RefreshAndSkipWithoutModel(t *testing.T) {
	root, application, output := initTestContext(t)
	writeInitSource(t, root, "a/one.go", "package a\n")
	writeInitSource(t, root, "b/two.go", "package b\n")
	ctx := context.Background()
	plan, err := planGenerationV1(ctx, application, nil, false, "")
	if err != nil || len(plan) != 3 || len(plan[0].files) != 0 || plan[0].skip {
		t.Fatalf("plan=%+v: %v", plan, err)
	}
	var calls atomic.Int32
	generate := func(_ context.Context, job generationJobV1) (directorymeta.DirectoryV1, error) {
		calls.Add(1)
		return generatedV1(job), nil
	}
	if err := runGenerationV1(ctx, application, root, plan, 2, generate); err != nil || calls.Load() != 2 {
		t.Fatalf("generate calls=%d: %v", calls.Load(), err)
	}
	if got := output.String(); got != "write .grepple/grepple.yaml (.)\nwrite .grepple/grepple.yaml (a)\nwrite .grepple/grepple.yaml (b)\n" {
		t.Fatalf("ordered output=%q", got)
	}
	document, err := directorymeta.ReadDocumentV1(root)
	if err != nil || len(document.Directory) != 3 || document.Directory["a"].Files[0].Name != "one.go" {
		t.Fatalf("snapshot=%+v: %v", document, err)
	}
	output.Reset()
	plan, err = planGenerationV1(ctx, application, nil, false, "")
	if err != nil || plan.needsGeneration() {
		t.Fatalf("fresh plan=%+v: %v", plan, err)
	}
	if err := Execute(application, &Args{Concurrency: 1}); err != nil || output.String() != "skip .\nskip a\nskip b\n" {
		t.Fatalf("no-model Execute=%q: %v", output.String(), err)
	}
	writeInitSource(t, root, "a/one.go", "package a\n// changed\n")
	plan, err = planGenerationV1(ctx, application, nil, false, "a")
	if err != nil || len(plan) != 1 || plan[0].skip || len(plan[0].files) != 1 {
		t.Fatalf("only a: %+v: %v", plan, err)
	}
	if err := runGenerationV1(ctx, application, root, plan, 1, generate); err != nil {
		t.Fatal(err)
	}
	document, err = directorymeta.ReadDocumentV1(root)
	if err != nil || len(document.Directory) != 3 || document.Directory["b"].Files[0].Name != "two.go" {
		t.Fatalf("sibling lost: %+v: %v", document, err)
	}
	plan, err = planGenerationV1(ctx, application, nil, true, "")
	if err != nil || !plan.needsGeneration() || plan[1].skip {
		t.Fatalf("forced plan=%+v: %v", plan, err)
	}
	if _, err := planGenerationV1(ctx, application, []string{"a/one.go"}, false, "a"); err == nil {
		t.Fatal("combined scope accepted")
	}
}

func TestInitV1RefusesChangedSourceAndOldSchema(t *testing.T) {
	root, application, _ := initTestContext(t)
	writeInitSource(t, root, "a/one.go", "package a\n")
	ctx := context.Background()
	plan, err := planGenerationV1(ctx, application, nil, false, "a")
	if err != nil {
		t.Fatal(err)
	}
	writeInitSource(t, root, "a/one.go", "package a\n// changed while model ran\n")
	err = runGenerationV1(ctx, application, root, plan, 1, func(_ context.Context, job generationJobV1) (directorymeta.DirectoryV1, error) {
		return generatedV1(job), nil
	})
	if err == nil || !strings.Contains(err.Error(), "selected source changed") {
		t.Fatalf("accepted drift: %v", err)
	}
	if _, err := os.Stat(directorymeta.RepositoryPath(root)); !os.IsNotExist(err) {
		t.Fatalf("wrote stale metadata: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(directorymeta.RepositoryPath(root)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(directorymeta.RepositoryPath(root), []byte("version: 1\ndirectories: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := planGenerationV1(ctx, application, nil, true, ""); err == nil {
		t.Fatal("force accepted old schema")
	}
}

func TestInitV1RechecksSelectedMembershipAfterGeneration(t *testing.T) {
	for _, tc := range []struct {
		name      string
		mutate    func(*testing.T, string)
		wantError bool
	}{
		{"new selected file", func(t *testing.T, root string) { writeInitSource(t, root, "a/new.go", "package a\n") }, true},
		{"removed selected file", func(t *testing.T, root string) {
			if err := os.Remove(filepath.Join(root, "a", "one.go")); err != nil {
				t.Fatal(err)
			}
		}, true},
		{"new ignored file", func(t *testing.T, root string) { writeInitSource(t, root, "a/ignored.go", "package a\n") }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, application, _ := initTestContext(t)
			writeInitSource(t, root, "a/one.go", "package a\n")
			writeInitSource(t, root, ".gitignore", "a/ignored.go\n")
			plan, err := planGenerationV1(context.Background(), application, nil, false, "a")
			if err != nil {
				t.Fatal(err)
			}
			err = runGenerationV1(context.Background(), application, root, plan, 1, func(_ context.Context, job generationJobV1) (directorymeta.DirectoryV1, error) {
				tc.mutate(t, root)
				return generatedV1(job), nil
			})
			if (err != nil) != tc.wantError {
				t.Fatalf("selection change accepted=%v: %v", !tc.wantError, err)
			}
			_, readErr := directorymeta.ReadDocumentV1(root)
			if tc.wantError && !os.IsNotExist(readErr) {
				t.Fatalf("wrote metadata for changed membership: %v", readErr)
			}
			if !tc.wantError && readErr != nil {
				t.Fatalf("ignored file blocked generation: %v", readErr)
			}
		})
	}
}
