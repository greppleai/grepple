package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"charm.land/fantasy"
	"github.com/greppleai/grepple/api"
	"github.com/greppleai/grepple/parser"
	"github.com/greppleai/grepple/search"
)

type askSearchInput struct {
	Query      string   `json:"query" description:"Literal text to find, or a regular expression when regex is true"`
	Paths      []string `json:"paths,omitempty" description:"Local repository-relative files, directories, or globs; defaults to the workspace"`
	Repository string   `json:"repository,omitempty" description:"Exact indexed OWNER/REPO[@REF] selector; omit for local search"`
	Regex      bool     `json:"regex,omitempty" description:"Interpret query as a JavaScript regular expression; false means literal text"`
	IgnoreCase bool     `json:"ignore_case,omitempty" description:"Match without case distinctions"`
	Mode       string   `json:"mode,omitempty" description:"Result shape: snippets (default), files, or count"`
	Limit      int      `json:"limit,omitempty" description:"Maximum returned files, 1-20; defaults to 8"`
	Context    int      `json:"context,omitempty" description:"Context lines around matches, 0-3"`
}

type askNavigateInput struct {
	Location    string `json:"location" description:"Repository-relative PATH:LINE or PATH:START-END inside a declaration"`
	Repository  string `json:"repository,omitempty" description:"Exact indexed OWNER/REPO[@REF] selector; omit for the local workspace"`
	FollowDepth int    `json:"follow_depth,omitempty" description:"Inline resolved callees this many levels, 0-2; immediate callers/callees are always returned"`
}

type askStructuralInput struct {
	Query      string   `json:"query" description:"Native gritql-v1 structural query text"`
	Paths      []string `json:"paths,omitempty" description:"Local source files, directories, or globs; defaults to the workspace"`
	Repository string   `json:"repository,omitempty" description:"Exact indexed OWNER/REPO[@REF] selector; omit for local structural search"`
	Limit      int      `json:"limit,omitempty" description:"Maximum findings, 1-20; defaults to 10"`
}

type askArchitectureInput struct {
	Operation  string   `json:"operation" description:"One of directory, resolve, why, or responsibilities"`
	Paths      []string `json:"paths,omitempty" description:"Source files, directories, or globs; defaults to the workspace or selected repository"`
	Repository string   `json:"repository,omitempty" description:"Exact indexed OWNER/REPO[@REF] selector; omit for local architecture"`
	Symbol     string   `json:"symbol,omitempty" description:"Symbol name required by resolve"`
	From       string   `json:"from,omitempty" description:"Source directory required by why"`
	To         string   `json:"to,omitempty" description:"Target directory required by why"`
	MaxFiles   int      `json:"max_files,omitempty" description:"Maximum parsed files; 0 means all"`
}

type askGraphInput struct {
	Direction  string   `json:"direction" description:"One of callers, callees, dependencies, dependents, or impact"`
	Symbol     string   `json:"symbol,omitempty" description:"Exact declaration name; provide this or location, not both"`
	Location   string   `json:"location,omitempty" description:"Repository-relative PATH:LINE selecting one declaration; provide this or symbol, not both"`
	Paths      []string `json:"paths,omitempty" description:"Graph source scope; defaults to the workspace or selected repository"`
	Repository string   `json:"repository,omitempty" description:"Exact indexed OWNER/REPO[@REF] selector; omit for the local workspace"`
	Depth      int      `json:"depth,omitempty" description:"Traversal depth, 1-3; defaults to 1"`
	Language   string   `json:"language,omitempty" description:"Optional canonical language ID filter such as go or typescript"`
	Confidence string   `json:"confidence,omitempty" description:"Optional edge-confidence filter such as exact, import-resolved, context-resolved, or candidate"`
}

type askSourceScopeInput struct {
	Paths []string `json:"paths,omitempty" description:"Local files or directories to classify; defaults to the repository root"`
}

