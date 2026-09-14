package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/alexflint/go-arg"
	"github.com/greppleai/grepple/api"
	"github.com/greppleai/grepple/parser"
	"github.com/greppleai/grepple/search"
)

const navigationResolveSchema = "grepple-navigation-resolve-v1"

type graphResolveArgs struct {
	JSON           bool     `arg:"--json" help:"emit every matching declaration and continuation command as JSON"`
	Compact        bool     `arg:"--compact" help:"emit bounded declaration alternatives with copyable commands"`
	Symbol         string   `arg:"--symbol,required" placeholder:"NAME" help:"preview exact or terminal-name declaration matches"`
	Languages      []string `arg:"--language,separate" placeholder:"ID" help:"retain one navigation language; repeatable"`
	Visibilities   []string `arg:"--visibility,separate" placeholder:"LEVEL" help:"retain public, non-public, or unknown declarations; repeatable"`
	MaxFiles       int      `arg:"--max-files" placeholder:"N" help:"parse at most N discovered files (0 = unlimited)"`
	MaxOutputBytes int      `arg:"--max-output-bytes" placeholder:"N" help:"cap compact output (default 16384; 0 = unlimited; JSON is uncapped)"`
	Paths          []string `arg:"positional" placeholder:"PATH" help:"graph universe; defaults to the working directory"`
}

func (graphResolveArgs) Description() string {
	return "Preview every declaration matching a symbol without graph traversal. Exactly one of --json or --compact is required."
}

type graphResolveOutput struct {
	Schema     string                     `json:"schema"`
	Metadata   *api.ResultMetadata        `json:"metadata,omitempty"`
	Symbol     string                     `json:"symbol"`
	Sources    navigationSourceSummary    `json:"sources"`
	Matches    []graphResolveMatch        `json:"matches"`
	Truncation *navigationGraphTruncation `json:"truncation,omitempty"`
}

type graphResolveMatch struct {
	ID             string                      `json:"id"`
	Name           string                      `json:"name"`
	Kind           string                      `json:"kind"`
	Language       string                      `json:"language"`
	Path           string                      `json:"path"`
	StartLine      int                         `json:"startLine"`
	EndLine        int                         `json:"endLine"`
	Visibility     parser.NavigationVisibility `json:"visibility"`
	At             string                      `json:"at"`
	CallersCommand string                      `json:"callersCommand"`
	CalleesCommand string                      `json:"calleesCommand"`
	ImpactCommand  string                      `json:"impactCommand"`
}

func runGraphResolve(args []string) error {
	values := graphResolveArgs{MaxOutputBytes: DefaultTextOutputBytes}
	argumentParser, err := arg.NewParser(arg.Config{Program: "grepple graph resolve"}, &values)
	if err != nil {
		return err
	}
	if err := argumentParser.Parse(args); err != nil {
		if errors.Is(err, arg.ErrHelp) {
			argumentParser.WriteHelp(os.Stdout)
			fmt.Fprintln(os.Stdout, "Required output mode: (--json | --compact); choose exactly one.")
			fmt.Fprintln(os.Stdout, "Required selector: --symbol NAME.")
			return nil
		}
		return err
	}
	if err := validateGraphResolveArgs(values); err != nil {
		return err
	}
	graph, err := buildNavigationGraphOutput(values.Paths, values.MaxFiles)
	if err != nil {
		return err
	}
	filter, err := search.NormalizeNavigationGraphFilter(search.NavigationGraphFilter{Languages: values.Languages, Visibilities: values.Visibilities})
	if err != nil {
		return err
	}
	filtered, err := search.FilterNavigationGraph(parser.NavigationGraph{Declarations: graph.Declarations}, filter)
	if err != nil {
		return err
	}
	matches := graphResolveMatches(filtered.Declarations, values.Symbol, values.Paths)
	output := graphResolveOutput{Schema: navigationResolveSchema, Symbol: values.Symbol, Sources: graph.Sources, Matches: matches, Truncation: graph.Truncation}
	output.Metadata = graphResultMetadata(values.Paths, len(matches), values.MaxFiles, values.MaxOutputBytes, values.JSON, graph.Sources, graph.Truncation, graphResolveContinuationCommand(values, graph.Truncation))
	output.Metadata.Scope.Languages = normalizedResultScope(filter.Languages, "")
	var outputErr error
	if values.Compact {
		outputErr = renderCompactGraphResolve(output, values.MaxOutputBytes)
	} else {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetEscapeHTML(false)
		encoder.SetIndent("", "  ")
		outputErr = encoder.Encode(output)
	}
	if outputErr != nil {
		return outputErr
	}
	if len(matches) == 0 {
		setExit(1)
	}
	return nil
}

func validateGraphResolveArgs(values graphResolveArgs) error {
	if values.JSON == values.Compact {
		return fmt.Errorf("grepple graph resolve requires exactly one of --json or --compact")
	}
	if strings.TrimSpace(values.Symbol) == "" {
		return fmt.Errorf("grepple graph resolve requires --symbol NAME")
	}
	if values.MaxFiles < 0 || values.MaxOutputBytes < 0 {
		return fmt.Errorf("graph resolve limits must be non-negative")
	}
	return nil
}

