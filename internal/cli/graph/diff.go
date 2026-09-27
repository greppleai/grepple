package graph

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/alexflint/go-arg"
	"github.com/greppleai/grepple/internal/wire"
	cliruntime "github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/navigation"
	"github.com/greppleai/grepple/internal/parser"
)

type graphDiffArgs struct {
	JSON           bool   `arg:"--json" help:"emit the complete semantic graph diff as JSON"`
	Before         string `arg:"--before,required" placeholder:"PATH" help:"source tree before the change"`
	After          string `arg:"--after,required" placeholder:"PATH" help:"source tree after the change"`
	MaxFiles       int    `arg:"--max-files" placeholder:"N" help:"parse at most N files in each tree (0 = unlimited)"`
	MaxOutputBytes int    `arg:"--max-output-bytes" default:"16384" placeholder:"N" help:"cap human-readable output (default 16384; 0 = unlimited; JSON is uncapped)"`
}

func (graphDiffArgs) Description() string {
	return "Compare semantic declarations and calls while ignoring position-only movement. Human output is the default; --json emits the complete diff."
}

// DiffOutput is the complete navigation graph diff projection.
type DiffOutput struct {
	Metadata      *wire.ResultMetadata     `json:"metadata,omitempty"`
	BeforeFiles   int                     `json:"beforeFiles"`
	BeforeSources navigationSourceSummary `json:"beforeSources"`
	AfterFiles    int                     `json:"afterFiles"`
	AfterSources  navigationSourceSummary `json:"afterSources"`
	navigation.NavigationGraphDiff
}

// runDiff compares two navigation graphs.
func runDiff(application cliruntime.Context, args []string) error {
	values := graphDiffArgs{MaxOutputBytes: defaultTextOutputBytes}
	argumentParser, err := arg.NewParser(arg.Config{Program: "grepple graph diff"}, &values)
	if err != nil {
		return err
	}
	if err := argumentParser.Parse(args); err != nil {
		if errors.Is(err, arg.ErrHelp) {
			argumentParser.WriteHelp(application.Stdout())
			return nil
		}
		return err
	}
	return executeDiff(application, &values)
}

func executeDiff(application cliruntime.Context, values *DiffArgs) error {
	if values.MaxFiles < 0 || values.MaxOutputBytes < 0 {
		return fmt.Errorf("graph diff limits must be non-negative")
	}
	before, err := buildNavigationGraphOutput(application, []string{values.Before}, values.MaxFiles)
	if err != nil {
		return err
	}
	after, err := buildNavigationGraphOutput(application, []string{values.After}, values.MaxFiles)
	if err != nil {
		return err
	}
	beforeGraph := relativeNavigationGraph(before, values.Before)
	afterGraph := relativeNavigationGraph(after, values.After)
	output := DiffOutput{BeforeFiles: before.Files, BeforeSources: before.Sources, AfterFiles: after.Files, AfterSources: after.Sources, NavigationGraphDiff: navigation.DiffNavigationGraphs(
		beforeGraph,
		afterGraph,
	)}
	output.Metadata = graphDiffResultMetadata(application, diffMetadataInput{BeforePath: values.Before, AfterPath: values.After, MaxFiles: values.MaxFiles, MaxOutputBytes: values.MaxOutputBytes, JSON: values.JSON, Before: before, After: after, Diff: output.NavigationGraphDiff})
	if values.JSON {
		encoder := json.NewEncoder(application.Stdout())
		encoder.SetEscapeHTML(false)
		encoder.SetIndent("", "  ")
		return encoder.Encode(output)
	}
	return renderCompactGraphDiff(application.Stdout(), output, values.MaxOutputBytes)
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

func renderCompactGraphDiff(destination io.Writer, diff DiffOutput, maxBytes int) error {
	output := cliruntime.NewOutput(destination)
	if maxBytes > 0 {
		output = cliruntime.NewBoundedOutput(destination, maxBytes)
	}
	write := func(value string) bool { return output.WriteString(value+"\n") == nil }
	if !write(fmt.Sprintf("graph-diff files=%d->%d declarations=+%d/-%d/~%d/>%d calls=+%d/-%d/~%d", diff.BeforeFiles, diff.AfterFiles, len(diff.AddedDeclarations), len(diff.RemovedDeclarations), len(diff.ChangedDeclarations), len(diff.MovedDeclarations), len(diff.AddedCalls), len(diff.RemovedCalls), len(diff.ChangedCalls))) {
		return nil
	}
	if (diff.Metadata != nil && !diff.Metadata.Page.Complete) || diff.BeforeSources.Recovered > 0 || diff.AfterSources.Recovered > 0 {
		if !write(fmt.Sprintf("! incomplete before(%s) after(%s); inspect --json diagnostics", compactNavigationSourceSummary(diff.BeforeSources), compactNavigationSourceSummary(diff.AfterSources))) {
			return nil
		}
	}
	if diff.Metadata != nil && diff.Metadata.NextCommand != "" && !write("continue: "+diff.Metadata.NextCommand) {
		return nil
	}
	if !renderCompactDeclarationDiff(write, diff) {
		return nil
	}
	if !renderCompactCallDiff(write, diff) {
		return nil
	}
	return nil
}

func renderCompactDeclarationDiff(write func(string) bool, diff DiffOutput) bool {
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

func renderCompactCallDiff(write func(string) bool, diff DiffOutput) bool {
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
