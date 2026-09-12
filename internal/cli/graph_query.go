package cli

import (
	"context"
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

const maxNavigationQueryDepth = 10

type graphQueryArgs struct {
	JSON           bool     `arg:"--json" help:"emit the complete queried subgraph as JSON"`
	Compact        bool     `arg:"--compact" help:"emit a bounded agent-facing queried subgraph"`
	Symbol         string   `arg:"--symbol" placeholder:"NAME" help:"select one exact declaration name"`
	At             string   `arg:"--at" placeholder:"PATH:LINE" help:"select the declaration containing a source location"`
	Depth          int      `arg:"--depth" default:"1" placeholder:"N" help:"maximum traversal depth (1-10)"`
	MaxFiles       int      `arg:"--max-files" placeholder:"N" help:"parse at most N discovered files (0 = unlimited)"`
	MaxOutputBytes int      `arg:"--max-output-bytes" placeholder:"N" help:"cap compact output (default 40960; 0 = unlimited; JSON is uncapped)"`
	Paths          []string `arg:"positional" placeholder:"PATH" help:"file, directory, or glob to include; defaults to the working directory"`
}

func (graphQueryArgs) Description() string {
	return "Query callers or callees over the deterministic local navigation graph."
}

func runGraphQuery(direction search.NavigationQueryDirection, args []string) error {
	values := graphQueryArgs{MaxOutputBytes: DefaultTextOutputBytes, Depth: 1}
	argumentParser, err := arg.NewParser(arg.Config{Program: "grepple graph " + string(direction)}, &values)
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
	if err := validateGraphQueryArgs(values); err != nil {
		return err
	}
	output, err := buildNavigationGraphOutput(values.Paths, values.MaxFiles)
	if err != nil {
		return err
	}
	root, err := selectNavigationQueryRoot(output.Declarations, values.Symbol, values.At)
	if err != nil {
		return err
	}
	queried, err := search.QueryNavigationGraph(parser.NavigationGraph{Declarations: output.Declarations, Calls: output.Calls}, []string{root.ID}, direction, values.Depth)
	if err != nil {
		return err
	}
	output.Declarations = queried.Declarations
	output.Calls = queried.Calls
	output.Query = &navigationGraphQuery{Direction: string(direction), Depth: values.Depth, RootIDs: []string{root.ID}}
	if values.Compact {
		return renderCompactNavigationGraph(output, values.MaxOutputBytes)
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}

func validateGraphQueryArgs(values graphQueryArgs) error {
	if values.JSON == values.Compact {
		return fmt.Errorf("grepple graph query requires exactly one of --json or --compact")
	}
	if (values.Symbol == "") == (values.At == "") {
		return fmt.Errorf("grepple graph query requires exactly one of --symbol or --at")
	}
	if values.Depth < 1 || values.Depth > maxNavigationQueryDepth {
		return fmt.Errorf("--depth must be between 1 and %d", maxNavigationQueryDepth)
	}
	if values.MaxFiles < 0 {
		return fmt.Errorf("--max-files must be non-negative")
	}
	if values.MaxOutputBytes < 0 {
		return fmt.Errorf("--max-output-bytes must be non-negative")
	}
	return nil
}

func buildNavigationGraphOutput(globs []string, maxFiles int) (navigationGraphOutput, error) {
	paths, err := search.ListFilePathsContext(context.Background(), search.Params{Files: true, Globs: globs}, nil)
	if err != nil {
		return navigationGraphOutput{}, err
	}
	paths = navigationSourcePaths(paths)
	var truncation *navigationGraphTruncation
	if maxFiles > 0 && len(paths) > maxFiles {
		truncation = &navigationGraphTruncation{Reason: "max_files", Limit: maxFiles, Skipped: len(paths) - maxFiles}
		paths = paths[:maxFiles]
	}
	graph := search.BuildNavigationGraph(paths)
	declarations := graph.Declarations
	if declarations == nil {
		declarations = []parser.NavigationDeclaration{}
	}
	calls := graph.Calls
	if calls == nil {
		calls = []parser.NavigationCall{}
	}
	return navigationGraphOutput{
		Schema: navigationGraphSchema, Files: len(paths), Declarations: declarations, Calls: calls, Truncation: truncation,
	}, nil
}

func selectNavigationQueryRoot(declarations []parser.NavigationDeclaration, symbol, at string) (parser.NavigationDeclaration, error) {
	if symbol != "" {
		matches := make([]parser.NavigationDeclaration, 0, 1)
		for _, declaration := range declarations {
			if declaration.Name == symbol {
				matches = append(matches, declaration)
			}
		}
		return requireUniqueNavigationRoot(matches, "symbol "+fmt.Sprintf("%q", symbol))
	}
	path, line, err := extractAt(at)
	if err != nil {
		return parser.NavigationDeclaration{}, err
	}
	path = navigationQueryDisplayPath(path)
	matches := make([]parser.NavigationDeclaration, 0, 1)
	for _, declaration := range declarations {
		if filepath.ToSlash(declaration.Path) == path && line >= declaration.Start && line <= declaration.End {
			matches = append(matches, declaration)
		}
	}
	if len(matches) > 1 {
		matches = narrowestNavigationRoots(matches)
	}
	return requireUniqueNavigationRoot(matches, "location "+at)
}

func navigationQueryDisplayPath(path string) string {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return filepath.ToSlash(filepath.Clean(path))
	}
	cwd, err := os.Getwd()
	if err != nil {
		return filepath.ToSlash(filepath.Clean(path))
	}
	relative, err := filepath.Rel(cwd, absolute)
	if err != nil {
		return filepath.ToSlash(filepath.Clean(path))
	}
	return filepath.ToSlash(relative)
}

func narrowestNavigationRoots(matches []parser.NavigationDeclaration) []parser.NavigationDeclaration {
	width := matches[0].End - matches[0].Start
	for _, match := range matches[1:] {
		if candidateWidth := match.End - match.Start; candidateWidth < width {
			width = candidateWidth
		}
	}
	narrowest := matches[:0]
	for _, match := range matches {
		if match.End-match.Start == width {
			narrowest = append(narrowest, match)
		}
	}
	return narrowest
}

func requireUniqueNavigationRoot(matches []parser.NavigationDeclaration, selector string) (parser.NavigationDeclaration, error) {
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) == 0 {
		return parser.NavigationDeclaration{}, fmt.Errorf("no navigation declaration matches %s", selector)
	}
	locations := make([]string, 0, len(matches))
	for _, match := range matches {
		locations = append(locations, fmt.Sprintf("--at %s:%d", match.Path, match.Start))
	}
	return parser.NavigationDeclaration{}, fmt.Errorf("navigation declaration %s is ambiguous; try %s", selector, strings.Join(locations, " or "))
}
