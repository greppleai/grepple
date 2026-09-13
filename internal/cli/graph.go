package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/alexflint/go-arg"
	"github.com/greppleai/grepple/parser"
	"github.com/greppleai/grepple/search"
)

const navigationGraphSchema = "grepple-navigation-graph-v1"

type graphArgs struct {
	JSON           bool     `arg:"--json" help:"emit the complete normalized navigation graph as JSON"`
	Compact        bool     `arg:"--compact" help:"emit a bounded agent-facing declaration and call summary"`
	MaxFiles       int      `arg:"--max-files" placeholder:"N" help:"parse at most N discovered files (0 = unlimited)"`
	MaxOutputBytes int      `arg:"--max-output-bytes" placeholder:"N" help:"cap compact output (default 16384; 0 = unlimited; JSON is uncapped)"`
	Paths          []string `arg:"positional" placeholder:"PATH" help:"file, directory, or glob to include; defaults to the working directory"`
}

func (graphArgs) Description() string {
	return "Build a deterministic local navigation graph; use the callers or callees subcommand for focused traversal."
}

type navigationGraphOutput struct {
	Schema       string                         `json:"schema"`
	Files        int                            `json:"files"`
	Declarations []parser.NavigationDeclaration `json:"declarations"`
	Calls        []parser.NavigationCall        `json:"calls"`
	Query        *navigationGraphQuery          `json:"query,omitempty"`
	Truncation   *navigationGraphTruncation     `json:"truncation,omitempty"`
}

type navigationGraphQuery struct {
	Direction   string   `json:"direction"`
	Depth       int      `json:"depth"`
	RootIDs     []string `json:"rootIds"`
	Languages   []string `json:"languages,omitempty"`
	Confidences []string `json:"confidences,omitempty"`
}

type navigationGraphTruncation struct {
	Reason  string `json:"reason"`
	Limit   int    `json:"limit"`
	Skipped int    `json:"skipped"`
}

func runGraph(args []string) error {
	if len(args) > 0 && isGraphQueryDirection(args[0]) {
		return runGraphQuery(search.NavigationQueryDirection(args[0]), args[1:])
	}
	values := graphArgs{MaxOutputBytes: DefaultTextOutputBytes}
	argumentParser, err := arg.NewParser(arg.Config{Program: "grepple graph"}, &values)
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
		return fmt.Errorf("grepple graph requires exactly one of --json or --compact")
	}
	if values.MaxFiles < 0 {
		return fmt.Errorf("--max-files must be non-negative")
	}
	if values.MaxOutputBytes < 0 {
		return fmt.Errorf("--max-output-bytes must be non-negative")
	}
	output, err := buildNavigationGraphOutput(values.Paths, values.MaxFiles)
	if err != nil {
		return err
	}
	if values.Compact {
		return renderCompactNavigationGraph(output, values.MaxOutputBytes)
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}

func renderCompactNavigationGraph(graph navigationGraphOutput, maxBytes int) error {
	output := stdoutWriter()
	if maxBytes > 0 {
		output = newBoundedOutputWriter(os.Stdout, maxBytes)
	}
	write := func(line string) bool {
		err := output.writeString(line + "\n")
		return err == nil
	}
	visibleCalls := compactNavigationCalls(graph.Calls)
	if !write(fmt.Sprintf("graph %s files=%d declarations=%d calls=%d", graph.Schema, graph.Files, len(graph.Declarations), len(visibleCalls))) {
		return nil
	}
	if graph.Query != nil && !write(compactGraphQueryLine(*graph.Query)) {
		return nil
	}
	if graph.Truncation != nil && !write(fmt.Sprintf("! truncated %s limit=%d skipped=%d", graph.Truncation.Reason, graph.Truncation.Limit, graph.Truncation.Skipped)) {
		return nil
	}
	declarations, callsByCaller := indexCompactNavigationGraph(graph.Declarations, visibleCalls)
	writeCompactNavigationDeclarations(write, graph.Declarations, declarations, callsByCaller)
	return nil
}
func compactGraphQueryLine(query navigationGraphQuery) string {
	parts := []string{fmt.Sprintf("query %s depth=%d roots=%s", query.Direction, query.Depth, shortGraphIDs(query.RootIDs))}
	if len(query.Languages) > 0 {
		parts = append(parts, "languages="+strings.Join(query.Languages, ","))
	}
	if len(query.Confidences) > 0 {
		parts = append(parts, "confidences="+strings.Join(query.Confidences, ","))
	}
	return strings.Join(parts, " ")
}