type askRepositoryRefsInput struct {
	Repository string `json:"repository" description:"Source OWNER/REPO whose indexed selectors should be listed"`
	Kind       string `json:"kind,omitempty" description:"Optional ref kind: default, branch, or tag"`
}

type askRepositoryTreeInput struct {
	Repository string `json:"repository" description:"Exact indexed OWNER/REPO[@REF] selector"`
	Path       string `json:"path,omitempty" description:"Optional repository-relative subtree"`
	Depth      int    `json:"depth,omitempty" description:"Tree depth, 1-4; defaults to 2"`
}

func askResearchToolNames() []string {
	return []string{"search_code", "navigate_code", "structural_search", "inspect_architecture", "query_graph", "explain_sources", "repository_refs", "repository_tree", "read_file"}
}

func askResearchToolInfo(tools []fantasy.AgentTool) []fantasy.ToolInfo {
	info := make([]fantasy.ToolInfo, 0, len(tools))
	for _, tool := range tools {
		info = append(info, tool.Info())
	}
	return info
}

func newAskResearchTools(root, server string) []fantasy.AgentTool {
	return newAskResearchToolsForSession(newResearchSession(context.Background(), nil, root, server), root, server)
}

func newAskResearchToolsForSession(session *researchSession, root, server string) []fantasy.AgentTool {
	return []fantasy.AgentTool{
		cachedAskTool(session, "search_code", "Search source text directly. Use mode=count to locate concentration, mode=files to choose files, then mode=snippets for bounded source-backed context. Set repository for remote indexed code.", func(ctx context.Context, input askSearchInput) (fantasy.ToolResponse, error) {
			return askToolResult(runAskSearch(ctx, root, server, input))
		}),
		cachedAskTool(session, "navigate_code", "Retrieve the exact declaration containing a known PATH:LINE and its immediate callers/callees. Use after search_code; candidate edges are leads, while exact/import-resolved/context-resolved edges are stronger evidence.", func(ctx context.Context, input askNavigateInput) (fantasy.ToolResponse, error) {
			return askToolResult(runAskNavigateWithSession(ctx, session, root, server, input))
		}),
		cachedAskTool(session, "structural_search", "Run a native read-only gritql-v1 syntax query across Tree-sitter-backed source. Use for code shapes, not type resolution, call impact, or data-flow proof.", func(ctx context.Context, input askStructuralInput) (fantasy.ToolResponse, error) {
			return askToolResult(runAskStructural(ctx, root, server, input))
		}),
		cachedAskTool(session, "inspect_architecture", "Inspect local or exact indexed-repository language-neutral architecture. directory orients ownership; resolve locates a symbol; why returns exact relation evidence; responsibilities summarizes physical directory ownership.", func(_ context.Context, input askArchitectureInput) (fantasy.ToolResponse, error) {
			return askToolResult(runAskArchitectureWithSession(session, root, input))
		}),
		cachedAskTool(session, "query_graph", "Query a local or exact indexed-repository parser-owned graph for callers, callees, dependencies, dependents, or impact. Select exactly one symbol or source location and keep depth small.", func(_ context.Context, input askGraphInput) (fantasy.ToolResponse, error) {
			return askToolResult(runAskGraphWithSession(session, root, input))
		}),
		cachedAskTool(session, "explain_sources", "Report which local files are selected, excluded, ignored, generated, vendored, tests, fixtures, or production. Use before completeness-sensitive conclusions.", func(_ context.Context, input askSourceScopeInput) (fantasy.ToolResponse, error) {
			return askToolResult(runAskSourceScope(root, input))
		}),
		cachedAskTool(session, "repository_refs", "Resolve a source OWNER/REPO and requested version to exact indexed repository selectors. Use before remote search, navigation, tree, or read when a branch or tag matters.", func(ctx context.Context, input askRepositoryRefsInput) (fantasy.ToolResponse, error) {
			return askToolResult(runAskRepositoryRefs(ctx, server, input))
		}),
		cachedAskTool(session, "repository_tree", "List a bounded tree from one exact indexed repository selector returned by repository_refs. Use it to discover remote paths before search or read.", func(ctx context.Context, input askRepositoryTreeInput) (fantasy.ToolResponse, error) {
			return askToolResult(runAskRepositoryTree(ctx, server, input))
		}),
		cachedAskTool(session, "read_file", "Batch-read up to eight bounded local or indexed-repository file ranges. Use files for implementation-oriented retrieval after paths are known. Local source rows carry HASH│LINE│content whenever anchors are enabled; preserve them exactly. Use outline=true for structural symbols.", func(ctx context.Context, input readToolInput) (fantasy.ToolResponse, error) {
			return runAskReadTool(ctx, root, server, input)
		}),
	}
}

