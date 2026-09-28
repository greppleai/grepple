package search

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/greppleai/grepple/internal/navigation"
	"github.com/greppleai/grepple/internal/parser"
)

func buildRelatedNavigationIndexFromFiles(files []string, options NavigationBuildOptions) *navigationIndex {
	graph, stats := navigation.NewGraphEngine(options).BuildFiles(files)
	contents := make(map[string]string, len(files))
	for _, sourcePath := range files {
		content, err := os.ReadFile(sourcePath)
		if err == nil {
			contents[sourcePath] = string(content)
		}
	}
	return newResolvedNavigationIndex(graph, contents, stats)
}

func newResolvedNavigationIndex(graph parser.NavigationGraph, contents map[string]string, stats NavigationSourceStats) *navigationIndex {
	index := newNavigationIndex()
	index.graph = graph
	index.sourceStats = stats
	cwd, _ := os.Getwd()
	graphPaths := make(map[string]string, len(contents)*2)
	paths := make([]string, 0, len(contents))
	for sourcePath, content := range contents {
		cleanPath := filepath.Clean(sourcePath)
		paths = append(paths, cleanPath)
		index.contents[cleanPath] = content
		graphPaths[cleanPath] = cleanPath
		graphPaths[filepath.Clean(displayPathFrom(sourcePath, cwd))] = cleanPath
	}
	sort.Strings(paths)
	indexRelatedDeclarations(index, graph.Declarations, graphPaths)
	indexRelatedTypeFacts(index, graph, graphPaths)
	for _, call := range graph.Calls {
		caller, ok := index.byID[call.CallerID]
		if !ok {
			continue
		}
		indexed := navigationCall{
			id: call.ID, callerID: call.CallerID, name: call.Name, display: call.Display, resolvedName: call.ResolvedName, qualifier: call.Qualifier, importPath: call.ImportPath, moduleScope: caller.moduleScope, receiverType: call.ReceiverType, rootType: call.ReceiverRootType, rootImport: call.ReceiverRootImport, receiverMembers: append([]string(nil), call.ReceiverMembers...), receiverFactory: call.ReceiverFactory, factoryImport: call.ReceiverFactoryImport, file: caller.file, language: call.Language, importSourceFile: caller.file, packageName: caller.packageName, targetID: call.TargetID, candidateTargetIDs: append([]string(nil), call.CandidateTargetIDs...), confidence: call.Confidence, line: call.Line,
		}
		index.calls[relatedLocationKey(caller.point)] = append(index.calls[relatedLocationKey(caller.point)], indexed)
		if call.TargetID != "" {
			index.callersByTargetID[call.TargetID] = append(index.callersByTargetID[call.TargetID], navigationCaller{declaration: caller, call: indexed})
		}
	}
	return index
}

func indexRelatedDeclarations(index *navigationIndex, declarations []parser.NavigationDeclaration, graphPaths map[string]string) {
	for _, declaration := range declarations {
		terminal := terminalSymbolName(declaration.Name)
		if terminal == "" {
			continue
		}
		cleanPath := resolvedNavigationSourcePath(declaration.Path, graphPaths)
		item := navigationDeclaration{
			id: declaration.ID, terminal: terminal, container: navigationDeclarationContainer(declaration), returnType: declaration.ResultType, returnImportPath: declaration.ResultImportPath, packageName: declaration.Package, packageID: declaration.PackageID, moduleScope: declaration.Scope, visibilityDetail: declaration.VisibilityDetail, language: declaration.Language, file: cleanPath, matchStart: declaration.Start,
			point: RelatedPoint{Name: declaration.Name, Path: declaration.Path, File: cleanPath, Kind: declaration.Kind, Start: declaration.Start, End: declaration.End},
		}
		index.declarations[navigationSymbolKey(declaration.Language, terminal)] = append(index.declarations[navigationSymbolKey(declaration.Language, terminal)], item)
		index.byFile[cleanPath] = append(index.byFile[cleanPath], item)
		index.byLocation[relatedLocationKey(item.point)] = item
		index.byID[item.id] = item
	}
}

func indexRelatedTypeFacts(index *navigationIndex, graph parser.NavigationGraph, graphPaths map[string]string) {
	for _, declaration := range graph.TypeDeclarations {
		terminal := terminalSymbolName(declaration.Name)
		if terminal == "" {
			continue
		}
		cleanPath := resolvedNavigationSourcePath(declaration.Path, graphPaths)
		item := navigationTypeDeclaration{
			terminal: terminal, packageName: declaration.Package, packageID: declaration.PackageID, language: declaration.Language, file: cleanPath,
			point: RelatedPoint{Name: declaration.Name, Path: declaration.Path, File: cleanPath, Kind: declaration.Kind, Start: declaration.Start, End: declaration.End},
		}
		key := navigationSymbolKey(declaration.Language, terminal)
		index.types[key] = append(index.types[key], item)
	}
	for _, usage := range graph.TypeUsages {
		index.typeUsages[usage.CallerID] = append(index.typeUsages[usage.CallerID], navigationTypeUsage{typeName: usage.Type, importPath: usage.ImportPath, role: usage.Role, line: usage.Line})
	}
	for _, fact := range graph.Imports {
		indexRelatedTypeImport(index, fact, graphPaths)
	}
}

func indexRelatedTypeImport(index *navigationIndex, fact parser.NavigationImport, graphPaths map[string]string) {
	if fact.ImportPath == "" || len(fact.TargetPaths) == 0 {
		return
	}
	sourcePath := resolvedNavigationSourcePath(fact.Path, graphPaths)
	key := typeImportTargetKey(sourcePath, fact.ImportPath)
	for _, target := range fact.TargetPaths {
		index.typeImportTargets[key] = append(index.typeImportTargets[key], resolvedNavigationSourcePath(target, graphPaths))
	}
}

func resolvedNavigationSourcePath(graphPath string, paths map[string]string) string {
	if sourcePath := paths[filepath.Clean(graphPath)]; sourcePath != "" {
		return sourcePath
	}
	return filepath.Clean(graphPath)
}

func navigationDeclarationContainer(declaration parser.NavigationDeclaration) string {
	if declaration.Container != "" {
		return terminalSymbolName(declaration.Container)
	}
	parts := strings.Split(declaration.Name, ".")
	if len(parts) > 1 {
		return parts[len(parts)-2]
	}
	return ""
}

// NavigationRepositoryContextFiles is a compatibility wrapper for repository metadata discovery.
func NavigationRepositoryContextFiles(paths []string) []string {
	return navigation.NewGraphEngine(navigation.BuildOptions{}).RepositoryContextFiles(paths)
}
