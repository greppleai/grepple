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
		"orient —", "sources explain", "architecture directory", "architecture resolve", "retrieve —", "--files-with-matches", "--at", "edit —", "--line-only", "grepple write",
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

func TestExamplesDoNotRepeatWorkflowNamesOrSteps(t *testing.T) {
	names := map[string]bool{}
	commands := map[string]string{}
	for _, example := range taskExamples {
		if names[example.Name] {
			t.Errorf("duplicate example task %q", example.Name)
		}
		names[example.Name] = true
		if example.Description == "" || len(example.Commands) == 0 {
			t.Errorf("example task %q has incomplete guidance", example.Name)
		}
		for _, command := range example.Commands {
			if owner, duplicate := commands[command]; duplicate {
				t.Errorf("workflow command %q is duplicated by %q and %q", command, owner, example.Name)
			}
			commands[command] = example.Name
		}
	}
}