func askToolResult(value any, err error) (fantasy.ToolResponse, error) {
	if err != nil {
		return fantasy.NewTextErrorResponse(err.Error()), nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return fantasy.NewTextErrorResponse(err.Error()), nil
	}
	if len(encoded) > defaultToolOutputSize {
		prefix := string(encoded[:defaultToolOutputSize/2])
		encoded, _ = json.Marshal(map[string]any{"truncated": true, "message": "result exceeded 64 KiB; narrow scope, limit, depth, or paths", "jsonPrefix": prefix})
	}
	return fantasy.NewTextResponse(string(encoded)), nil
}

func runAskSearch(ctx context.Context, root, server string, input askSearchInput) (any, error) {
	if strings.TrimSpace(input.Query) == "" {
		return nil, fmt.Errorf("query is required")
	}
	if input.Repository == "" {
		if err := validateAskLocalPaths(root, input.Paths); err != nil {
			return nil, err
		}
	}
	mode := input.Mode
	if mode == "" {
		mode = "snippets"
	}
	if mode != "snippets" && mode != "files" && mode != "count" {
		return nil, fmt.Errorf("mode must be snippets, files, or count")
	}
	limit, err := askBoundedValue(input.Limit, 8, 1, 20, "limit")
	if err != nil {
		return nil, err
	}
	if input.Context < 0 {
		return nil, fmt.Errorf("context must not be negative")
	}
	contextLines := min(input.Context, 3)
	params := search.Params{Query: input.Query, Globs: input.Paths, Regex: input.Regex, IgnoreCase: input.IgnoreCase, Root: root, Limit: limit + 1, Sort: search.ResultSortPath, BeforeContext: contextLines, AfterContext: contextLines, SkipSegments: mode != "snippets"}
	results, err := askSearchResults(ctx, server, params, input.Repository)
	if err != nil {
		return nil, err
	}
	hasMore := len(results) > limit
	if hasMore {
		results = results[:limit]
	}
	if mode == "snippets" {
		return map[string]any{"mode": mode, "repository": input.Repository, "returnedFiles": len(results), "hasMore": hasMore, "results": results}, nil
	}
	files := make([]map[string]any, 0, len(results))
	totalLines := 0
	for _, result := range results {
		files = append(files, map[string]any{"repo": result.Repo, "path": result.Path, "matchingLines": len(result.Matches)})
		totalLines += len(result.Matches)
	}
	return map[string]any{"mode": mode, "repository": input.Repository, "returnedFiles": len(files), "matchingLinesInReturnedFiles": totalLines, "hasMore": hasMore, "files": files}, nil
}

func askSearchResults(ctx context.Context, server string, params search.Params, repository string) ([]api.FileResult, error) {
	if repository != "" {
		params.Repo = []string{repository}
		return searchRemoteContext(ctx, &cliOptions{Params: params}, server)
	}
	if err := applyRepositorySourceConfig(&params); err != nil {
		return nil, err
	}
	matches, err := search.Files(params, nil)
	if err != nil {
		return nil, err
	}
	return search.BuildResults(matches, params.BeforeContext, params.AfterContext, !params.SkipSegments), nil
}

