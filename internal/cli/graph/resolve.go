package graph

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/alexflint/go-arg"
	"github.com/greppleai/grepple/api"
	cliruntime "github.com/greppleai/grepple/internal/cli/runtime"
	"github.com/greppleai/grepple/parser"
	"github.com/greppleai/grepple/search"
)

// NavigationResolveSchema identifies graph symbol-resolution responses.
const NavigationResolveSchema = "grepple-navigation-resolve-v1"
const defaultTextOutputBytes = 16 * 1024

type resolveArgs struct {
	JSON           bool     `arg:"--json" help:"emit every matching declaration and continuation command as JSON"`
	Compact        bool     `arg:"--compact" help:"emit bounded declaration alternatives with copyable commands"`
	Symbol         string   `arg:"--symbol,required" placeholder:"NAME" help:"preview exact or terminal-name declaration matches"`
	Languages      []string `arg:"--language,separate" placeholder:"ID" help:"retain one navigation language; repeatable"`
	Visibilities   []string `arg:"--visibility,separate" placeholder:"LEVEL" help:"retain public, non-public, or unknown declarations; repeatable"`
	MaxFiles       int      `arg:"--max-files" placeholder:"N" help:"parse at most N discovered files (0 = unlimited)"`
	MaxOutputBytes int      `arg:"--max-output-bytes" placeholder:"N" help:"cap compact output (default 16384; 0 = unlimited; JSON is uncapped)"`
	Paths          []string `arg:"positional" placeholder:"PATH" help:"graph universe; defaults to the working directory"`
}

func (resolveArgs) Description() string {
	return "Preview every declaration matching a symbol without graph traversal. Exactly one of --json or --compact is required."
}

// ResolveOutput is the complete graph symbol-resolution response.
type ResolveOutput struct {
	Schema     string              `json:"schema"`
	Metadata   *api.ResultMetadata `json:"metadata,omitempty"`
	Symbol     string              `json:"symbol"`
	Sources    SourceSummary       `json:"sources"`
	Matches    []ResolveMatch      `json:"matches"`
	Truncation *Truncation         `json:"truncation,omitempty"`
}

