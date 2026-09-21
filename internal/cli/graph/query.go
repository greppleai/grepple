package graph

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/alexflint/go-arg"
	"github.com/greppleai/grepple/api"
	codeextract "github.com/greppleai/grepple/internal/cli/extract"
	cliruntime "github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/shellquote"
	"github.com/greppleai/grepple/parser"
	"github.com/greppleai/grepple/search"
)

const maxNavigationQueryDepth = 10

type graphQueryArgs struct {
	JSON    bool `arg:"--json" help:"emit the complete queried subgraph as JSON"`
	Compact bool `arg:"--compact" help:"emit a bounded agent-facing queried subgraph"`
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
	MaxOutputBytes int      `arg:"--max-output-bytes" placeholder:"N" help:"cap compact output (default 16384; 0 = unlimited; JSON is uncapped)"`
	Paths          []string `arg:"positional" placeholder:"PATH" help:"file, directory, or glob to include; defaults to the working directory"`
}

func (graphQueryArgs) Description() string {
	return "Query a deterministic local or exact indexed-repository navigation graph. Exactly one of --json or --compact and exactly one root selector are required."
}

func graphQuerySemantics(direction search.NavigationQueryDirection) string {
	switch direction {
	case search.NavigationQueryDependencies:
		return "Navigation semantics: dependencies traverse outgoing call/navigation edges; they are not build-system or package-manager dependencies."
	case search.NavigationQueryDependents:
		return "Navigation semantics: dependents traverse incoming call/navigation edges; they are not build-system or package-manager dependents."
	default:
		return ""
	}
}