func runAskNavigate(ctx context.Context, root, server string, input askNavigateInput) (any, error) {
	session := newResearchSession(ctx, nil, root, server)
	defer session.Close()
	return runAskNavigateWithSession(ctx, session, root, server, input)
}

func runAskNavigateWithSession(ctx context.Context, session *researchSession, root, server string, input askNavigateInput) (any, error) {
	if strings.TrimSpace(input.Location) == "" {
		return nil, fmt.Errorf("location is required")
	}
	if input.FollowDepth < 0 || input.FollowDepth > 2 {
		return nil, fmt.Errorf("follow_depth must be between 0 and 2")
	}
	params := search.Params{At: input.Location, Root: root, Related: true, FollowRelated: input.FollowDepth, Sort: search.ResultSortPath, Limit: 1}
	if input.Repository != "" {
		params.Repo = []string{input.Repository}
		results, err := searchRemoteContext(ctx, &cliOptions{Params: params}, server)
		if err != nil {
			return nil, err
		}
		return map[string]any{"repository": input.Repository, "location": input.Location, "results": results}, nil
	}
	if err := validateAskLocation(root, input.Location); err != nil {
		return nil, err
	}
	if err := applyRepositorySourceConfig(&params); err != nil {
		return nil, err
	}
	universe, err := session.localUniverse(nil, 0)
	if err != nil {
		return nil, err
	}
	separator := strings.LastIndex(input.Location, ":")
	document := universe.document(filepath.Join(root, input.Location[:separator]))
	var match *search.FileMatch
	if document == nil {
		match, err = search.At(params)
	} else {
		match, err = search.AtFromDocument(params, document, universe.analysis)
	}
	if err != nil {
		return nil, err
	}
	results := search.BuildResults([]search.FileMatch{*match}, 0, 0, true)
	response := map[string]any{"location": input.Location, "results": results}
	if !match.CallableDeclaration {
		response["correction"] = map[string]string{
			"reason": "location is not inside a callable declaration; related graph construction was skipped",
			"next":   "use search_code snippets or read_file with outline=true to choose a function or method location",
		}
	}
	return response, nil
}

func runAskStructural(ctx context.Context, root, server string, input askStructuralInput) (api.GritResponse, error) {
	if input.Repository == "" {
		if err := validateAskLocalPaths(root, input.Paths); err != nil {
			return api.GritResponse{}, err
		}
	}
	limit, err := askBoundedValue(input.Limit, 10, 1, 20, "limit")
	if err != nil {
		return api.GritResponse{}, err
	}
	values := gritArgs{Query: input.Query, Globs: input.Paths, Limit: limit, MaxFindings: 100}
	if input.Repository != "" {
		values.Remote = true
		values.Repositories = []string{input.Repository}
	}
	if err := validateGritArgs(values); err != nil {
		return api.GritResponse{}, err
	}
	query, program, err := compileGritQuery(values)
	if err != nil {
		return api.GritResponse{}, err
	}
	if input.Repository != "" {
		return requestGritRemote(ctx, gritRequest(values, query), server)
	}
	response, err := acquireGritLocal(ctx, values, program)
	if err != nil {
		return api.GritResponse{}, err
	}
	response.Findings = windowGritFindings(response.Findings, 0, limit)
	return response, nil
}

func runAskArchitecture(root string, input askArchitectureInput) (any, error) {
	session := newResearchSession(context.Background(), nil, root, "")
	defer session.Close()
	return runAskArchitectureWithSession(session, root, input)
}

