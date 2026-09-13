package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/alexflint/go-arg"
	"github.com/greppleai/grepple/parser"
	"github.com/greppleai/grepple/search"
)

type graphDiffArgs struct {
	JSON           bool   `arg:"--json" help:"emit the complete semantic graph diff as JSON"`
	Compact        bool   `arg:"--compact" help:"emit a bounded agent-facing semantic graph diff"`
	Before         string `arg:"--before,required" placeholder:"PATH" help:"source tree before the change"`
	After          string `arg:"--after,required" placeholder:"PATH" help:"source tree after the change"`
	MaxFiles       int    `arg:"--max-files" placeholder:"N" help:"parse at most N files in each tree (0 = unlimited)"`
	MaxOutputBytes int    `arg:"--max-output-bytes" placeholder:"N" help:"cap compact output (default 16384; 0 = unlimited; JSON is uncapped)"`
}

func (graphDiffArgs) Description() string {
	return "Compare two source trees by semantic declarations and call edges while ignoring position-only line changes."
}

type graphDiffOutput struct {
	BeforeFiles int `json:"beforeFiles"`
	AfterFiles  int `json:"afterFiles"`
	search.NavigationGraphDiff
}

func runGraphDiff(args []string) error {
	values := graphDiffArgs{MaxOutputBytes: DefaultTextOutputBytes}
	argumentParser, err := arg.NewParser(arg.Config{Program: "grepple graph diff"}, &values)
	if err != nil {
		return err
	}
	if err := argumentParser.Parse(args); err != nil {
		if errors.Is(err, arg.ErrHelp) {
			argumentParser.WriteHelp(os.Stdout)
			return nil
		}
		return err
	}
	if values.JSON == values.Compact {
		return fmt.Errorf("grepple graph diff requires exactly one of --json or --compact")
	}
	if values.MaxFiles < 0 || values.MaxOutputBytes < 0 {
		return fmt.Errorf("graph diff limits must be non-negative")
	}
	before, err := buildNavigationGraphOutput([]string{values.Before}, values.MaxFiles)
	if err != nil {
		return err
	}
	after, err := buildNavigationGraphOutput([]string{values.After}, values.MaxFiles)
	if err != nil {
		return err
	}
	beforeGraph := relativeNavigationGraph(before, values.Before)
	afterGraph := relativeNavigationGraph(after, values.After)
	output := graphDiffOutput{BeforeFiles: before.Files, AfterFiles: after.Files, NavigationGraphDiff: search.DiffNavigationGraphs(
		beforeGraph,
		afterGraph,
	)}
	if values.JSON {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetEscapeHTML(false)
		encoder.SetIndent("", "  ")
		return encoder.Encode(output)
	}
	return renderCompactGraphDiff(output, values.MaxOutputBytes)
}

func relativeNavigationGraph(output navigationGraphOutput, root string) parser.NavigationGraph {
	graph := parser.NavigationGraph{Declarations: append([]parser.NavigationDeclaration(nil), output.Declarations...), Calls: append([]parser.NavigationCall(nil), output.Calls...)}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return graph
	}
	for index := range graph.Declarations {
		graph.Declarations[index].Path = relativeGraphDiffPath(absoluteRoot, graph.Declarations[index].Path)
	}
	for index := range graph.Calls {
		graph.Calls[index].Path = relativeGraphDiffPath(absoluteRoot, graph.Calls[index].Path)
	}
	return graph
}

func relativeGraphDiffPath(root, path string) string {
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	relative, err := filepath.Rel(root, absolutePath)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(relative)
}

func renderCompactGraphDiff(diff graphDiffOutput, maxBytes int) error {
	output := stdoutWriter()
	if maxBytes > 0 {
		output = newBoundedOutputWriter(os.Stdout, maxBytes)
	}
	write := func(value string) bool { return output.writeString(value+"\n") == nil }
	if !write(fmt.Sprintf("graph-diff %s files=%d->%d declarations=+%d/-%d/~%d/>%d calls=+%d/-%d/~%d", diff.Schema, diff.BeforeFiles, diff.AfterFiles, len(diff.AddedDeclarations), len(diff.RemovedDeclarations), len(diff.ChangedDeclarations), len(diff.MovedDeclarations), len(diff.AddedCalls), len(diff.RemovedCalls), len(diff.ChangedCalls))) {
		return nil
	}
	if !renderCompactDeclarationDiff(write, diff) {
		return nil
	}
	renderCompactCallDiff(write, diff)
	return nil
}

func renderCompactDeclarationDiff(write func(string) bool, diff graphDiffOutput) bool {
	for _, declaration := range diff.AddedDeclarations {
		if !write("+ D " + compactDiffDeclaration(declaration)) {
			return false
		}
	}
	for _, declaration := range diff.RemovedDeclarations {
		if !write("- D " + compactDiffDeclaration(declaration)) {
			return false
		}
	}
	for _, change := range diff.MovedDeclarations {
		if !write(fmt.Sprintf("> D %s %s -> %s", change.Before.Name, graphDeclarationLocation(*change.Before), graphDeclarationLocation(*change.After))) {
			return false
		}
	}
	for _, change := range diff.ChangedDeclarations {
		if !write("~ D " + compactDiffDeclaration(*change.After)) {
			return false
		}
	}
	return true
}

func renderCompactCallDiff(write func(string) bool, diff graphDiffOutput) bool {
	for _, call := range diff.AddedCalls {
		if !write("+ C " + compactDiffCall(call)) {
			return false
		}
	}
	for _, call := range diff.RemovedCalls {
		if !write("- C " + compactDiffCall(call)) {
			return false
		}
	}
	for _, change := range diff.ChangedCalls {
		if !write("~ C " + compactDiffCall(*change.After)) {
			return false
		}
	}
	return true
}

func compactDiffDeclaration(declaration parser.NavigationDeclaration) string {
	return fmt.Sprintf("%s %s %s %s visibility=%s", declaration.Language, declaration.Kind, declaration.Name, graphDeclarationLocation(declaration), declaration.Visibility)
}

func compactDiffCall(call parser.NavigationCall) string {
	return fmt.Sprintf("%s %s confidence=%s %s:%d", call.Language, call.Display, call.Confidence, call.Path, call.Line)
}
