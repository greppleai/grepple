package graph

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/alexflint/go-arg"
	"github.com/greppleai/grepple/internal/analysis"
	cliruntime "github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/daemon"
	"github.com/greppleai/grepple/internal/navigation"
	"github.com/greppleai/grepple/internal/parser"
	"github.com/greppleai/grepple/internal/search"
	"github.com/greppleai/grepple/internal/wire"
)

// NavigationResolveSchema identifies graph symbol-resolution responses.
const NavigationResolveSchema = "grepple-navigation-resolve-v1"
const defaultTextOutputBytes = 16 * 1024

type resolveArgs struct {
	JSON           bool     `arg:"--json" help:"emit every matching declaration and continuation command as JSON"`
	Symbol         string   `arg:"--symbol,required" placeholder:"NAME" help:"preview exact or terminal-name declaration matches"`
	Languages      []string `arg:"--language,separate" placeholder:"ID" help:"retain one navigation language; repeatable"`
	Visibilities   []string `arg:"--visibility,separate" placeholder:"LEVEL" help:"retain public, non-public, or unknown declarations; repeatable"`
	MaxFiles       int      `arg:"--max-files" placeholder:"N" help:"parse at most N discovered files (0 = unlimited)"`
	MaxOutputBytes int      `arg:"--max-output-bytes" default:"16384" placeholder:"N" help:"cap human-readable output (default 16384; 0 = unlimited; JSON is uncapped)"`
	Paths          []string `arg:"positional" placeholder:"PATH" help:"graph universe; defaults to the working directory"`
}

func (resolveArgs) Description() string {
	return "Preview every declaration matching a symbol without graph traversal. Human output is the default; --json emits every match."
}

// ResolveOutput is the complete graph symbol-resolution response.
type ResolveOutput struct {
	Schema     string               `json:"schema"`
	Metadata   *wire.ResultMetadata `json:"metadata,omitempty"`
	Symbol     string               `json:"symbol"`
	Sources    SourceSummary        `json:"sources"`
	Matches    []ResolveMatch       `json:"matches"`
	Truncation *Truncation          `json:"truncation,omitempty"`
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
}

func (command *command) runResolve(args []string) error {
	values := resolveArgs{MaxOutputBytes: defaultTextOutputBytes}
	argumentParser, err := arg.NewParser(arg.Config{Program: "grepple graph resolve"}, &values)
	if err != nil {
		return err
	}
	if err := argumentParser.Parse(args); err != nil {
		if errors.Is(err, arg.ErrHelp) {
			argumentParser.WriteHelp(command.context.Stdout())
			fmt.Fprintln(command.context.Stdout(), "Required selector: --symbol NAME.")
			return nil
		}
		return err
	}
	return executeResolve(command.context, &values)
}

func executeResolve(application cliruntime.Context, values *ResolveArgs) error {
	return executeResolveWithDaemon(application, values, false)
}

func executeResolveWithDaemon(application cliruntime.Context, values *ResolveArgs, useDaemon bool) error {
	if err := validateResolveArgs(*values); err != nil {
		return err
	}
	filter, err := navigation.NewGraphOperations().NormalizeFilter(navigation.NavigationGraphFilter{Languages: values.Languages, Visibilities: values.Visibilities})
	if err != nil {
		return err
	}
	paths, err := ResolveInputPaths(values.Paths, search.SourcePolicyConfigurer(application.Repository()))
	if err != nil {
		return err
	}
	selection := daemon.ResolveSelection{Symbol: values.Symbol, Languages: filter.Languages, Visibilities: filter.Visibilities}
	var projection daemon.ResolveProjection
	if useDaemon {
		projection, _ = daemon.QueryResolve(paths, values.MaxFiles, selection)
	}
	if projection.Schema == "" {
		sources := analysis.ReadSources(paths)
		key := ""
		if useDaemon {
			key, _ = daemon.KeyResolve(paths, values.MaxFiles, sources, selection)
		}
		universe, err := analysis.NewUniverse(sources, values.MaxFiles)
		if err != nil {
			return err
		}
		defer universe.Close()
		report, err := analysis.BuildGraph(universe, nil)
		if err != nil {
			return err
		}
		filtered, err := navigation.NewGraphOperations().Filter(parser.NavigationGraph{Declarations: report.Declarations}, filter)
		if err != nil {
			return err
		}
		projection = daemon.ResolveProjection{Schema: daemon.ResolveProjectionSchema, Sources: report.Sources, Truncation: report.Truncation, Declarations: matchingDeclarations(filtered.Declarations, values.Symbol)}
		if key != "" {
			_ = daemon.StoreResolve(paths, values.MaxFiles, key, selection, projection)
		}
	}
	matches := resolveMatches(projection.Declarations, values.Symbol, values.Paths)
	output := ResolveOutput{Schema: NavigationResolveSchema, Symbol: values.Symbol, Sources: projection.Sources, Matches: matches, Truncation: projection.Truncation}
	output.Metadata = resolveMetadata(*values, Output{Sources: projection.Sources, Truncation: projection.Truncation}, len(matches), application.Repository().AppendScopeFlags)
	output.Metadata.Scope.Languages = normalizedScope(filter.Languages, "")
	if !values.JSON {
		err = renderCompactResolve(application.Stdout(), output, values.MaxOutputBytes)
	} else {
		err = writeResolveJSON(application.Stdout(), output)
	}
	if err != nil {
		return err
	}
	if len(matches) == 0 {
		application.RequestExit(1)
	}
	return nil
}