func runAskArchitectureWithSession(session *researchSession, root string, input askArchitectureInput) (any, error) {
	if input.MaxFiles < 0 {
		return nil, fmt.Errorf("max_files must not be negative")
	}
	switch input.Operation {
	case "directory", "responsibilities":
	case "resolve":
		if strings.TrimSpace(input.Symbol) == "" {
			return nil, fmt.Errorf("symbol is required for resolve")
		}
	case "why":
		if strings.TrimSpace(input.From) == "" || strings.TrimSpace(input.To) == "" {
			return nil, fmt.Errorf("from and to are required for why")
		}
	default:
		return nil, fmt.Errorf("operation must be directory, resolve, why, or responsibilities")
	}
	if input.Repository != "" {
		return runAskRemoteArchitecture(session, input)
	}
	if err := validateAskLocalPaths(root, input.Paths); err != nil {
		return nil, err
	}
	universe, err := session.localUniverse(input.Paths, input.MaxFiles)
	if err != nil {
		return nil, err
	}
	architecture := universe.architecture()
	switch input.Operation {
	case "directory":
		return architecture, nil
	case "resolve":
		return architectureResolveOutput{Schema: "grepple-architecture-resolve-v1", Symbol: input.Symbol, Sources: architecture.Sources, Matches: resolveArchitectureSymbols(architecture.Symbols, input.Symbol)}, nil
	case "why":
		evidence := architectureRelationEvidenceFor(architecture.Relations, input.From, input.To)
		return architectureWhyOutput{Schema: "grepple-architecture-why-v2", From: cleanArchitectureDirectory(input.From), To: cleanArchitectureDirectory(input.To), Relation: architectureEvidenceRelation(evidence), Sources: architecture.Sources, Evidence: evidence}, nil
	case "responsibilities":
		return buildArchitectureResponsibilitiesOutput(architecture), nil
	}
	return nil, fmt.Errorf("unsupported architecture operation")
}

func runAskRemoteArchitecture(session *researchSession, input askArchitectureInput) (any, error) {
	operation := api.AnalysisArchitecture
	if input.Operation == "responsibilities" {
		operation = api.AnalysisResponsibilities
	}
	response, err := requestAnalysisRemote(session.ctx, api.AnalysisRequest{Operation: operation, Repository: input.Repository, Paths: input.Paths, MaxFiles: input.MaxFiles}, serverDefault(session.server))
	if err != nil {
		return nil, err
	}
	if input.Operation == "directory" || input.Operation == "responsibilities" {
		return response, nil
	}
	var architecture directoryArchitecture
	if err := json.Unmarshal(response.Result, &architecture); err != nil {
		return nil, fmt.Errorf("decode remote architecture: %w", err)
	}
	var projection any
	switch input.Operation {
	case "resolve":
		projection = architectureResolveOutput{Schema: "grepple-architecture-resolve-v1", Symbol: input.Symbol, Sources: architecture.Sources, Matches: resolveArchitectureSymbols(architecture.Symbols, input.Symbol)}
	case "why":
		evidence := architectureRelationEvidenceFor(architecture.Relations, input.From, input.To)
		projection = architectureWhyOutput{Schema: "grepple-architecture-why-v2", From: cleanArchitectureDirectory(input.From), To: cleanArchitectureDirectory(input.To), Relation: architectureEvidenceRelation(evidence), Sources: architecture.Sources, Evidence: evidence}
	}
	response.Result, err = json.Marshal(projection)
	if err != nil {
		return nil, err
	}
	return response, nil
}

func runAskRemoteGraph(session *researchSession, input askGraphInput, depth int) (any, error) {
	request := api.AnalysisRequest{Operation: api.AnalysisGraph, Repository: input.Repository, Paths: input.Paths, Graph: &api.GraphQueryRequest{Direction: input.Direction, Depth: depth, Symbol: input.Symbol, At: input.Location}}
	if input.Language != "" {
		request.Graph.Languages = []string{input.Language}
	}
	if input.Confidence != "" {
		request.Graph.Confidences = []string{input.Confidence}
	}
	return requestAnalysisRemote(session.ctx, request, serverDefault(session.server))
}

