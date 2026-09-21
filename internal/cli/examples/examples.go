// Package examples implements the task-oriented examples command.
package examples

import (
	"fmt"
	"io"
	"strings"

	cliruntime "github.com/greppleai/grepple/internal/cli/runtime"
)

type taskExample struct {
	Name        string
	Description string
	Commands    []string
}

var taskExamples = []taskExample{
	{Name: "orient", Description: "Inspect source scope, orient by directory, and resolve one declaration", Commands: []string{
		"grepple sources explain --compact .",
		"grepple architecture directory --depth 2 --compact .",
		"grepple architecture resolve --symbol Document --compact .",
	}},
	{Name: "retrieve", Description: "Find content-matching files, then retrieve an exact declaration range", Commands: []string{
		"grepple -F 'BuildNavigationGraph' --files-with-matches ./parser",
		"grepple --line-only --enclosing -F 'BuildNavigationGraph' ./parser",
		"grepple --at parser/navigation.go:113",
	}},
	{Name: "edit", Description: "Retrieve native hashline anchors, then apply literal replacement text without JSON escaping", Commands: []string{
		"grepple --line-only -F 'BuildNavigationGraph' parser/navigation.go",
		"printf '%s\n' 'literal replacement' | grepple write edit --root . --path parser/navigation.go --start START --end END",
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
	{Name: "architecture", Description: "Explain one relation or diagnose drift between complete directory reports", Commands: []string{
		"grepple architecture why rulespec search --compact rulespec search",
		"grepple architecture compare --compact before.json after.json",
	}},
}

// Dependencies supplies process-owned examples command resources.
type Dependencies struct{ Output io.Writer }

type command struct{ dependencies Dependencies }

// New constructs the examples command.
func New(dependencies Dependencies) cliruntime.Command { return &command{dependencies: dependencies} }

// Run executes the examples command. Deprecated: construct the command with New.
func Run(args []string, output io.Writer) error { return New(Dependencies{Output: output}).Run(args) }

// Run executes the examples command.
func (command *command) Run(args []string) error {
	output := command.dependencies.Output
	if len(args) > 1 {
		return fmt.Errorf("examples accepts at most one task")
	}
	writer := cliruntime.NewOutput(output)
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		return writer.WriteString(help())
	}
	selected, err := selectTaskExamples(args)
	if err != nil {
		return err
	}
	return writer.WriteString(renderTaskExamples(selected))
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

func help() string {
	return "Task-oriented Grepple workflows.\nUsage: grepple examples [TASK]\nTasks: " + taskExampleNames() + "\nOmit TASK to print every workflow. Commands are copyable templates; replace example paths and symbols.\n"
}

func taskExampleNames() string {
	names := make([]string, len(taskExamples))
	for index, example := range taskExamples {
		names[index] = example.Name
	}
	return strings.Join(names, ", ")
}
