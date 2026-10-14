package search

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/greppleai/grepple/internal/navigation"
	"github.com/greppleai/grepple/internal/parser"
)

func (index *navigationIndex) relatedContent(path string) string {
	if content, ok := index.contents[path]; ok {
		return content
	}
	if index.sourceContent != nil {
		content := index.sourceContent(path)
		index.contents[path] = content
		return content
	}
	return ""
}

// AttachStoredNavigation is a request-local projection, not a retained rich graph
// or repository source-content map. The host pins revision and checkout lifetime.
func AttachStoredNavigation(matches []FileMatch, snapshot navigation.StoredSnapshot, root, buildCWD string, followDepth int, outgoingOnly bool, observers ...navigation.SnapshotObserver) error {
	var observer navigation.SnapshotObserver
	if len(observers) > 0 {
		observer = observers[0]
	}
	selection := beginStoredNavigationStage(observer, "navigation.select_matches")
	selected, indexes, files, absoluteRoot, err := selectStoredMatches(matches, root, outgoingOnly)
	selection(err)
	if err != nil {
		return err
	}
	if len(selected) == 0 {
		return nil
	}
	project := beginStoredNavigationStage(observer, "navigation.graph_projection")
	graph, err := snapshot.RelatedProjection(files, buildCWD, max(1, followDepth))
	project(err)
	if err != nil {
		return err
	}
	indexStage := beginStoredNavigationStage(observer, "navigation.result_index")
	index := storedRelatedIndex(graph, selected, absoluteRoot, buildCWD)
	indexStage(nil)
	contextStage := beginStoredNavigationStage(observer, "navigation.context_preview")
	attachRelatedNavigation(selected, index, followDepth, outgoingOnly)
	contextStage(nil)
	for row, match := range selected {
		matches[indexes[row]].Related = match.Related
		matches[indexes[row]].OmittedRelatedCallers = match.OmittedRelatedCallers
		matches[indexes[row]].OmittedRelatedCallees = match.OmittedRelatedCallees
		matches[indexes[row]].OmittedRelatedTypes = match.OmittedRelatedTypes
	}
	return nil
}
func storedPathWithin(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
func indexStoredCalls(index *navigationIndex, calls []parser.NavigationCall) {
	for _, call := range calls {
		caller, ok := index.byID[call.CallerID]
		if !ok {
			continue
		}
		item := navigationCall{id: call.ID, callerID: call.CallerID, name: call.Name, display: call.Display, resolvedName: call.ResolvedName, qualifier: call.Qualifier, importPath: call.ImportPath, moduleScope: caller.moduleScope, receiverType: call.ReceiverType, rootType: call.ReceiverRootType, rootImport: call.ReceiverRootImport, receiverMembers: append([]string(nil), call.ReceiverMembers...), receiverFactory: call.ReceiverFactory, factoryImport: call.ReceiverFactoryImport, file: caller.file, language: call.Language, importSourceFile: caller.file, packageName: caller.packageName, targetID: call.TargetID, candidateTargetIDs: append([]string(nil), call.CandidateTargetIDs...), confidence: call.Confidence, line: call.Line}
		index.calls[relatedLocationKey(caller.point)] = append(index.calls[relatedLocationKey(caller.point)], item)
		if call.TargetID != "" {
			index.callersByTargetID[call.TargetID] = append(index.callersByTargetID[call.TargetID], navigationCaller{declaration: caller, call: item})
		}
	}
}

func storedRelatedIndex(graph parser.NavigationGraph, selected []FileMatch, absoluteRoot, buildCWD string) *navigationIndex {
	paths := map[string]string{}
	contents := map[string]string{}
	add := func(path string) {
		absolute := path
		if !filepath.IsAbs(absolute) {
			absolute = filepath.Join(buildCWD, path)
		}
		absolute = filepath.Clean(absolute)
		paths[filepath.Clean(path)] = absolute
		paths[absolute] = absolute
	}
	for _, declaration := range graph.Declarations {
		add(declaration.Path)
	}
	for _, declaration := range graph.TypeDeclarations {
		add(declaration.Path)
	}
	for _, fact := range graph.Imports {
		add(fact.Path)
	}
	// Populate aliases without materializing file content for every declaration.
	index := newResolvedNavigationIndex(parser.NavigationGraph{}, nil, NavigationSourceStats{})
	indexRelatedDeclarations(index, graph.Declarations, paths)
	indexRelatedTypeFacts(index, graph, paths)
	for _, match := range selected {
		absolute, _ := filepath.Abs(match.File)
		contents[absolute] = match.Content
	}
	index.contents = contents
	index.sourceContent = func(path string) string {
		absolute, err := filepath.EvalSymlinks(path)
		if err != nil || !storedPathWithin(absoluteRoot, absolute) {
			return ""
		}
		content, err := os.ReadFile(absolute)
		if err != nil {
			return ""
		}
		return string(content)
	}
	indexStoredCalls(index, graph.Calls)
	return index
}

func selectStoredMatches(matches []FileMatch, root string, outgoingOnly bool) ([]FileMatch, []int, []string, string, error) {
	var selected []FileMatch
	var indexes []int
	var files []string
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, nil, nil, "", err
	}
	for row, match := range matches {
		if !supportsNavigation(match.Language) || (outgoingOnly && !match.CallableDeclaration) {
			continue
		}
		absolute, err := filepath.Abs(match.File)
		if err != nil {
			return nil, nil, nil, "", err
		}
		if storedPathWithin(absoluteRoot, absolute) {
			selected = append(selected, match)
			indexes = append(indexes, row)
			files = append(files, absolute)
		}
	}
	return selected, indexes, files, absoluteRoot, nil
}

func beginStoredNavigationStage(observer navigation.SnapshotObserver, operation string) func(error) {
	if errors, ok := observer.(navigation.SnapshotErrorObserver); ok {
		return errors.StartError(operation)
	}
	if observer != nil {
		end := observer.Start(operation)
		return func(error) { end() }
	}
	return func(error) {}
}
