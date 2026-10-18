package initcommand

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/greppleai/grepple/internal/directorymeta"
)

func TestInitV1ConcurrentWritesKeepSiblingsAndOrderOutput(t *testing.T) {
	for _, tc := range []struct {
		name      string
		failFirst bool
	}{{"both succeed", false}, {"first fails after second finishes", true}} {
		t.Run(tc.name, func(t *testing.T) {
			root, application, output := initTestContext(t)
			writeInitSource(t, root, "a/one.go", "package a\n")
			writeInitSource(t, root, "b/two.go", "package b\n")
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			plan, err := planGenerationV1(ctx, application, nil, false, "")
			if err != nil || len(plan) != 3 {
				t.Fatalf("plan=%+v err=%v", plan, err)
			}
			// Restrict this run to the two file-bearing directories, so a worker
			// writing an empty root entry cannot mask a sibling-write regression.
			doneB := make(chan struct{})
			err = runGenerationV1(ctx, application, root, plan[1:], 2, func(ctx context.Context, job generationJobV1) (directorymeta.DirectoryV1, error) {
				if job.directory == plan[2].directory {
					close(doneB)
					return generatedV1(job), nil
				}
				select {
				case <-doneB:
				case <-ctx.Done():
					return directorymeta.DirectoryV1{}, ctx.Err()
				}
				if tc.failFirst {
					return directorymeta.DirectoryV1{}, errors.New("first agent failed")
				}
				return generatedV1(job), nil
			})
			if tc.failFirst {
				if err == nil || !strings.Contains(err.Error(), "generate a: first agent failed") {
					t.Fatalf("missing failure=%v", err)
				}
				if output.String() != "write .grepple/grepple.yaml (b)\n" {
					t.Fatalf("unordered successful output=%q", output.String())
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if output.String() != "write .grepple/grepple.yaml (a)\nwrite .grepple/grepple.yaml (b)\n" {
					t.Fatalf("unordered output=%q", output.String())
				}
			}
			document, readErr := directorymeta.ReadDocumentV1(root)
			if readErr != nil || len(document.Directory["b"].Files) != 1 || (len(document.Directory["a"].Files) == 1) == tc.failFirst {
				t.Fatalf("lost sibling: %+v err=%v", document, readErr)
			}
		})
	}
}
