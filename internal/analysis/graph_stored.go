package analysis

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/greppleai/grepple/internal/navigation"
	"github.com/greppleai/grepple/internal/parser"
)

var storedExecutableOnce sync.Once
var storedExecutableDigest string

// BuildStoredGraph uses a source-fenced query-ready filtered view. The complete
// resolved graph/facts remain owned by the existing build path; the view is not
// a replacement semantic artifact. Roots and directions share the same view.
func BuildStoredGraph(sources []Source, maxFiles int, request GraphQuery) (GraphReport, error) {
	if os.Getenv("GREPPLE_NAVIGATION_REFERENCE_GRAPH") == "1" || os.Getenv("GREPPLE_NAVIGATION_MONOLITHIC") == "1" {
		return buildMonolithicStoredGraph(sources, maxFiles, request)
	}
	return buildLanguageStoredGraph(sources, maxFiles, request)
}
func buildMonolithicStoredGraph(sources []Source, maxFiles int, request GraphQuery) (GraphReport, error) {
	// Reference path is retained for same-binary rollout verification only.
	if os.Getenv("GREPPLE_NAVIGATION_REFERENCE_GRAPH") == "1" {
		return buildReferenceStoredGraph(sources, maxFiles, request)
	}
	if err := validateGraphQuery(request); err != nil {
		return GraphReport{}, err
	}
	operations := navigation.NewGraphOperations()
	filter, err := operations.NormalizeFilter(navigation.NavigationGraphFilter{Languages: request.Languages, Confidences: request.Confidences, Visibilities: request.Visibilities})
	if err != nil {
		return GraphReport{}, err
	}
	key, path := storedGraphKey(sources, maxFiles, filter)
	store := operations.SnapshotStore()
	if path != "" {
		if snapshot, metadata, openErr := store.Open(path, key); openErr == nil {
			report, queryErr := queryStoredGraph(snapshot, metadata, request, filter)
			snapshot.Close()
			if queryErr == nil {
				return report, nil
			}
			// Rebuild corrupted details once. Semantic selector errors are reproduced
			// by the fresh path, never hidden behind an incomplete projection.
		}
	}
	universe, err := NewUniverse(sources, maxFiles)
	if err != nil {
		return GraphReport{}, err
	}
	defer universe.Close()
	graph, err := operations.Filter(universe.Graph(), filter)
	if err != nil {
		return GraphReport{}, err
	}
	prepared, err := operations.PrepareQuery(graph)
	if err != nil {
		return GraphReport{}, err
	}
	metadata, err := json.Marshal(storedGraphMetadata{Summary: universe.Summary(), Truncation: universe.Truncation()})
	if err != nil {
		return GraphReport{}, err
	}
	currentKey, _ := storedGraphKey(sources, maxFiles, filter)
	if path != "" && currentKey == key && universe.Summary().Failed == 0 {
		_ = store.Save(path, key, prepared, metadata)
	}
	return projectStoredGraph(prepared, graph.Declarations, universe, request, filter)
}

type storedGraphMetadata struct {
	Summary    SourceSummary
	Truncation *Truncation
}

func queryStoredGraph(snapshot navigation.StoredSnapshot, metadata []byte, request GraphQuery, filter navigation.NavigationGraphFilter) (GraphReport, error) {
	var state storedGraphMetadata
	if err := json.Unmarshal(metadata, &state); err != nil {
		return GraphReport{}, err
	}
	declarations, err := snapshot.Declarations()
	if err != nil {
		return GraphReport{}, err
	}
	universe := &Universe{summary: state.Summary, truncation: state.Truncation}
	return projectStoredGraph(snapshot, declarations, universe, request, filter)
}
func projectStoredGraph(snapshot navigation.QuerySnapshot, declarations []parser.NavigationDeclaration, universe *Universe, request GraphQuery, filter navigation.NavigationGraphFilter) (GraphReport, error) {
	roots, err := selectGraphRoots(declarations, request)
	if err != nil {
		return GraphReport{}, err
	}
	ids := make([]string, 0, len(roots))
	for _, root := range roots {
		ids = append(ids, root.ID)
	}
	graph, err := snapshot.Query(ids, navigation.NavigationQueryDirection(request.Direction), request.Depth)
	if err != nil {
		return GraphReport{}, err
	}
	report := graphReport(graph, universe)
	report.Query = &GraphQuery{Direction: request.Direction, Depth: request.Depth, RootIDs: ids, Languages: filter.Languages, Confidences: filter.Confidences, Visibilities: filter.Visibilities}
	return report, nil
}
func storedGraphKey(sources []Source, maxFiles int, filter navigation.NavigationGraphFilter) (string, string) {
	directory := os.Getenv(parser.NavigationCacheDirectoryEnv)
	if directory == "" {
		return "", ""
	}
	storedExecutableOnce.Do(func() {
		file, err := os.Open(executablePath())
		if err != nil {
			return
		}
		defer file.Close()
		digest := sha256.New()
		if _, err = io.Copy(digest, file); err == nil {
			storedExecutableDigest = hex.EncodeToString(digest.Sum(nil))
		}
	})
	if storedExecutableDigest == "" {
		return "", ""
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", ""
	}
	digest := sha256.New()
	encode := json.NewEncoder(digest)
	_ = encode.Encode([]any{"cli-filtered-navigation-v1", storedExecutableDigest, cwd, maxFiles, filter})
	ordered := append([]Source(nil), sources...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Path < ordered[j].Path })
	paths := make([]string, 0, len(ordered))
	for _, source := range ordered {
		if source.ReadError != nil {
			return "", ""
		}
		paths = append(paths, source.Path)
		sum := sha256.Sum256(source.Content)
		_ = encode.Encode([]any{source.Path, sum})
	}
	contextFiles := storedGraphContextPaths(paths, cwd)
	sort.Strings(contextFiles)
	for _, path := range contextFiles {
		sum, missing, err := storedContextDigest(path)
		if err != nil {
			return "", ""
		}
		_ = encode.Encode([]any{path, missing, sum})
	}
	key := hex.EncodeToString(digest.Sum(nil))
	return key, filepath.Join(directory, "query-ready", key+".gnav")
}
func executablePath() string {
	path, err := os.Executable()
	if err != nil {
		return ""
	}
	return path
}
func storedGraphContextPaths(paths []string, cwd string) []string {
	candidates := map[string]bool{}
	for _, path := range paths {
		absolute := filepath.Clean(path)
		if !filepath.IsAbs(absolute) {
			absolute = filepath.Join(cwd, absolute)
		}
		for directory := filepath.Dir(absolute); ; directory = filepath.Dir(directory) {
			for _, name := range []string{"go.mod", "go.work", "tsconfig.json", "Cargo.toml", "Cargo.lock", "package.json", "pyproject.toml", "composer.json"} {
				candidates[filepath.Join(directory, name)] = true
			}
			if filepath.Dir(directory) == directory {
				break
			}
		}
	}
	result := make([]string, 0, len(candidates))
	for path := range candidates {
		result = append(result, path)
	}
	sort.Strings(result)
	return result
}
func buildReferenceStoredGraph(sources []Source, maxFiles int, request GraphQuery) (GraphReport, error) {
	universe, err := NewUniverse(sources, maxFiles)
	if err != nil {
		return GraphReport{}, err
	}
	defer universe.Close()
	return BuildGraph(universe, &request)
}