func graphResolveMatches(declarations []parser.NavigationDeclaration, symbol string, paths []string) []graphResolveMatch {
	matched := matchingNavigationDeclarations(declarations, symbol)
	sort.Slice(matched, func(left, right int) bool {
		if matched[left].Path != matched[right].Path {
			return matched[left].Path < matched[right].Path
		}
		if matched[left].Start != matched[right].Start {
			return matched[left].Start < matched[right].Start
		}
		return matched[left].ID < matched[right].ID
	})
	scope := graphResolveScope(paths)
	result := make([]graphResolveMatch, 0, len(matched))
	for _, declaration := range matched {
		at := fmt.Sprintf("%s:%d", declaration.Path, declaration.Start)
		commandPrefix := " --at " + quoteCommandArgument(at) + " --depth 2 --compact " + scope
		result = append(result, graphResolveMatch{
			ID: declaration.ID, Name: declaration.Name, Kind: declaration.Kind, Language: declaration.Language,
			Path: declaration.Path, StartLine: declaration.Start, EndLine: declaration.End, Visibility: declaration.Visibility, At: at,
			CallersCommand: "grepple graph callers" + commandPrefix,
			CalleesCommand: "grepple graph callees" + commandPrefix,
			ImpactCommand:  "grepple graph impact" + commandPrefix,
		})
	}
	return result
}

func matchingNavigationDeclarations(declarations []parser.NavigationDeclaration, symbol string) []parser.NavigationDeclaration {
	exact := make([]parser.NavigationDeclaration, 0)
	terminal := make([]parser.NavigationDeclaration, 0)
	for _, declaration := range declarations {
		if declaration.Name == symbol {
			exact = append(exact, declaration)
		} else if navigationTerminalName(declaration.Name) == symbol {
			terminal = append(terminal, declaration)
		}
	}
	if len(exact) > 0 {
		return exact
	}
	return terminal
}

func navigationTerminalName(name string) string {
	if separator := strings.LastIndexAny(name, ".#:"); separator >= 0 {
		return name[separator+1:]
	}
	return name
}

func graphResolveScope(paths []string) string {
	if len(paths) == 0 {
		return "."
	}
	quoted := make([]string, len(paths))
	for index, path := range paths {
		quoted[index] = quoteCommandArgument(path)
	}
	return strings.Join(quoted, " ")
}

func graphResolveContinuationCommand(values graphResolveArgs, truncation *navigationGraphTruncation) string {
	if truncation == nil {
		return ""
	}
	parts := []string{"grepple", "graph", "resolve", "--symbol", quoteCommandArgument(values.Symbol), "--max-files", "0", "--json"}
	for _, language := range values.Languages {
		parts = append(parts, "--language", quoteCommandArgument(language))
	}
	for _, visibility := range values.Visibilities {
		parts = append(parts, "--visibility", quoteCommandArgument(visibility))
	}
	for _, path := range normalizedResultScope(values.Paths, ".") {
		parts = append(parts, quoteCommandArgument(path))
	}
	return strings.Join(parts, " ")
}

func renderCompactGraphResolve(output graphResolveOutput, maxBytes int) error {
	writer := stdoutWriter()
	if maxBytes > 0 {
		writer = newBoundedOutputWriter(os.Stdout, maxBytes)
	}
	if err := writer.writeString(fmt.Sprintf("resolve %s symbol=%s matches=%d sources=%s\n", output.Schema, quoteCommandArgument(output.Symbol), len(output.Matches), compactNavigationSourceSummary(output.Sources))); err != nil {
		return nil
	}
	if output.Truncation != nil {
		if err := writer.writeString(fmt.Sprintf("! truncated %s limit=%d skipped=%d\n", output.Truncation.Reason, output.Truncation.Limit, output.Truncation.Skipped)); err != nil {
			return nil
		}
	}
	if output.Metadata != nil && output.Metadata.NextCommand != "" {
		if err := writer.writeString("continue: " + output.Metadata.NextCommand + "\n"); err != nil {
			return nil
		}
	}
	for _, match := range output.Matches {
		line := fmt.Sprintf("D %s %s %s %s %s visibility=%s\n  at: %s\n  callers: %s\n  callees: %s\n  impact: %s\n", shortGraphID(match.ID), match.Language, match.Kind, match.Name, graphResolveLocation(match), match.Visibility, match.At, match.CallersCommand, match.CalleesCommand, match.ImpactCommand)
		if err := writer.writeString(line); err != nil {
			return nil
		}
	}
	return nil
}

func graphResolveLocation(match graphResolveMatch) string {
	if match.StartLine == match.EndLine {
		return fmt.Sprintf("%s:%d", match.Path, match.StartLine)
	}
	return fmt.Sprintf("%s:%d-%d", match.Path, match.StartLine, match.EndLine)
}
