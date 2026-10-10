package api

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestNavigationSelectionRestoresHitsAndRejectsChangedSource(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "sample.go")
	source := []byte("package sample\nfunc Root(){Leaf()}\nfunc Leaf(){}\n")
	if err := os.WriteFile(path, source, 0600); err != nil {
		t.Fatal(err)
	}
	batch, err := SearchFiles(NewSearchPlan(SearchPlanOptions{Root: root, Query: "Root"}), []string{path})
	if err != nil {
		t.Fatal(err)
	}
	rows := CaptureNavigationSelections(batch)
	restored, err := RestoreNavigationSelections(t.Context(), rows, []string{path})
	if err != nil {
		t.Fatal(err)
	}
	expected := batch.Results(0, 0, false)
	actual := restored.Results(0, 0, false)
	if !reflect.DeepEqual(expected[0].Matches, actual[0].Matches) || expected[0].Path != actual[0].Path {
		t.Fatal("matching facts changed")
	}
	if err := os.WriteFile(path, []byte("package changed\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := RestoreNavigationSelections(t.Context(), rows, []string{path}); err == nil {
		t.Fatal("stale selection accepted")
	}
}
func TestNavigationBatchMergePreservesOriginalAndRejectsOverlap(t *testing.T) {
	root := t.TempDir()
	a := filepath.Join(root, "a.go")
	b := filepath.Join(root, "b.go")
	for _, path := range []string{a, b} {
		if err := os.WriteFile(path, []byte("package p\nfunc Root(){}\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	batch, err := SearchFiles(NewSearchPlan(SearchPlanOptions{Root: root, Query: "Root"}), []string{a, b})
	if err != nil {
		t.Fatal(err)
	}
	first, _ := SelectNavigationBatchPaths(batch, []string{a})
	second, _ := SelectNavigationBatchPaths(batch, []string{b})
	merged, err := MergeNavigationBatches(batch, []SearchBatch{second, first})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(batch.Results(0, 0, true), merged.Results(0, 0, true)) {
		t.Fatal("result order or facts changed")
	}
	if _, err := MergeNavigationBatches(batch, []SearchBatch{first, first}); err == nil {
		t.Fatal("overlapping ownership accepted")
	}
}
