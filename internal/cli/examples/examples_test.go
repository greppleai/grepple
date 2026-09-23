package examples

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunSelectsAndDescribesExamples(t *testing.T) {
	var output bytes.Buffer
	if err := runExamples([]string{"impact"}, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "graph callers") || strings.Contains(output.String(), "extract flow") {
		t.Fatalf("filtered examples output:\n%s", output.String())
	}
}

func TestRunHelpAndValidation(t *testing.T) {
	var output bytes.Buffer
	if err := runExamples([]string{"--help"}, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Usage: grepple examples [TASK]") {
		t.Fatalf("help=%q", output.String())
	}
	if err := runExamples([]string{"missing"}, &output); err == nil || !strings.Contains(err.Error(), "orient, retrieve, edit") {
		t.Fatalf("unknown task error=%v", err)
	}
	if err := runExamples([]string{"orient", "retrieve"}, &output); err == nil {
		t.Fatal("expected too-many-arguments error")
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