// ResolveMatch describes one matching declaration and its follow-up commands.
type ResolveMatch struct {
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

func (command *command) runResolve(args []string) error {
	values := resolveArgs{MaxOutputBytes: defaultTextOutputBytes}
	argumentParser, err := arg.NewParser(arg.Config{Program: "grepple graph resolve"}, &values)
	if err != nil {
		return err
	}
	if err := argumentParser.Parse(args); err != nil {
		if errors.Is(err, arg.ErrHelp) {
			argumentParser.WriteHelp(command.stdout())
			fmt.Fprintln(command.stdout(), "Required output mode: (--json | --compact); choose exactly one.")
			fmt.Fprintln(command.stdout(), "Required selector: --symbol NAME.")
			return nil
		}
		return err
	}
	if err := validateResolveArgs(values); err != nil {
		return err
	}
	if command.dependencies.LoadOutput == nil {
		return fmt.Errorf("graph source loading is unavailable")
	}
	graph, err := command.dependencies.LoadOutput(values.Paths, values.MaxFiles)
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
	matches := resolveMatches(filtered.Declarations, values.Symbol, values.Paths)
	output := ResolveOutput{Schema: NavigationResolveSchema, Symbol: values.Symbol, Sources: graph.Sources, Matches: matches, Truncation: graph.Truncation}
	output.Metadata = resolveMetadata(values, graph, len(matches), command.activeScopeFlags)
	output.Metadata.Scope.Languages = normalizedScope(filter.Languages, "")
	if values.Compact {
		err = command.renderCompactResolve(output, values.MaxOutputBytes)
	} else {
		err = writeResolveJSON(command.stdout(), output)
	}
	if err != nil {
		return err
	}
	if len(matches) == 0 && command.dependencies.RequestExit != nil {
		command.dependencies.RequestExit(1)
	}
	return nil
}

func (command *command) stdout() io.Writer {
	if command.dependencies.Stdout != nil {
		return command.dependencies.Stdout
	}
	return os.Stdout
}
func (command *command) activeScopeFlags(parts []string) []string {
	if command.dependencies.ActiveScopeFlags == nil {
		return parts
	}
	return command.dependencies.ActiveScopeFlags(parts)
}

func validateResolveArgs(values resolveArgs) error {
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

func resolveMatches(declarations []parser.NavigationDeclaration, symbol string, paths []string) []ResolveMatch {
	matched := matchingDeclarations(declarations, symbol)
	sort.Slice(matched, func(left, right int) bool {
		if matched[left].Path != matched[right].Path {
			return matched[left].Path < matched[right].Path
		}
		if matched[left].Start != matched[right].Start {
			return matched[left].Start < matched[right].Start
		}
		return matched[left].ID < matched[right].ID
	})
	scope := resolveScope(paths)
	result := make([]ResolveMatch, 0, len(matched))
	for _, declaration := range matched {
		at := fmt.Sprintf("%s:%d", declaration.Path, declaration.Start)
		prefix := " --at " + quoteArgument(at) + " --depth 2 --compact " + scope
		result = append(result, ResolveMatch{ID: declaration.ID, Name: declaration.Name, Kind: declaration.Kind, Language: declaration.Language, Path: declaration.Path, StartLine: declaration.Start, EndLine: declaration.End, Visibility: declaration.Visibility, At: at, CallersCommand: "grepple graph callers" + prefix, CalleesCommand: "grepple graph callees" + prefix, ImpactCommand: "grepple graph impact" + prefix})
	}
	return result
}

func matchingDeclarations(declarations []parser.NavigationDeclaration, symbol string) []parser.NavigationDeclaration {
	exact := make([]parser.NavigationDeclaration, 0)
	terminal := make([]parser.NavigationDeclaration, 0)
	for _, declaration := range declarations {
		if declaration.Name == symbol {
			exact = append(exact, declaration)
		} else if terminalName(declaration.Name) == symbol {
			terminal = append(terminal, declaration)
		}
	}
	if len(exact) > 0 {
		return exact
	}
	return terminal
}
func terminalName(name string) string {
	if separator := strings.LastIndexAny(name, ".#:"); separator >= 0 {
		return name[separator+1:]
	}
	return name
}
func resolveScope(paths []string) string {
	if len(paths) == 0 {
		return "."
	}
	quoted := make([]string, len(paths))
	for index, path := range paths {
		quoted[index] = quoteArgument(path)
	}
	return strings.Join(quoted, " ")
}

func resolveMetadata(values resolveArgs, graph Output, returned int, scopeFlags func([]string) []string) *api.ResultMetadata {
	omitted := 0
	if graph.Truncation != nil {
		omitted = graph.Truncation.Skipped
	}
	metadata := &api.ResultMetadata{Scope: api.ResultScope{Mode: "local", Paths: normalizedScope(values.Paths, "."), ExcludedPaths: []string{}, Repositories: []string{}, ExcludedRepositories: []string{}, Languages: []string{}}, Order: "source", Page: api.ResultPage{Returned: returned, Complete: omitted == 0 && graph.Sources.Failed == 0 && graph.Sources.Recovered == 0}, Limits: api.ResultLimits{MaxFiles: values.MaxFiles, MaxOutputBytes: values.MaxOutputBytes, JSONByteUncapped: values.JSON}, Omitted: api.ResultOmissions{Sources: omitted}, Diagnostics: sourceDiagnostics(graph.Sources)}
	if graph.Truncation != nil {
		parts := scopeFlags([]string{"grepple", "graph", "resolve", "--symbol", quoteArgument(values.Symbol), "--max-files", "0", "--json"})
		for _, language := range values.Languages {
			parts = append(parts, "--language", quoteArgument(language))
		}
		for _, visibility := range values.Visibilities {
			parts = append(parts, "--visibility", quoteArgument(visibility))
		}
		for _, path := range normalizedScope(values.Paths, ".") {
			parts = append(parts, quoteArgument(path))
		}
		metadata.NextCommand = strings.Join(parts, " ")
	}
	return metadata
}

func normalizedScope(values []string, fallback string) []string {
	if len(values) == 0 {
		if fallback == "" {
			return []string{}
		}
		return []string{fallback}
	}
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}
func sourceDiagnostics(sources SourceSummary) []api.ResultDiagnostic {
	diagnostics := []api.ResultDiagnostic{}
	if sources.Failed > 0 {
		diagnostics = append(diagnostics, api.ResultDiagnostic{Code: "source-failed", Message: fmt.Sprintf("%d selected source files failed analysis", sources.Failed)})
	}
	if sources.Recovered > 0 {
		diagnostics = append(diagnostics, api.ResultDiagnostic{Code: "source-recovered", Message: fmt.Sprintf("%d source files required parser recovery", sources.Recovered)})
	}
	if sources.Skipped > 0 {
		diagnostics = append(diagnostics, api.ResultDiagnostic{Code: "source-skipped", Message: fmt.Sprintf("%d discovered source files were unsupported or binary", sources.Skipped)})
	}
	return diagnostics
}
func writeResolveJSON(writer io.Writer, output ResolveOutput) error {
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}

func (command *command) renderCompactResolve(output ResolveOutput, maxBytes int) error {
	writer := cliruntime.NewBoundedOutput(command.stdout(), maxBytes)
	write := func(value string) bool { return writer.WriteString(value) == nil }
	if !write(fmt.Sprintf("resolve %s symbol=%s matches=%d sources=%s\n", output.Schema, quoteArgument(output.Symbol), len(output.Matches), compactSourceSummary(output.Sources))) {
		return nil
	}
	if output.Truncation != nil && !write(fmt.Sprintf("! truncated %s limit=%d skipped=%d\n", output.Truncation.Reason, output.Truncation.Limit, output.Truncation.Skipped)) {
		return nil
	}
	if output.Metadata != nil && output.Metadata.NextCommand != "" && !write("continue: "+output.Metadata.NextCommand+"\n") {
		return nil
	}
	for _, match := range output.Matches {
		line := fmt.Sprintf("D %s %s %s %s %s visibility=%s\n  at: %s\n  callers: %s\n  callees: %s\n  impact: %s\n", shortID(match.ID), match.Language, match.Kind, match.Name, resolveLocation(match), match.Visibility, match.At, match.CallersCommand, match.CalleesCommand, match.ImpactCommand)
		if !write(line) {
			return nil
		}
	}
	return nil
}
func compactSourceSummary(summary SourceSummary) string {
	return fmt.Sprintf("discovered:%d,selected:%d,parsed:%d,skipped:%d,failed:%d,recovered:%d", summary.Discovered, summary.Selected, summary.Parsed, summary.Skipped, summary.Failed, summary.Recovered)
}
func shortID(id string) string {
	if len(id) <= 16 {
		return id
	}
	return id[:16]
}
func resolveLocation(match ResolveMatch) string {
	if match.StartLine == match.EndLine {
		return fmt.Sprintf("%s:%d", match.Path, match.StartLine)
	}
	return fmt.Sprintf("%s:%d-%d", match.Path, match.StartLine, match.EndLine)
}
func quoteArgument(value string) string {
	if value != "" && !strings.ContainsAny(value, " \t\r\n'\"") {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