func runAskGraph(root string, input askGraphInput) (any, error) {
	session := newResearchSession(context.Background(), nil, root, "")
	defer session.Close()
	return runAskGraphWithSession(session, root, input)
}

func runAskGraphWithSession(session *researchSession, root string, input askGraphInput) (any, error) {
	direction, depth, err := validateAskGraphInput(root, input)
	if err != nil {
		return nil, err
	}
	if input.Repository != "" {
		return runAskRemoteGraph(session, input, depth)
	}
	universe, err := session.localUniverse(input.Paths, 0)
	if err != nil {
		return nil, err
	}
	output := universe.navigationOutput()
	filter := search.NavigationGraphFilter{}
	if input.Language != "" {
		filter.Languages = []string{input.Language}
	}
	if input.Confidence != "" {
		filter.Confidences = []string{input.Confidence}
	}
	filter, err = search.NormalizeNavigationGraphFilter(filter)
	if err != nil {
		return nil, err
	}
	graph := navigationOutputGraph(output)
	graph, err = search.FilterNavigationGraph(graph, filter)
	if err != nil {
		return nil, err
	}
	values := graphQueryArgs{Symbol: input.Symbol, At: input.Location, Depth: depth}
	roots, err := selectNavigationQueryRoots(graph.Declarations, values)
	if err != nil {
		return nil, err
	}
	rootIDs := navigationDeclarationIDs(roots)
	queried, err := search.QueryNavigationGraph(graph, rootIDs, direction, depth)
	if err != nil {
		return nil, err
	}
	output.Declarations, output.Calls, output.Imports = queried.Declarations, queried.Calls, queried.Imports
	output.Exports, output.Fields, output.TypeUsages = queried.Exports, queried.Fields, queried.TypeUsages
	output.MemberAccesses, output.RepositoryRoots = queried.MemberAccesses, queried.RepositoryRoots
	output.Resolution = search.MeasureNavigationResolution(queried)
	output.Query = &navigationGraphQuery{Direction: input.Direction, Depth: depth, RootIDs: rootIDs, Languages: filter.Languages, Confidences: filter.Confidences}
	return output, nil
}

func validateAskGraphInput(root string, input askGraphInput) (search.NavigationQueryDirection, int, error) {
	if input.Repository == "" {
		if err := validateAskLocalPaths(root, input.Paths); err != nil {
			return "", 0, err
		}
		if input.Location != "" {
			if err := validateAskLocation(root, input.Location); err != nil {
				return "", 0, err
			}
		}
	}
	direction := search.NavigationQueryDirection(input.Direction)
	if !isGraphQueryDirection(input.Direction) {
		return "", 0, fmt.Errorf("direction must be callers, callees, dependencies, dependents, or impact")
	}
	if (input.Symbol == "") == (input.Location == "") {
		return "", 0, fmt.Errorf("provide exactly one of symbol or location")
	}
	depth, err := askBoundedValue(input.Depth, 1, 1, 3, "depth")
	return direction, depth, err
}

func navigationOutputGraph(output navigationGraphOutput) parser.NavigationGraph {
	return parser.NavigationGraph{Declarations: output.Declarations, Calls: output.Calls, Imports: output.Imports, Exports: output.Exports, Fields: output.Fields, TypeUsages: output.TypeUsages, MemberAccesses: output.MemberAccesses, RepositoryRoots: output.RepositoryRoots}
}

func runAskSourceScope(root string, input askSourceScopeInput) (sourceScopeReport, error) {
	if err := validateAskLocalPaths(root, input.Paths); err != nil {
		return sourceScopeReport{}, err
	}
	return buildSourceScopeReport(input.Paths)
}

