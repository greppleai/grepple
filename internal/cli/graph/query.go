package graph

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/alexflint/go-arg"
	"github.com/greppleai/grepple/internal/analysis"
	cliruntime "github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/daemon"
	"github.com/greppleai/grepple/internal/search"
	"github.com/greppleai/grepple/internal/shellquote"
	"github.com/greppleai/grepple/internal/wire"
)

const maxNavigationQueryDepth = 10

type graphQueryArgs struct {
	JSON bool `arg:"--json" help:"emit the complete queried subgraph as JSON"`
	cliruntime.CommonArgs
	Repository     string   `arg:"--repo" placeholder:"OWNER/REPO[@REF]" help:"analyze one exact indexed repository"`
	Symbol         string   `arg:"--symbol" placeholder:"NAME" help:"select one exact declaration name"`
	At             string   `arg:"--at" placeholder:"PATH:LINE" help:"select the declaration containing a source location"`
	Package        string   `arg:"--package" placeholder:"NAME" help:"select every declaration in an exact package name or ID"`
	Module         string   `arg:"--module" placeholder:"ID" help:"select every declaration in an exact module ID"`
	RootPath       string   `arg:"--root-path" placeholder:"PATH" help:"select declarations at or below a repository-relative path"`
	Languages      []string `arg:"--language,separate" placeholder:"ID" help:"retain one navigation language; repeatable"`
	Confidences    []string `arg:"--confidence,separate" placeholder:"LEVEL" help:"retain one edge confidence; repeatable"`
	Visibilities   []string `arg:"--visibility,separate" placeholder:"LEVEL" help:"retain public, non-public, or unknown declarations; repeatable"`
	Depth          int      `arg:"--depth" default:"1" placeholder:"N" help:"maximum traversal depth (1-10)"`
	MaxFiles       int      `arg:"--max-files" placeholder:"N" help:"parse at most N discovered files (0 = unlimited)"`
	MaxOutputBytes int      `arg:"--max-output-bytes" default:"16384" placeholder:"N" help:"cap human-readable output (default 16384; 0 = unlimited; JSON is uncapped)"`
	Paths          []string `arg:"positional" placeholder:"PATH" help:"file, directory, or glob to include; defaults to the working directory"`
}

func (graphQueryArgs) Description() string {
	return "Query a deterministic local or exact indexed-repository navigation graph. Human output is the default; --json emits the complete subgraph. Exactly one root selector is required."
}

// runQuery traverses a navigation graph.
func runQuery(application cliruntime.Context, direction search.NavigationQueryDirection, args []string) error {
	values, help, err := parseGraphQueryArgs(application, direction, args)
	if err != nil || help {
		return err
	}
	return executeQuery(application, direction, &values)
}

func executeQuery(application cliruntime.Context, direction search.NavigationQueryDirection, values *QueryArgs) error {
	return executeQueryWithDaemon(application, direction, values, false)
}

func executeQueryWithDaemon(application cliruntime.Context, direction search.NavigationQueryDirection, values *QueryArgs, useDaemon bool) error {
	if err := validateGraphQueryArgs(*values); err != nil {
		return err
	}
	if values.Repository != "" {
		return runRemoteGraphQuery(application, context.Background(), direction, *values)
	}
	paths, err := ResolveInputPaths(values.Paths, search.SourcePolicyConfigurer(application.Repository()))
	if err != nil {
		return err
	}
	query := analysis.GraphQuery{
		Direction: string(direction), Depth: values.Depth,
		Symbol: values.Symbol, At: values.At, Package: values.Package, Module: values.Module, RootPath: values.RootPath,
		Languages: values.Languages, Confidences: values.Confidences, Visibilities: values.Visibilities,
	}
	var report analysis.GraphReport
	if useDaemon {
		if cached, ok := daemon.QueryGraph(paths, values.MaxFiles, query); ok {
			report = cached
		}
	}
	if report.Schema == "" {
		sources := analysis.ReadSources(paths)
		key := ""
		if useDaemon {
			key, _ = daemon.KeyGraph(paths, values.MaxFiles, sources, query)
		}
		universe, err := analysis.NewUniverse(sources, values.MaxFiles)
		if err != nil {
			return err
		}
		defer universe.Close()
		report, err = analysis.BuildGraph(universe, &query)
		if err != nil {
			return err
		}
		if key != "" {
			_ = daemon.StoreGraph(paths, values.MaxFiles, key, query, report)
		}
	}
	output := FromAnalysis(report)
	output.Metadata = graphResultMetadata(metadataInput{Paths: values.Paths, Returned: len(output.Declarations), MaxFiles: values.MaxFiles, MaxOutputBytes: values.MaxOutputBytes, JSON: values.JSON, Sources: output.Sources, Truncation: output.Truncation, NextCommand: graphQueryContinuationCommand(application, direction, *values, output.Truncation)})
	output.Metadata.Scope.Languages = normalizedScope(output.Query.Languages, "")
	if !values.JSON {
		return renderCompactNavigationGraph(application.Stdout(), output, values.MaxOutputBytes)
	}
	encoder := json.NewEncoder(application.Stdout())
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}
func parseGraphQueryArgs(application cliruntime.Context, direction search.NavigationQueryDirection, args []string) (graphQueryArgs, bool, error) {
	values := graphQueryArgs{MaxOutputBytes: defaultTextOutputBytes, Depth: 1}
	argumentParser, err := arg.NewParser(arg.Config{Program: "grepple graph " + string(direction)}, &values)
	if err != nil {
		return values, false, err
	}
	if err := argumentParser.Parse(args); err != nil {
		if errors.Is(err, arg.ErrHelp) {
			argumentParser.WriteHelp(application.Stdout())
			fmt.Fprintln(application.Stdout(), "Required root selector: choose exactly one of --symbol, --at, --package, --module, or --root-path.")
			return values, true, nil
		}
		return values, false, err
	}
	if err := validateGraphQueryArgs(values); err != nil {
		return values, false, err
	}
	return values, false, nil
}

