package examples

import (
	"bytes"
	"strings"
	"testing"
)

func TestExamplesCoverTaskOrientedWorkflows(t *testing.T) {
	var output bytes.Buffer
	if err := Run(nil, &output); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"orient —", "sources explain", "architecture directory", "architecture resolve", "retrieve —", "--files-with-matches", "--at", "edit —", "--line-only", "grepple write", "impact —", "graph resolve", "graph callers", "boundaries —", "boundaries --json", "audit —", "grit --limit 0 --json", "diagram —", "extract structure", "extract flow", "architecture —", "architecture why"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("examples output missing %q:\n%s", expected, output.String())
		}
	}
}

func TestExamplesSelectOneTaskAndRejectUnknownTask(t *testing.T) {
	var output bytes.Buffer
	if err := Run([]string{"impact"}, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "graph callers") || strings.Contains(output.String(), "make schema-generate") {
		t.Fatalf("filtered examples output:\n%s", output.String())
	}
	if err := Run([]string{"missing"}, &output); err == nil || !strings.Contains(err.Error(), "orient, retrieve, edit") {
		t.Fatalf("unknown task error = %v", err)
	}
}

func TestExamplesHelpListsTasks(t *testing.T) {
	var output bytes.Buffer
	if err := Run([]string{"--help"}, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Usage: grepple examples [TASK]") || !strings.Contains(output.String(), "architecture") {
		t.Fatalf("examples help:\n%s", output.String())
	}
}
