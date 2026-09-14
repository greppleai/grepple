package cli

import (
	"fmt"
	"strings"
)

type taskExample struct {
	Name        string
	Description string
	Commands    []string
}

var taskExamples = []taskExample{
	{Name: "orient", Description: "Orient by directory and resolve one declaration", Commands: []string{
		"grepple architecture directory --depth 2 --compact .",
		"grepple architecture resolve --symbol Document --compact .",
	}},
	{Name: "retrieve", Description: "Locate a declaration and retrieve its exact range", Commands: []string{
		"grepple --line-only --enclosing -F 'BuildNavigationGraph' ./parser",
		"grepple --at parser/navigation.go:113",
	}},
	{Name: "edit", Description: "Validate edit anchors, then emit edit-ready source rows", Commands: []string{
		"grepple anchors doctor",
		"grepple --anchors --line-only -F 'BuildNavigationGraph' parser/navigation.go",
	}},
	{Name: "impact", Description: "Resolve an ambiguous symbol before inspecting callers", Commands: []string{
		"grepple graph resolve --symbol BuildNavigationGraph --compact .",
		"grepple graph callers --at parser/navigation.go:113 --depth 2 --compact .",
	}},
	{Name: "boundaries", Description: "Triage boundary signals, then request complete evidence", Commands: []string{
		"grepple boundaries ./parser",
		"grepple boundaries --json ./parser",
	}},
	{Name: "audit", Description: "Run a native read-only structural audit with complete JSON", Commands: []string{
		"grepple grit explain --json $'language go\n`exec.Command($args)`'",
		"grepple grit --limit 0 --json $'language go\n`exec.Command($args)`' '**/*.go'",
	}},
	{Name: "diagram", Description: "Generate focused source-linked structure and flow diagrams", Commands: []string{
		"grepple extract structure --entry NavigationGraph --source ./parser",
		"grepple extract flow --entry BuildNavigationGraph --source ./parser",
	}},
	{Name: "architecture", Description: "Explain one strongly resolved cross-directory relation", Commands: []string{
		"grepple architecture why rulespec search --compact rulespec search",
	}},
}

func runExamples(args []string) error {
	if len(args) > 1 {
		return fmt.Errorf("examples accepts at most one task")
	}
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		return stdoutWriter().writeString(examplesHelp())
	}
	selected, err := selectTaskExamples(args)
	if err != nil {
		return err
	}
	return stdoutWriter().writeString(renderTaskExamples(selected))
}

func selectTaskExamples(args []string) ([]taskExample, error) {
	if len(args) == 0 {
		return taskExamples, nil
	}
	for _, example := range taskExamples {
		if example.Name == args[0] {
			return []taskExample{example}, nil
		}
	}
	return nil, fmt.Errorf("unknown example task %q; choose %s", args[0], taskExampleNames())
}

func renderTaskExamples(examples []taskExample) string {
	var output strings.Builder
	for index, example := range examples {
		if index > 0 {
			output.WriteByte('\n')
		}
		fmt.Fprintf(&output, "%s — %s\n", example.Name, example.Description)
		for _, command := range example.Commands {
			fmt.Fprintf(&output, "  %s\n", command)
		}
	}
	return output.String()
}

func examplesHelp() string {
	return "Task-oriented Grepple workflows.\nUsage: grepple examples [TASK]\nTasks: " + taskExampleNames() + "\nOmit TASK to print every workflow. Commands are copyable templates; replace example paths and symbols.\n"
}

func taskExampleNames() string {
	names := make([]string, len(taskExamples))
	for index, example := range taskExamples {
		names[index] = example.Name
	}
	return strings.Join(names, ", ")
}