func indexCompactNavigationGraph(declarationList []parser.NavigationDeclaration, calls []parser.NavigationCall) (map[string]parser.NavigationDeclaration, map[string][]parser.NavigationCall) {
	declarations := make(map[string]parser.NavigationDeclaration, len(declarationList))
	callsByCaller := make(map[string][]parser.NavigationCall)
	for _, declaration := range declarationList {
		declarations[declaration.ID] = declaration
	}
	for _, call := range calls {
		callsByCaller[call.CallerID] = append(callsByCaller[call.CallerID], call)
	}
	return declarations, callsByCaller
}

func writeCompactNavigationDeclarations(write func(string) bool, declarationList []parser.NavigationDeclaration, declarations map[string]parser.NavigationDeclaration, callsByCaller map[string][]parser.NavigationCall) bool {
	for _, declaration := range declarationList {
		line := fmt.Sprintf("D %s %s %s %s %s", shortGraphID(declaration.ID), declaration.Language, declaration.Kind, declaration.Name, graphDeclarationLocation(declaration))
		if !write(line) {
			return false
		}
		for _, call := range callsByCaller[declaration.ID] {
			if !write(compactCallLine(call, declaration.Name, declarations)) {
				return false
			}
		}
	}
	return true
}

func compactNavigationCalls(calls []parser.NavigationCall) []parser.NavigationCall {
	visible := make([]parser.NavigationCall, 0, len(calls))
	for _, call := range calls {
		if call.TargetID != "" || len(call.CandidateTargetIDs) > 0 {
			visible = append(visible, call)
		}
	}
	return visible
}

func compactCallLine(call parser.NavigationCall, callerName string, declarations map[string]parser.NavigationDeclaration) string {
	return fmt.Sprintf("C %s %s -> %s [%s] %s:%d", shortGraphID(call.ID), callerName, compactCallTarget(call, declarations), call.Confidence, call.Path, call.Line)
}

func shortGraphID(id string) string {
	const length = 16
	if len(id) <= length {
		return id
	}
	return id[:length]
}

func shortGraphIDs(ids []string) string {
	const maxIDs = 3
	count := len(ids)
	if count > maxIDs {
		count = maxIDs
	}
	short := make([]string, 0, count+1)
	for _, id := range ids[:count] {
		short = append(short, shortGraphID(id))
	}
	if omitted := len(ids) - count; omitted > 0 {
		short = append(short, fmt.Sprintf("+%d", omitted))
	}
	return strings.Join(short, ",")
}

func graphDeclarationLocation(declaration parser.NavigationDeclaration) string {
	if declaration.Start == declaration.End {
		return fmt.Sprintf("%s:%d", declaration.Path, declaration.Start)
	}
	return fmt.Sprintf("%s:%d-%d", declaration.Path, declaration.Start, declaration.End)
}

func compactCallTarget(call parser.NavigationCall, declarations map[string]parser.NavigationDeclaration) string {
	if target, ok := declarations[call.TargetID]; ok {
		return target.Name + "#" + shortGraphID(target.ID)
	}
	if len(call.CandidateTargetIDs) == 0 {
		if call.Display != "" {
			return call.Display
		}
		return call.Name
	}
	const maxCandidates = 3
	count := len(call.CandidateTargetIDs)
	if count > maxCandidates {
		count = maxCandidates
	}
	candidates := make([]string, 0, count+1)
	for _, id := range call.CandidateTargetIDs[:count] {
		target := declarations[id]
		candidates = append(candidates, target.Name+"#"+shortGraphID(id))
	}
	if omitted := len(call.CandidateTargetIDs) - count; omitted > 0 {
		candidates = append(candidates, fmt.Sprintf("+%d", omitted))
	}
	return "? " + strings.Join(candidates, ",")
}

func navigationSourcePaths(paths []string) []string {
	sources := make([]string, 0, len(paths))
	for _, path := range paths {
		language := parser.LanguageFor(path)
		capabilities, ok := parser.CapabilitiesForLanguage(language)
		if ok && capabilities.Navigation {
			sources = append(sources, path)
		}
	}
	return sources
}
