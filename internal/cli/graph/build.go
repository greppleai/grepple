package graph

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/alexflint/go-arg"
	"github.com/greppleai/grepple/internal/wire"
	cliruntime "github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/shellquote"
	"github.com/greppleai/grepple/internal/parser"
)

type graphArgs struct {
	JSON bool `arg:"--json" help:"emit the complete normalized navigation graph as JSON"`
	cliruntime.CommonArgs
	Repository     string   `arg:"--repo" placeholder:"OWNER/REPO[@REF]" help:"analyze one exact indexed repository"`
	MaxFiles       int      `arg:"--max-files" placeholder:"N" help:"parse at most N discovered files (0 = unlimited)"`
	MaxOutputBytes int      `arg:"--max-output-bytes" default:"16384" placeholder:"N" help:"cap human-readable output (default 16384; 0 = unlimited; JSON is uncapped)"`
	Paths          []string `arg:"positional" placeholder:"PATH" help:"file, directory, or glob to include; defaults to the working directory"`
}

func (graphArgs) Description() string {
	return "Build a deterministic local or exact indexed-repository navigation graph. Use resolve to preview symbol alternatives, callers/callees/impact for traversal, or diff for comparison. Human output is the default; --json emits the complete graph."
}

// Parent aliases preserve integrations while graph projection ownership moves to the command package.
type navigationGraphOutput = Output
type navigationGraphQuery = Query
type navigationGraphTruncation = Truncation
type navigationSourceSummary = SourceSummary

// runBuild builds a navigation graph.
func runBuild(application cliruntime.Context, args []string) error {
	values := graphArgs{MaxOutputBytes: defaultTextOutputBytes}
	argumentParser, err := arg.NewParser(arg.Config{Program: "grepple graph"}, &values)
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
	return executeBuild(application, &values)
}

func executeBuild(application cliruntime.Context, values *BuildArgs) error {
	if values.MaxFiles < 0 {
		return fmt.Errorf("--max-files must be non-negative")
	}
	if values.MaxOutputBytes < 0 {
		return fmt.Errorf("--max-output-bytes must be non-negative")
	}
	output, remote, err := loadGraphCommandOutput(application, *values)
	if err != nil {
		return err
	}
	if values.JSON && remote != nil {
		return cliruntime.NewOutput(application.Stdout()).WriteJSON(remote)
	}
	if !values.JSON {
		return renderCompactNavigationGraph(application.Stdout(), output, values.MaxOutputBytes)
	}
	encoder := json.NewEncoder(application.Stdout())
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}

func loadGraphCommandOutput(application cliruntime.Context, values graphArgs) (navigationGraphOutput, *wire.AnalysisResponse, error) {
	if values.Repository != "" {
		response, err := requestRemoteAnalysis(application, context.Background(), wire.AnalysisRequest{Operation: wire.AnalysisGraph, Repository: values.Repository, Paths: values.Paths, MaxFiles: values.MaxFiles}, application.Configuration().ServerDefault(values.Server))
		if err != nil {
			return navigationGraphOutput{}, nil, err
		}
		var output navigationGraphOutput
		if err := json.Unmarshal(response.Result, &output); err != nil {
			return navigationGraphOutput{}, nil, fmt.Errorf("decode remote graph: %w", err)
		}
		return output, &response, nil
	}
	output, err := buildNavigationGraphOutput(application, values.Paths, values.MaxFiles)
	if err != nil {
		return navigationGraphOutput{}, nil, err
	}
	output.Metadata = graphResultMetadata(metadataInput{Paths: values.Paths, Returned: len(output.Declarations), MaxFiles: values.MaxFiles, MaxOutputBytes: values.MaxOutputBytes, JSON: values.JSON, Sources: output.Sources, Truncation: output.Truncation, NextCommand: graphContinuationCommand(application, "graph", values.Paths, output.Truncation)})
	return output, nil, nil
}
func graphContinuationCommand(application cliruntime.Context, mode string, paths []string, truncation *Truncation) string {
	if truncation == nil {
		return ""
	}
	parts := []string{"grepple", "graph"}
	parts = application.Repository().AppendScopeFlags(parts)
	if mode != "graph" {
		parts = append(parts, mode)
	}
	parts = append(parts, "--max-files", "0", "--json")
	if len(paths) == 0 {
		paths = []string{"."}
	}
	for _, path := range paths {
		parts = append(parts, shellquote.Argument(path))
	}
	return strings.Join(parts, " ")
}

