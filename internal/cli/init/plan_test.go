package initcommand

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/directorymeta"
)

func initTestContext(t *testing.T) (string, cliruntime.Context, *bytes.Buffer) {
	t.Helper()
	root := t.TempDir()
	t.Chdir(root)
	var output bytes.Buffer
	return root, cliruntime.NewContext(cliruntime.ContextOptions{Output: &output, ErrorOutput: &bytes.Buffer{}}), &output
}

func writeInitSource(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func generatedInitMetadata(job generationJob) directorymeta.Metadata {
	files := append([]directorymeta.File(nil), job.files...)
	for i := range files {
		files[i].Description = "Source file."
		files[i].Kind = "production"
	}
	return directorymeta.Metadata{Description: "Source directory.", Responsibilities: []string{"Own source files."}, Files: files}
}

func TestInitRefreshesOnlyMissingStaleAndInvalidDirectories(t *testing.T) {
	root, application, output := initTestContext(t)
	writeInitSource(t, root, "a/one.go", "package a\n")
	writeInitSource(t, root, "b/two.go", "package b\n")
	ctx := context.Background()
	plan, err := planGeneration(ctx, application, nil, false, "")
	if err != nil || len(plan) != 3 {
		t.Fatalf("initial plan=%+v err=%v", plan, err)
	}
	var calls int
	generate := func(_ context.Context, job generationJob) (directorymeta.Metadata, error) {
		calls++
		return generatedInitMetadata(job), nil
	}
	if err := runGeneration(ctx, application, root, plan, 1, generate); err != nil || calls != 3 {
		t.Fatalf("initial generation: calls=%d err=%v", calls, err)
	}
	output.Reset()
	plan, err = planGeneration(ctx, application, nil, false, "")
	if err != nil || plan.needsGeneration() {
		t.Fatalf("fresh plan=%+v err=%v", plan, err)
	}
	if err := runGeneration(ctx, application, root, plan, 1, nil); err != nil || !strings.Contains(output.String(), "skip a") {
		t.Fatalf("no-op output=%q err=%v", output.String(), err)
	}
	output.Reset()
	if err := Execute(application, &Args{Concurrency: 1}); err != nil || !strings.Contains(output.String(), "skip a") {
		t.Fatalf("no-op Execute output=%q err=%v", output.String(), err)
	}
	writeInitSource(t, root, "a/one.go", "package a\n// changed\n")
	if err := os.Remove(filepath.Join(root, "b", directorymeta.FileName)); err != nil {
		t.Fatal(err)
	}
	plan, err = planGeneration(ctx, application, nil, false, "")
	if err != nil || !plan[0].skip || plan[1].skip || plan[2].skip {
		t.Fatalf("refresh plan=%+v err=%v", plan, err)
	}
	output.Reset()
	calls = 0
	if err := runGeneration(ctx, application, root, plan, 1, generate); err != nil || calls != 2 {
		t.Fatalf("refresh calls=%d err=%v", calls, err)
	}
	if got := output.String(); got != "skip .\nwrite a/grepple.yaml\nwrite b/grepple.yaml\n" {
		t.Fatalf("refresh output=%q", got)
	}
	if err := os.WriteFile(filepath.Join(root, "a", directorymeta.FileName), []byte("invalid: ["), 0o600); err != nil {
		t.Fatal(err)
	}
	plan, err = planGeneration(ctx, application, nil, false, "a")
	if err != nil || len(plan) != 1 || plan[0].skip || plan[0].directory != filepath.Join(root, "a") {
		t.Fatalf("invalid metadata plan=%+v err=%v", plan, err)
	}
	plan, err = planGeneration(ctx, application, nil, true, "")
	if err != nil || len(plan) != 3 || !plan.needsGeneration() {
		t.Fatalf("force plan=%+v err=%v", plan, err)
	}
	for _, job := range plan {
		if job.skip {
			t.Fatalf("force skipped %s", job.directory)
		}
	}
}

func TestInitConcurrencyIsBoundedAndOutputOrdered(t *testing.T) {
	root, application, output := initTestContext(t)
	plan := make(generationPlan, 4)
	for index, name := range []string{"a", "b", "c", "d"} {
		directory := filepath.Join(root, name)
		if err := os.Mkdir(directory, 0o755); err != nil {
			t.Fatal(err)
		}
		plan[index] = generationJob{directory: directory}
	}
	var active, maxActive atomic.Int32
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	generate := func(_ context.Context, job generationJob) (directorymeta.Metadata, error) {
		current := active.Add(1)
		for previous := maxActive.Load(); current > previous && !maxActive.CompareAndSwap(previous, current); previous = maxActive.Load() {
		}
		if current <= 2 {
			started <- struct{}{}
			<-release
		}
		active.Add(-1)
		if filepath.Base(job.directory) == "c" {
			return directorymeta.Metadata{}, errors.New("model failed")
		}
		return generatedInitMetadata(job), nil
	}
	finished := make(chan error, 1)
	go func() { finished <- runGeneration(context.Background(), application, root, plan, 2, generate) }()
	for range 2 {
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			close(release)
			t.Fatal("workers did not run concurrently")
		}
	}
	close(release)
	select {
	case err := <-finished:
		if err == nil || !strings.Contains(err.Error(), "generate c: model failed") {
			t.Fatalf("generation error=%v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("workers did not finish")
	}
	if got := maxActive.Load(); got != 2 {
		t.Fatalf("maximum concurrency=%d, want 2", got)
	}
	if got := output.String(); got != "write a/grepple.yaml\nwrite b/grepple.yaml\nwrite d/grepple.yaml\n" {
		t.Fatalf("output=%q", got)
	}
	if _, err := os.Stat(filepath.Join(root, "c", directorymeta.FileName)); !os.IsNotExist(err) {
		t.Fatalf("failed directory unexpectedly written: %v", err)
	}
}

func TestInitRejectsZeroConcurrencyBeforeModelSelection(t *testing.T) {
	_, application, _ := initTestContext(t)
	if err := Execute(application, &Args{Concurrency: 0}); err == nil || !strings.Contains(err.Error(), "--concurrency") {
		t.Fatalf("invalid concurrency error=%v", err)
	}
}