func runRemoteGraphQuery(application cliruntime.Context, ctx context.Context, direction search.NavigationQueryDirection, values graphQueryArgs) error {
	request := wire.AnalysisRequest{
		Operation: wire.AnalysisGraph, Repository: values.Repository, Paths: values.Paths, MaxFiles: values.MaxFiles,
		Graph: &wire.GraphQueryRequest{Direction: string(direction), Depth: values.Depth, Symbol: values.Symbol, At: values.At, Package: values.Package, Module: values.Module, RootPath: values.RootPath, Languages: values.Languages, Confidences: values.Confidences, Visibilities: values.Visibilities},
	}
	response, err := requestRemoteAnalysis(application, ctx, request, application.Configuration().ServerDefault(values.Server))
	if err != nil {
		return err
	}
	if values.JSON {
		return cliruntime.NewOutput(application.Stdout()).WriteJSON(response)
	}
	var output navigationGraphOutput
	if err := json.Unmarshal(response.Result, &output); err != nil {
		return fmt.Errorf("decode remote graph query: %w", err)
	}
	return renderCompactNavigationGraph(application.Stdout(), output, values.MaxOutputBytes)
}

func graphQueryContinuationCommand(application cliruntime.Context, direction search.NavigationQueryDirection, values graphQueryArgs, truncation *navigationGraphTruncation) string {
	if truncation == nil {
		return ""
	}
	parts := []string{"grepple", "graph", string(direction), "--max-files", "0", "--depth", fmt.Sprint(values.Depth), "--json"}
	parts = application.Repository().AppendScopeFlags(parts)
	for _, selector := range []struct{ flag, value string }{{"--symbol", values.Symbol}, {"--at", values.At}, {"--package", values.Package}, {"--module", values.Module}, {"--root-path", values.RootPath}} {
		if selector.value != "" {
			parts = append(parts, selector.flag, shellquote.Argument(selector.value))
		}
	}
	for _, language := range values.Languages {
		parts = append(parts, "--language", shellquote.Argument(language))
	}
	for _, confidence := range values.Confidences {
		parts = append(parts, "--confidence", shellquote.Argument(confidence))
	}
	for _, visibility := range values.Visibilities {
		parts = append(parts, "--visibility", shellquote.Argument(visibility))
	}
	for _, path := range normalizedScope(values.Paths, ".") {
		parts = append(parts, shellquote.Argument(path))
	}
	return strings.Join(parts, " ")
}

func validateGraphQueryArgs(values graphQueryArgs) error {
	if graphQuerySelectorCount(values) != 1 {
		return fmt.Errorf("grepple graph query requires exactly one of --symbol, --at, --package, --module, or --root-path")
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

func graphQuerySelectorCount(values graphQueryArgs) int {
	count := 0
	for _, value := range []string{values.Symbol, values.At, values.Package, values.Module, values.RootPath} {
		if value != "" {
			count++
		}
	}
	return count
}

func buildNavigationGraphOutput(application cliruntime.Context, globs []string, maxFiles int) (navigationGraphOutput, error) {
	paths, err := ResolveInputPaths(globs, search.SourcePolicyConfigurer(application.Repository()))
	if err != nil {
		return navigationGraphOutput{}, err
	}
	return BuildFromPaths(paths, maxFiles), nil
}

// ResolveInputPaths applies repository source policy and resolves local graph paths.
func ResolveInputPaths(globs []string, applySourceConfig func(*search.Params) error) ([]string, error) {
	params := search.Params{Files: true, Globs: globs}
	if applySourceConfig != nil {
		if err := applySourceConfig(&params); err != nil {
			return nil, err
		}
	}
	paths, err := search.ListFilePathsContext(context.Background(), params, nil)
	if err != nil {
		return nil, err
	}
	filtered := paths[:0]
	for _, path := range paths {
		slashPath := "/" + strings.TrimPrefix(filepath.ToSlash(filepath.Clean(path)), "./")
		if strings.Contains(slashPath, "/.grepple/cache/") {
			continue
		}
		filtered = append(filtered, path)
	}
	return filtered, nil
}

// BuildFromPaths builds a graph projection from resolved paths.
func BuildFromPaths(paths []string, maxFiles int) Output {
	return BuildFromPathsWithOptions(paths, maxFiles, search.NavigationBuildOptions{})
}

// BuildFromPathsWithOptions builds a graph projection from resolved paths and options.
func BuildFromPathsWithOptions(paths []string, maxFiles int, options search.NavigationBuildOptions) Output {
	return BuildOutput(paths, maxFiles, options)
}