func renderCompactNavigationGraph(destination io.Writer, graph navigationGraphOutput, maxBytes int) error {
	output := cliruntime.NewOutput(destination)
	if maxBytes > 0 {
		output = cliruntime.NewBoundedOutput(destination, maxBytes)
	}
	write := func(line string) bool {
		err := output.WriteString(line + "\n")
		return err == nil
	}
	visibleCalls := compactNavigationCalls(graph.Calls)
	if graph.Query == nil {
		summary := fmt.Sprintf("graph files=%d declarations=%d calls=%d", graph.Files, len(graph.Declarations), len(graph.Calls))
		if len(visibleCalls) != len(graph.Calls) {
			summary += fmt.Sprintf(" shown=%d", len(visibleCalls))
		}
		if !write(summary) {
			return nil
		}
	} else if graph.Query.Direction != "callers" && graph.Query.Direction != "callees" && graph.Query.Direction != "impact" && !write(compactGraphQueryLine(*graph.Query)) {
		return nil
	}
	if graph.Sources.Failed > 0 || graph.Sources.Recovered > 0 {
		if !write("! incomplete sources=" + compactNavigationSourceSummary(graph.Sources) + "; inspect --json diagnostics") {
			return nil
		}
	}
	if graph.Truncation != nil && !write(fmt.Sprintf("! truncated %s limit=%d skipped=%d", graph.Truncation.Reason, graph.Truncation.Limit, graph.Truncation.Skipped)) {
		return nil
	}
	if graph.Metadata != nil && graph.Metadata.NextCommand != "" && !write("continue: "+graph.Metadata.NextCommand) {
		return nil
	}
	if graph.Query != nil && (graph.Query.Direction == "callers" || graph.Query.Direction == "callees" || graph.Query.Direction == "impact") {
		writeCompactDirectionalQuery(write, *graph.Query, graph.Declarations, visibleCalls)
		return nil
	}
	if graph.Query == nil {
		writeCompactBuildGraph(write, graph.Declarations, visibleCalls)
		return nil
	}
	declarations, callsByCaller := indexCompactNavigationGraph(graph.Declarations, visibleCalls)
	if !writeCompactNavigationDeclarations(write, graph.Declarations, declarations, callsByCaller) {
		return nil
	}
	return nil
}

func compactNavigationSourceSummary(summary navigationSourceSummary) string {
	return fmt.Sprintf("discovered:%d,selected:%d,parsed:%d,skipped:%d,failed:%d,recovered:%d", summary.Discovered, summary.Selected, summary.Parsed, summary.Skipped, summary.Failed, summary.Recovered)
}
func compactGraphQueryLine(query navigationGraphQuery) string {
	parts := []string{fmt.Sprintf("query %s depth=%d roots=%s", query.Direction, query.Depth, shortGraphIDs(query.RootIDs))}
	if len(query.Languages) > 0 {
		parts = append(parts, "languages="+strings.Join(query.Languages, ","))
	}
	if len(query.Confidences) > 0 {
		parts = append(parts, "confidences="+strings.Join(query.Confidences, ","))
	}
	if len(query.Visibilities) > 0 {
		parts = append(parts, "visibilities="+strings.Join(query.Visibilities, ","))
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
		line := fmt.Sprintf("D %s %s %s %s %s %s visibility=%s", shortGraphID(declaration.ID), declaration.Language, declaration.Kind, declaration.Name, graphDeclarationLocation(declaration), compactEntrypoint(declaration.Entrypoint), declaration.Visibility)
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

func compactEntrypoint(entrypoint string) string {
	if entrypoint == "" {
		return "entrypoint=none"
	}
	return "entrypoint=" + entrypoint
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

func navigationSourcePaths(paths []string) []string { return SourcePaths(paths) }

// ShortID returns the compact stable suffix used in graph output.
func ShortID(id string) string { return shortGraphID(id) }
