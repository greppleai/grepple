package cli

import (
	"strings"
	"testing"
)

func TestExamplesCoverTaskOrientedWorkflows(t *testing.T) {
	output := captureStdout(t, func() {
		if err := Run([]string{"examples"}); err != nil {
			t.Fatal(err)
		}
	})
	for _, expected := range []string{
		"orient —", "sources explain", "architecture directory", "architecture resolve", "retrieve —", "--files-with-matches", "--at", "edit —", "--anchors", "grepple write",
		"impact —", "graph resolve", "graph callers", "boundaries —", "boundaries --json",
		"audit —", "grit --limit 0 --json", "diagram —", "extract structure", "extract flow",
		"architecture —", "architecture why",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("examples output missing %q:\n%s", expected, output)
		}
	}
}

func TestExamplesSelectOneTaskAndRejectUnknownTask(t *testing.T) {
	output := captureStdout(t, func() {
		if err := Run([]string{"examples", "impact"}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(output, "graph callers") || strings.Contains(output, "make schema-generate") {
		t.Fatalf("filtered examples output:\n%s", output)
	}
	if err := Run([]string{"examples", "missing"}); err == nil || !strings.Contains(err.Error(), "orient, retrieve, edit") {
		t.Fatalf("unknown task error = %v", err)
	}
}

func TestExamplesHelpListsTasks(t *testing.T) {
	output := captureStdout(t, func() {
		if err := Run([]string{"help", "examples"}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(output, "Usage: grepple examples [TASK]") || !strings.Contains(output, "architecture") {
		t.Fatalf("examples help:\n%s", output)
	}
}
