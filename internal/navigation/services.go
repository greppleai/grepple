package navigation

import "github.com/greppleai/grepple/internal/parser"

// GraphAnalysis is the reusable, caller-owned projection of one resolved graph.
type GraphAnalysis interface {
	Graph() parser.NavigationGraph
}

// GraphEngine constructs repository graphs from one selected source form.
// Documents remain owned by the caller; options apply consistently to every build.
type GraphEngine interface {
	BuildFiles([]string) (parser.NavigationGraph, SourceStats)
	BuildDocuments([]DocumentSource) (GraphAnalysis, SourceStats)
	BuildTextSources([]TextSource) (GraphAnalysis, SourceStats)
	RepositoryContextFiles([]string) []string
}

type graphEngine struct{ options BuildOptions }

// NewGraphEngine returns the graph-building capability for the requested options.
func NewGraphEngine(options BuildOptions) GraphEngine { return graphEngine{options: options} }

func (engine graphEngine) BuildFiles(files []string) (parser.NavigationGraph, SourceStats) {
	return buildGraphWithOptions(files, engine.options)
}

func (engine graphEngine) BuildDocuments(sources []DocumentSource) (GraphAnalysis, SourceStats) {
	return buildAnalysisFromDocuments(sources, engine.options)
}

func (engine graphEngine) BuildTextSources(sources []TextSource) (GraphAnalysis, SourceStats) {
	return buildAnalysisFromTextSources(sources, engine.options)
}

func (graphEngine) RepositoryContextFiles(paths []string) []string {
	return repositoryContextFiles(paths)
}

// GraphOperations projects, compares, and measures already resolved graphs.
// It does not own the source documents or alter the input graph.
type GraphOperations interface {
	Diff(before, after parser.NavigationGraph) NavigationGraphDiff
	NormalizeFilter(NavigationGraphFilter) (NavigationGraphFilter, error)
	Filter(parser.NavigationGraph, NavigationGraphFilter) (parser.NavigationGraph, error)
	Query(parser.NavigationGraph, []string, NavigationQueryDirection, int) (parser.NavigationGraph, error)
	ResolutionStats(parser.NavigationGraph) NavigationResolutionStats
}

type graphOperations struct{}

// NewGraphOperations returns read-only graph projection capabilities.
func NewGraphOperations() GraphOperations { return graphOperations{} }

func (graphOperations) Diff(before, after parser.NavigationGraph) NavigationGraphDiff {
	return diffNavigationGraphs(before, after)
}

func (graphOperations) NormalizeFilter(filter NavigationGraphFilter) (NavigationGraphFilter, error) {
	return normalizeNavigationGraphFilter(filter)
}

func (graphOperations) Filter(graph parser.NavigationGraph, filter NavigationGraphFilter) (parser.NavigationGraph, error) {
	return filterNavigationGraph(graph, filter)
}

func (graphOperations) Query(graph parser.NavigationGraph, roots []string, direction NavigationQueryDirection, depth int) (parser.NavigationGraph, error) {
	return queryNavigationGraph(graph, roots, direction, depth)
}

func (graphOperations) ResolutionStats(graph parser.NavigationGraph) NavigationResolutionStats {
	return measureNavigationResolution(graph)
}