func runAskRepositoryRefs(ctx context.Context, server string, input askRepositoryRefsInput) (api.ReposResponse, error) {
	repository := strings.TrimSpace(input.Repository)
	if repository == "" {
		return api.ReposResponse{}, fmt.Errorf("repository is required")
	}
	kind := strings.ToLower(strings.TrimSpace(input.Kind))
	if kind != "" && kind != "default" && kind != "branch" && kind != "tag" {
		return api.ReposResponse{}, fmt.Errorf("kind must be default, branch, or tag")
	}
	entries, err := fetchRepoListContext(ctx, server)
	if err != nil {
		return api.ReposResponse{}, err
	}
	entries = refsForRepository(entries, repository, kind)
	return api.ReposResponse{OK: true, Count: len(entries), Repos: entries}, nil
}

func runAskRepositoryTree(ctx context.Context, server string, input askRepositoryTreeInput) (api.TreeResponse, error) {
	if strings.TrimSpace(input.Repository) == "" {
		return api.TreeResponse{}, fmt.Errorf("repository is required")
	}
	depth, err := askBoundedValue(input.Depth, 2, 1, 4, "depth")
	if err != nil {
		return api.TreeResponse{}, err
	}
	return fetchTreeContext(ctx, server, treeArgs{Repo: input.Repository, Path: input.Path, Depth: depth})
}

func runAskReadTool(ctx context.Context, root, server string, input readToolInput) (fantasy.ToolResponse, error) {
	inputs, err := expandReadToolInput(input)
	if err != nil {
		return fantasy.NewTextErrorResponse(err.Error()), nil
	}
	if len(inputs) == 1 {
		return runAskSingleReadTool(ctx, root, server, inputs[0])
	}
	var output strings.Builder
	for _, current := range inputs {
		response, err := runAskSingleReadTool(ctx, root, server, current)
		if err != nil {
			return response, err
		}
		if response.IsError {
			return response, nil
		}
		section := fmt.Sprintf("== %s ==\n%s", current.Path, response.Content)
		if output.Len()+len(section) > defaultToolOutputSize {
			output.WriteString("… batch read truncated at 64 KiB; request fewer or narrower ranges …\n")
			break
		}
		output.WriteString(section)
		if !strings.HasSuffix(section, "\n") {
			output.WriteByte('\n')
		}
	}
	return fantasy.NewTextResponse(output.String()), nil
}

func expandReadToolInput(input readToolInput) ([]readToolInput, error) {
	if len(input.Files) == 0 {
		if strings.TrimSpace(input.Path) == "" {
			return nil, fmt.Errorf("path or files is required")
		}
		return []readToolInput{input}, nil
	}
	if input.Path != "" || input.StartLine != 0 || input.EndLine != 0 || input.Outline {
		return nil, fmt.Errorf("path, start_line, end_line, and outline cannot be combined with files")
	}
	if len(input.Files) > 8 {
		return nil, fmt.Errorf("files accepts at most eight ranges")
	}
	result := make([]readToolInput, 0, len(input.Files))
	for _, file := range input.Files {
		if strings.TrimSpace(file.Path) == "" {
			return nil, fmt.Errorf("every files entry requires path")
		}
		result = append(result, readToolInput{Path: file.Path, Repository: input.Repository, StartLine: file.StartLine, EndLine: file.EndLine, Outline: file.Outline})
	}
	return result, nil
}

func runAskSingleReadTool(ctx context.Context, root, server string, input readToolInput) (fantasy.ToolResponse, error) {
	if input.Repository == "" {
		return runAskLocalReadTool(root, input)
	}
	return runAskRemoteReadTool(ctx, server, input)
}

func runAskLocalReadTool(root string, input readToolInput) (fantasy.ToolResponse, error) {
	if !input.Outline {
		return runSimpleReadTool(root, input)
	}
	path, err := confinedReadPath(root, input.Path)
	if err != nil {
		return fantasy.NewTextErrorResponse(err.Error()), nil
	}
	content, err := readBoundedSource(path)
	if err != nil {
		return fantasy.NewTextErrorResponse(err.Error()), nil
	}
	return askToolResult(parser.OutlineFileDepth(input.Path, string(content), 0), nil)
}

