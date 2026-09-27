package examples

import (
	"bytes"
	"strings"
	"testing"
)

func TestExamplesCoverTaskOrientedWorkflows(t *testing.T) {
	var output bytes.Buffer
	if err := runExamples(nil, &output); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"orient —", "tree --depth 1", "--outline", "retrieve —", "--files-with-matches", "--at", "edit —", "--line-only", "grepple write", "impact —", "graph resolve", "graph callers", "audit —", "grit --limit 0 --json"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("examples output missing %q:\n%s", expected, output.String())
		}
	}
	for _, removed := range []string{"grepple boundaries", "grepple extract", "architecture why", "architecture resolve"} {
		if strings.Contains(output.String(), removed) {
			t.Fatalf("examples expose removed command %q:\n%s", removed, output.String())
		}
	}
}

func TestExamplesSelectOneTaskAndRejectUnknownTask(t *testing.T) {
	var output bytes.Buffer
	if err := runExamples([]string{"impact"}, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "graph callers") || strings.Contains(output.String(), "make schema-generate") {
		t.Fatalf("filtered examples output:\n%s", output.String())
	}
	if err := runExamples([]string{"missing"}, &output); err == nil || !strings.Contains(err.Error(), "orient, retrieve, edit") {
		t.Fatalf("unknown task error = %v", err)
	}
}

func TestExamplesHelpListsTasks(t *testing.T) {
	var output bytes.Buffer
	if err := runExamples([]string{"--help"}, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Usage: grepple examples [TASK]") || !strings.Contains(output.String(), "orient, retrieve, edit, impact, audit") {
		t.Fatalf("examples help:\n%s", output.String())
	}
}