// RunQuery traverses a navigation graph.
func RunQuery(direction search.NavigationQueryDirection, args []string, services Services) error {
	values, help, err := parseGraphQueryArgs(direction, args)
	if err != nil || help {
		return err
	}
	if values.Repository != "" {
		return runRemoteGraphQuery(context.Background(), direction, values, services)
	}
	output, err := buildNavigationGraphOutput(values.Paths, values.MaxFiles, services)
	if err != nil {
		return err
	}
	filter, err := search.NormalizeNavigationGraphFilter(search.NavigationGraphFilter{Languages: values.Languages, Confidences: values.Confidences, Visibilities: values.Visibilities})
	if err != nil {
		return err
	}
	filtered, err := search.FilterNavigationGraph(parser.NavigationGraph{Declarations: output.Declarations, TypeDeclarations: output.TypeDeclarations, Calls: output.Calls, Imports: output.Imports, Exports: output.Exports, Fields: output.Fields, TypeUsages: output.TypeUsages, MemberAccesses: output.MemberAccesses, RepositoryRoots: output.RepositoryRoots}, filter)
	if err != nil {
		return err
	}
	output.Declarations = filtered.Declarations
	output.TypeDeclarations = filtered.TypeDeclarations
	output.Calls = filtered.Calls
	output.Imports = filtered.Imports
	output.RepositoryRoots = filtered.RepositoryRoots
	output.Exports = filtered.Exports
	output.Fields = filtered.Fields
	output.TypeUsages = filtered.TypeUsages
	output.MemberAccesses = filtered.MemberAccesses
	roots, err := selectNavigationQueryRoots(output.Declarations, values)
	if err != nil {
		return err
	}
	rootIDs := navigationDeclarationIDs(roots)
	queried, err := search.QueryNavigationGraph(parser.NavigationGraph{Declarations: output.Declarations, TypeDeclarations: output.TypeDeclarations, Calls: output.Calls, Imports: output.Imports, Exports: output.Exports, Fields: output.Fields, TypeUsages: output.TypeUsages, MemberAccesses: output.MemberAccesses, RepositoryRoots: output.RepositoryRoots}, rootIDs, direction, values.Depth)
	if err != nil {
		return err
	}
	output.Declarations = queried.Declarations
	output.TypeDeclarations = queried.TypeDeclarations
	output.Calls = queried.Calls
	output.Imports = queried.Imports
	output.RepositoryRoots = queried.RepositoryRoots
	output.Exports = queried.Exports
	output.Fields = queried.Fields
	output.TypeUsages = queried.TypeUsages
	output.MemberAccesses = queried.MemberAccesses
	output.Resolution = search.MeasureNavigationResolution(queried)
	output.Query = &navigationGraphQuery{
		Direction: string(direction), Depth: values.Depth, RootIDs: rootIDs,
		Languages: filter.Languages, Confidences: filter.Confidences, Visibilities: filter.Visibilities,
	}
	if services.Metadata != nil {
		output.Metadata = services.Metadata(MetadataInput{Paths: values.Paths, Returned: len(output.Declarations), MaxFiles: values.MaxFiles, MaxOutputBytes: values.MaxOutputBytes, JSON: values.JSON, Sources: output.Sources, Truncation: output.Truncation, NextCommand: graphQueryContinuationCommand(direction, values, output.Truncation, services)})
		output.Metadata.Scope.Languages = normalizedScope(filter.Languages, "")
	}
	if values.Compact {
		return renderCompactNavigationGraph(output, values.MaxOutputBytes)
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}
func parseGraphQueryArgs(direction search.NavigationQueryDirection, args []string) (graphQueryArgs, bool, error) {
	values := graphQueryArgs{MaxOutputBytes: defaultTextOutputBytes, Depth: 1}
	argumentParser, err := arg.NewParser(arg.Config{Program: "grepple graph " + string(direction)}, &values)
	if err != nil {
		return values, false, err
	}
	if err := argumentParser.Parse(args); err != nil {
		if errors.Is(err, arg.ErrHelp) {
			argumentParser.WriteHelp(os.Stdout)
			fmt.Fprintln(os.Stdout, "Required output mode: (--json | --compact); choose exactly one.")
			fmt.Fprintln(os.Stdout, "Required root selector: choose exactly one of --symbol, --at, --package, --module, or --root-path.")
			if semantics := graphQuerySemantics(direction); semantics != "" {
				fmt.Fprintln(os.Stdout, semantics)
			}
			return values, true, nil
		}
		return values, false, err
	}
	if err := validateGraphQueryArgs(values); err != nil {
		return values, false, err
	}
	return values, false, nil
}

func runRemoteGraphQuery(ctx context.Context, direction search.NavigationQueryDirection, values graphQueryArgs, services Services) error {
	request := api.AnalysisRequest{
		Operation: api.AnalysisGraph, Repository: values.Repository, Paths: values.Paths, MaxFiles: values.MaxFiles,
		Graph: &api.GraphQueryRequest{Direction: string(direction), Depth: values.Depth, Symbol: values.Symbol, At: values.At, Package: values.Package, Module: values.Module, RootPath: values.RootPath, Languages: values.Languages, Confidences: values.Confidences, Visibilities: values.Visibilities},
	}
	response, err := services.Remote(ctx, request, services.server(values.Server))
	if err != nil {
		return err
	}
	if values.JSON {
		return cliruntime.NewOutput(os.Stdout).WriteJSON(response)
	}
	var output navigationGraphOutput
	if err := json.Unmarshal(response.Result, &output); err != nil {
		return fmt.Errorf("decode remote graph query: %w", err)
	}
	return renderCompactNavigationGraph(output, values.MaxOutputBytes)
}

func graphQueryContinuationCommand(direction search.NavigationQueryDirection, values graphQueryArgs, truncation *navigationGraphTruncation, services Services) string {
	if truncation == nil {
		return ""
	}
	parts := []string{"grepple", "graph", string(direction), "--max-files", "0", "--depth", fmt.Sprint(values.Depth), "--json"}
	if services.ActiveScopeFlags != nil {
		parts = services.ActiveScopeFlags(parts)
	}
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
	if values.JSON == values.Compact {
		return fmt.Errorf("grepple graph query requires exactly one of --json or --compact")
	}
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

func isGraphQueryDirection(value string) bool {
	switch search.NavigationQueryDirection(value) {
	case search.NavigationQueryCallers, search.NavigationQueryCallees, search.NavigationQueryDependencies, search.NavigationQueryDependents, search.NavigationQueryImpact:
		return true
	default:
		return false
	}
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

func buildNavigationGraphOutput(globs []string, maxFiles int, services Services) (navigationGraphOutput, error) {
	paths, err := ResolveInputPaths(globs, services.ApplySourceConfig)
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

// FromParts projects an existing navigation graph.
func FromParts(paths []string, discovered, unsupported int, truncation *Truncation, graph parser.NavigationGraph, stats search.NavigationSourceStats) Output {
	return OutputFromParts(paths, discovered, unsupported, truncation, graph, stats)
}

// QuerySelection selects and filters an in-memory graph traversal.
type QuerySelection struct {
	Symbol, At string
	Depth      int
	Filter     search.NavigationGraphFilter
}

// QueryOutput traverses an existing graph projection.
func QueryOutput(output Output, direction search.NavigationQueryDirection, selection QuerySelection) (Output, error) {
	filter, err := search.NormalizeNavigationGraphFilter(selection.Filter)
	if err != nil {
		return Output{}, err
	}
	graph := parser.NavigationGraph{Declarations: output.Declarations, TypeDeclarations: output.TypeDeclarations, Calls: output.Calls, Imports: output.Imports, Exports: output.Exports, Fields: output.Fields, TypeUsages: output.TypeUsages, MemberAccesses: output.MemberAccesses, RepositoryRoots: output.RepositoryRoots}
	graph, err = search.FilterNavigationGraph(graph, filter)
	if err != nil {
		return Output{}, err
	}
	roots, err := selectNavigationQueryRoots(graph.Declarations, graphQueryArgs{Symbol: selection.Symbol, At: selection.At, Depth: selection.Depth})
	if err != nil {
		return Output{}, err
	}
	rootIDs := navigationDeclarationIDs(roots)
	queried, err := search.QueryNavigationGraph(graph, rootIDs, direction, selection.Depth)
	if err != nil {
		return Output{}, err
	}
	output.Declarations, output.TypeDeclarations, output.Calls, output.Imports = queried.Declarations, queried.TypeDeclarations, queried.Calls, queried.Imports
	output.Exports, output.Fields, output.TypeUsages = queried.Exports, queried.Fields, queried.TypeUsages
	output.MemberAccesses, output.RepositoryRoots = queried.MemberAccesses, queried.RepositoryRoots
	output.Resolution = search.MeasureNavigationResolution(queried)
	output.Query = &Query{Direction: string(direction), Depth: selection.Depth, RootIDs: rootIDs, Languages: filter.Languages, Confidences: filter.Confidences}
	return output, nil
}

// IsQueryDirection reports whether value names a supported traversal.
func IsQueryDirection(value string) bool { return isGraphQueryDirection(value) }

func selectNavigationQueryRoots(declarations []parser.NavigationDeclaration, values graphQueryArgs) ([]parser.NavigationDeclaration, error) {
	if values.Symbol != "" || values.At != "" {
		root, err := selectNavigationQueryRoot(declarations, values.Symbol, values.At)
		if err != nil {
			return nil, err
		}
		return []parser.NavigationDeclaration{root}, nil
	}
	matches := make([]parser.NavigationDeclaration, 0)
	for _, declaration := range declarations {
		if navigationDeclarationMatchesScope(declaration, values) {
			matches = append(matches, declaration)
		}
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("no navigation declarations match %s", graphQueryScopeDescription(values))
	}
	return matches, nil
}

func navigationDeclarationMatchesScope(declaration parser.NavigationDeclaration, values graphQueryArgs) bool {
	if values.Package != "" {
		return declaration.Package == values.Package || declaration.PackageID == values.Package
	}
	if values.Module != "" {
		return declaration.ModuleID == values.Module
	}
	root := strings.TrimSuffix(navigationQueryDisplayPath(values.RootPath), "/")
	path := filepath.ToSlash(declaration.Path)
	return root == "." || path == root || strings.HasPrefix(path, root+"/")
}

func graphQueryScopeDescription(values graphQueryArgs) string {
	if values.Package != "" {
		return fmt.Sprintf("package %q", values.Package)
	}
	if values.Module != "" {
		return fmt.Sprintf("module %q", values.Module)
	}
	return fmt.Sprintf("root path %q", values.RootPath)
}

func navigationDeclarationIDs(declarations []parser.NavigationDeclaration) []string {
	ids := make([]string, 0, len(declarations))
	for _, declaration := range declarations {
		ids = append(ids, declaration.ID)
	}
	return ids
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
	path, line, err := codeextract.ParseAt(at)
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