func runAskRemoteReadTool(ctx context.Context, server string, input readToolInput) (fantasy.ToolResponse, error) {
	if strings.TrimSpace(input.Path) == "" {
		return fantasy.NewTextErrorResponse("path is required"), nil
	}
	values := getArgs{commonArgs: commonArgs{Server: server}, Repo: input.Repository, Path: input.Path, Outline: input.Outline}
	if !input.Outline {
		start, end, err := askReadRange(input.StartLine, input.EndLine)
		if err != nil {
			return fantasy.NewTextErrorResponse(err.Error()), nil
		}
		values.Lines = fmt.Sprintf("%d:%d", start, end)
	}
	target, err := getRawURL(values)
	if err != nil {
		return fantasy.NewTextErrorResponse(err.Error()), nil
	}
	content, err := fetchRawContext(ctx, target.String(), server)
	if err != nil {
		return fantasy.NewTextErrorResponse(err.Error()), nil
	}
	if len(content) > maxReadBytes || bytes.IndexByte(content, 0) >= 0 {
		return fantasy.NewTextErrorResponse("file response is binary or exceeds 256 KiB"), nil
	}
	if input.Outline {
		return askToolResult(parser.OutlineFileDepth(input.Path, string(content), 0), nil)
	}
	start, _, _ := askReadRange(input.StartLine, input.EndLine)
	lines := strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n")
	var output strings.Builder
	fmt.Fprintf(&output, "%s/%s\n", input.Repository, input.Path)
	for index, line := range lines {
		fmt.Fprintf(&output, "%d│%s\n", start+index, line)
	}
	return fantasy.NewTextResponse(output.String()), nil
}

func readBoundedSource(path string) ([]byte, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(content) > maxReadBytes || bytes.IndexByte(content, 0) >= 0 {
		return nil, fmt.Errorf("file is binary or exceeds the 256 KiB read limit")
	}
	return content, nil
}

func askReadRange(start, end int) (int, int, error) {
	if start < 1 {
		start = 1
	}
	if end == 0 {
		end = start + 199
	}
	if end < start || end-start+1 > maxReadLines {
		return 0, 0, fmt.Errorf("read range must be ordered and at most 1000 lines")
	}
	return start, end, nil
}

func validateAskLocalPaths(root string, paths []string) error {
	for _, path := range paths {
		candidate, err := askWorkspaceRelativePath(root, path)
		if err != nil {
			return err
		}
		if _, err := confinedReadPath(root, askResearchPathPrefix(candidate)); err != nil {
			return fmt.Errorf("research path %q: %w", path, err)
		}
	}
	return nil
}

func askWorkspaceRelativePath(root, path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("local research paths must be non-empty")
	}
	if !filepath.IsAbs(path) {
		return path, nil
	}
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("research path %q escapes the workspace", path)
	}
	return relative, nil
}

func askResearchPathPrefix(path string) string {
	wildcard := strings.IndexAny(path, "*?[")
	if wildcard < 0 {
		return path
	}
	prefix := path[:wildcard]
	if prefix == "" {
		return "."
	}
	if !strings.HasSuffix(prefix, "/") && !strings.HasSuffix(prefix, string(filepath.Separator)) {
		return filepath.Dir(prefix)
	}
	return prefix
}

func validateAskLocation(root, location string) error {
	separator := strings.LastIndex(location, ":")
	if separator < 1 {
		return fmt.Errorf("location must be PATH:LINE or PATH:START-END")
	}
	return validateAskLocalPaths(root, []string{location[:separator]})
}

func askBoundedValue(value, fallback, minimum, maximum int, name string) (int, error) {
	if value == 0 {
		return fallback, nil
	}
	if value < minimum {
		return 0, fmt.Errorf("%s must be at least %d", name, minimum)
	}
	if value > maximum {
		return maximum, nil
	}
	return value, nil
}
