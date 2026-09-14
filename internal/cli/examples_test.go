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
		"orient —", "extract summary workspace", "retrieve —", "--at", "edit —", "anchors doctor",
		"impact —", "graph resolve", "graph callers", "boundaries —", "boundaries --json",
		"audit —", "grit --limit 0 --json", "diagram —", "extract structure", "extract flow",
		"canonical —", "make schema-generate", "make schema-check",
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
	if !strings.Contains(output, "Usage: grepple examples [TASK]") || !strings.Contains(output, "canonical") {
		t.Fatalf("examples help:\n%s", output)
	}
}
