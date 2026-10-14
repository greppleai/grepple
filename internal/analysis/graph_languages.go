package analysis

import (
	"errors"
	"fmt"
	"os"

	"github.com/greppleai/grepple/internal/navigation"
	"github.com/greppleai/grepple/internal/parser"
)

var errMultipleLanguageRoots = errors.New("multi-partition roots require complete view")

func buildLanguageStoredGraph(sources []Source, maxFiles int, request GraphQuery) (GraphReport, error) {
	if err := validateGraphQuery(request); err != nil {
		return GraphReport{}, err
	}
	operations := navigation.NewGraphOperations()
	filter, err := operations.NormalizeFilter(navigation.NavigationGraphFilter{Languages: request.Languages, Confidences: request.Confidences, Visibilities: request.Visibilities})
	if err != nil {
		return GraphReport{}, err
	}
	key, path := storedGraphKey(sources, maxFiles, filter)
	if path == "" {
		return buildMonolithicStoredGraph(sources, maxFiles, request)
	}
	if manifest, err := readLanguageManifest(path, key); err == nil {
		report, queryErr := queryLanguageManifest(manifest, path, key, request, filter)
		if queryErr == nil {
			return report, nil
		}
		if errors.Is(queryErr, navigation.ErrSnapshotBudget) {
			return GraphReport{}, queryErr
		}
		if errors.Is(queryErr, errMultipleLanguageRoots) {
			return buildMonolithicStoredGraph(sources, maxFiles, request)
		}
	}
	return buildFreshLanguageGraph(sources, maxFiles, request, filter, key, path, operations)
}
func buildFreshLanguageGraph(sources []Source, maxFiles int, request GraphQuery, filter navigation.NavigationGraphFilter, key, path string, operations navigation.GraphOperations) (GraphReport, error) {
	universe, err := NewUniverse(sources, maxFiles)
	if err != nil {
		return GraphReport{}, err
	}
	defer universe.Close()
	graph, err := operations.Filter(universe.Graph(), filter)
	if err != nil {
		return GraphReport{}, err
	}
	manifest := newLanguageManifest(graph, operations.PartitionLanguages(graph), storedGraphMetadata{Summary: universe.Summary(), Truncation: universe.Truncation()})
	if current, _ := storedGraphKey(sources, maxFiles, filter); current == key && universe.Summary().Failed == 0 {
		if err := publishLanguageManifest(path, key, &manifest, operations); err == nil {
			report, queryErr := queryLanguageManifest(manifest, path, key, request, filter)
			if !errors.Is(queryErr, errMultipleLanguageRoots) {
				return report, queryErr
			}
		}
	}
	prepared, err := operations.PrepareQuery(graph)
	if err != nil {
		return GraphReport{}, err
	}
	return projectStoredGraph(prepared, graph.Declarations, universe, request, filter)
}
func queryLanguageManifest(manifest languageManifest, path, key string, request GraphQuery, filter navigation.NavigationGraphFilter) (GraphReport, error) {
	declarations := languageManifestDeclarations(manifest)
	roots, err := selectGraphRoots(declarations, request)
	if err != nil {
		return GraphReport{}, err
	}
	selected := -1
	for _, root := range roots {
		index := languageManifestPartition(manifest, root.Language)
		if index < 0 {
			return GraphReport{}, fmt.Errorf("missing language partition")
		}
		if selected >= 0 && selected != index {
			return GraphReport{}, errMultipleLanguageRoots
		}
		selected = index
	}
	entry := manifest.Parts[selected]
	store := navigation.NewGraphOperations().SnapshotStore()
	var snapshot navigation.SnapshotLease
	if os.Getenv("GREPPLE_NAVIGATION_EAGER_PARTITIONS") == "1" {
		snapshot, _, err = store.Open(languagePartitionPath(path, manifest.Generation, selected), languagePartitionKey(key, entry.Languages))
	} else {
		snapshot, _, err = store.OpenPaged(languagePartitionPath(path, manifest.Generation, selected), languagePartitionKey(key, entry.Languages))
	}
	if err != nil {
		return GraphReport{}, err
	}
	defer snapshot.Close()
	universe := &Universe{summary: manifest.State.Summary, truncation: manifest.State.Truncation}
	return projectStoredGraph(snapshot, declarations, universe, request, filter)
}
func languageManifestPartition(manifest languageManifest, language string) int {
	for i, part := range manifest.Parts {
		for _, value := range part.Languages {
			if value == language {
				return i
			}
		}
	}
	return -1
}
func newLanguageManifest(graph parser.NavigationGraph, parts []navigation.LanguagePartition, state storedGraphMetadata) languageManifest {
	manifest := languageManifest{Version: 3, State: state, Roots: make([]languageRootRow, 0, len(graph.Declarations))}
	sources := map[languageRootSource]uint32{}
	for _, d := range graph.Declarations {
		source := languageRootSource{Path: d.Path, Language: d.Language, Package: d.Package, PackageID: d.PackageID, ModuleID: d.ModuleID}
		index, ok := sources[source]
		if !ok {
			index = uint32(len(manifest.RootSources))
			sources[source] = index
			manifest.RootSources = append(manifest.RootSources, source)
		}
		manifest.Roots = append(manifest.Roots, languageRootRow{ID: d.ID, Name: d.Name, Source: index, Start: d.Start, End: d.End})
	}
	for _, part := range parts {
		manifest.Parts = append(manifest.Parts, languageManifestPart{Languages: part.Languages, graph: part.Graph})
	}
	return manifest
}