func validateResolveArgs(values resolveArgs) error {
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
		prefix := " --at " + quoteArgument(at) + " --depth 2 " + scope
		result = append(result, ResolveMatch{ID: declaration.ID, Name: declaration.Name, Kind: declaration.Kind, Language: declaration.Language, Path: declaration.Path, StartLine: declaration.Start, EndLine: declaration.End, Visibility: declaration.Visibility, At: at, CallersCommand: "grepple graph callers" + prefix, CalleesCommand: "grepple graph callees" + prefix})
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

func resolveMetadata(values resolveArgs, graph Output, returned int, scopeFlags func([]string) []string) *wire.ResultMetadata {
	omitted := 0
	if graph.Truncation != nil {
		omitted = graph.Truncation.Skipped
	}
	metadata := &wire.ResultMetadata{Scope: wire.ResultScope{Mode: "local", Paths: normalizedScope(values.Paths, "."), ExcludedPaths: []string{}, Repositories: []string{}, ExcludedRepositories: []string{}, Languages: []string{}}, Order: "source", Page: wire.ResultPage{Returned: returned, Complete: omitted == 0 && graph.Sources.Failed == 0 && graph.Sources.Recovered == 0}, Limits: wire.ResultLimits{MaxFiles: values.MaxFiles, MaxOutputBytes: values.MaxOutputBytes, JSONByteUncapped: values.JSON}, Omitted: wire.ResultOmissions{Sources: omitted}, Diagnostics: sourceDiagnostics(graph.Sources)}
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
func sourceDiagnostics(sources SourceSummary) []wire.ResultDiagnostic {
	diagnostics := []wire.ResultDiagnostic{}
	if sources.Failed > 0 {
		diagnostics = append(diagnostics, wire.ResultDiagnostic{Code: "source-failed", Message: fmt.Sprintf("%d selected source files failed analysis", sources.Failed)})
	}
	if sources.Recovered > 0 {
		diagnostics = append(diagnostics, wire.ResultDiagnostic{Code: "source-recovered", Message: fmt.Sprintf("%d source files required parser recovery", sources.Recovered)})
	}
	if sources.Skipped > 0 {
		diagnostics = append(diagnostics, wire.ResultDiagnostic{Code: "source-skipped", Message: fmt.Sprintf("%d discovered source files were unsupported or binary", sources.Skipped)})
	}
	return diagnostics
}
func writeResolveJSON(writer io.Writer, output ResolveOutput) error {
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}

func renderCompactResolve(destination io.Writer, output ResolveOutput, maxBytes int) error {
	writer := cliruntime.NewBoundedOutput(destination, maxBytes)
	write := func(value string) bool { return writer.WriteString(value) == nil }
	if len(output.Matches) == 0 && !write("no matches for "+quoteArgument(output.Symbol)+"\n") {
		return nil
	}
	if output.Sources.Failed > 0 || output.Sources.Recovered > 0 {
		if !write("! incomplete sources=" + compactSourceSummary(output.Sources) + "; inspect --json diagnostics\n") {
			return nil
		}
	}
	if output.Truncation != nil && !write(fmt.Sprintf("! truncated %s limit=%d skipped=%d\n", output.Truncation.Reason, output.Truncation.Limit, output.Truncation.Skipped)) {
		return nil
	}
	if output.Metadata != nil && output.Metadata.NextCommand != "" && !write("continue: "+output.Metadata.NextCommand+"\n") {
		return nil
	}
	for _, match := range output.Matches {
		line := fmt.Sprintf("D %s %s %s %s %s visibility=%s\n  at: %s\n  callers: %s\n  callees: %s\n", shortID(match.ID), match.Language, match.Kind, match.Name, resolveLocation(match), match.Visibility, match.At, match.CallersCommand, match.CalleesCommand)
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
