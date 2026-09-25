// Package navigation builds the shared deterministic repository navigation graph.
package navigation

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/greppleai/grepple/parser"
)

type navigationDeclaration struct {
	id               string
	name             string
	kind             string
	terminal         string
	container        string
	returnType       string
	returnImportPath string
	packageName      string
	moduleScope      string
	visibilityDetail string
	language         string
	file             string
}

type navigationCall struct {
	id, callerID                       string
	name, display, resolvedName        string
	qualifier, importPath, moduleScope string
	receiverType, receiverFactory      string
	factoryImport, file, language      string
	importSourceFile                   string
	rootType, rootImport               string
	receiverMembers                    []string
	importTargetFiles                  []string
	importTargetScopes                 []string
	promotedReceiverTypes              []string
	importedReceiverTypes              []string
	importedIdentity                   string
	packageName                        string
	line                               int
}

type navigationExport struct {
	name, importedName, importPath, scope, language, file string
}

type navigationField struct {
	ownerType, name, typeName, importPath string
	language, packageName, file           string
	line                                  int
	embedded                              bool
	targetFiles                           []string
}

type navigationIndex struct {
	declarations    map[string][]navigationDeclaration
	calls           map[string][]navigationCall
	exports         map[string][]navigationExport
	fields          map[string][]navigationField
	embeddedFields  map[string][]navigationField
	contents        map[string]string
	languageIndexes map[string]languageNavigationIndex
	graph           parser.NavigationGraph
	sourceStats     SourceStats
}

func supportsNavigation(language string) bool {
	_, supported := languageNavigationIndexFactories[navigationLanguageFamily(language)]
	return supported
}

func buildNavigationIndex(files []string, useCache bool) *navigationIndex {
	index := newNavigationIndex()
	cwd, _ := os.Getwd()
	paths := append([]string(nil), files...)
	sort.Strings(paths)
	for _, path := range paths {
		index.addFile(path, cwd, useCache)
	}
	index.finalize(paths)
	return index
}

func newNavigationIndex() *navigationIndex {
	return &navigationIndex{
		declarations: make(map[string][]navigationDeclaration),
		calls:        make(map[string][]navigationCall), exports: make(map[string][]navigationExport), fields: make(map[string][]navigationField), embeddedFields: make(map[string][]navigationField), contents: make(map[string]string),
	}
}

func (index *navigationIndex) finalize(paths []string) {
	files := make([]string, 0, len(index.contents))
	for path := range index.contents {
		files = append(files, path)
	}
	sort.Strings(files)
	corpus := &navigationCorpus{contents: index.contents, exports: index.exports, graph: index.graph, files: files}
	index.languageIndexes, index.graph.RepositoryRoots = newLanguageNavigationIndexes(corpus, paths)
	index.inferReExportTargets()
	index.inferCrossFileFieldReceivers()
	index.inferReExportTargets()
	index.inferPromotedReceiverTypes()
	index.inferCallReturnReceivers()
	index.resolveGraphCalls()
	index.resolveGraphImports()
}

// SourceStats reports the completeness of one graph build.
type SourceStats struct {
	Attempted int `json:"attempted"`
	Parsed    int `json:"parsed"`
	Skipped   int `json:"skipped"`
	Failed    int `json:"failed"`
	Recovered int `json:"recovered"`
}

// BuildOptions controls optional performance behavior without changing graph facts.
type BuildOptions struct{ DisableCache bool }

// DocumentSource pairs one caller-owned document with its path.
type DocumentSource struct {
	Path     string
	Document *parser.Document
}

// TextSource pairs source text with its repository path.
type TextSource struct{ Path, Text string }

// Analysis retains one resolved navigation graph.
type Analysis struct{ index *navigationIndex }

// BuildGraph builds the resolved graph for local files.
func BuildGraph(files []string) parser.NavigationGraph {
	graph, _ := BuildGraphWithStats(files)
	return graph
}

// BuildGraphWithStats builds the resolved graph and reports source completeness.
func BuildGraphWithStats(files []string) (parser.NavigationGraph, SourceStats) {
	return BuildGraphWithOptions(files, BuildOptions{})
}

// BuildGraphWithOptions builds the resolved graph with explicit cache behavior.
func BuildGraphWithOptions(files []string, options BuildOptions) (parser.NavigationGraph, SourceStats) {
	index := buildNavigationIndex(files, !options.DisableCache)
	return index.graph, index.sourceStats
}

// BuildAnalysisFromDocuments builds reusable analysis from caller-owned documents.
func BuildAnalysisFromDocuments(sources []DocumentSource, options BuildOptions) (*Analysis, SourceStats) {
	ordered := append([]DocumentSource(nil), sources...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Path < ordered[j].Path })
	cwd, _ := os.Getwd()
	cacheKey := ""
	if !options.DisableCache {
		if key, ok := resolvedGraphCacheKey(ordered, cwd); ok {
			cacheKey = key
			if graph, hit := readResolvedGraphCache(key); hit {
				stats := documentSourceStats(ordered)
				return &Analysis{index: &navigationIndex{graph: graph, sourceStats: stats}}, stats
			}
		}
	}
	index := newNavigationIndex()
	paths := make([]string, 0, len(ordered))
	for _, source := range ordered {
		paths = append(paths, source.Path)
		index.addDocument(source, cwd, !options.DisableCache)
	}
	index.finalize(paths)
	if cacheKey != "" && index.sourceStats.Failed == 0 {
		writeResolvedGraphCache(cacheKey, index.graph, index.sourceStats.Recovered > 0)
	}
	return &Analysis{index: index}, index.sourceStats
}

// BuildAnalysisFromTextSources builds reusable analysis from in-memory sources.
func BuildAnalysisFromTextSources(sources []TextSource, options BuildOptions) (*Analysis, SourceStats) {
	ordered := append([]TextSource(nil), sources...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Path < ordered[j].Path })
	index := newNavigationIndex()
	cwd, _ := os.Getwd()
	paths := make([]string, 0, len(ordered))
	for _, source := range ordered {
		paths = append(paths, source.Path)
		index.addTextSource(source, cwd, !options.DisableCache)
	}
	index.finalize(paths)
	return &Analysis{index: index}, index.sourceStats
}

// Graph returns the immutable resolved graph projection.
func (analysis *Analysis) Graph() parser.NavigationGraph {
	if analysis == nil || analysis.index == nil {
		return parser.NavigationGraph{}
	}
	return analysis.index.graph
}

// BuildGraphFromDocuments resolves a graph from caller-owned documents.
func BuildGraphFromDocuments(sources []DocumentSource, options BuildOptions) (parser.NavigationGraph, SourceStats) {
	analysis, stats := BuildAnalysisFromDocuments(sources, options)
	return analysis.Graph(), stats
}

// BuildGraphFromTextSources resolves a graph from in-memory sources.
func BuildGraphFromTextSources(sources []TextSource, options BuildOptions) (parser.NavigationGraph, SourceStats) {
	analysis, stats := BuildAnalysisFromTextSources(sources, options)
	return analysis.Graph(), stats
}

func (index *navigationIndex) resolveGraphCalls() {
	declarationsByID := index.navigationDeclarationsByID()
	callsByID := index.navigationCallsByID()
	for callIndex := range index.graph.Calls {
		index.resolveGraphCall(&index.graph.Calls[callIndex], callsByID, declarationsByID)
	}
}

func (index *navigationIndex) resolveGraphImports() {
	packageFiles := make(map[string][]string)
	for _, declaration := range index.graph.Declarations {
		if declaration.PackageID != "" {
			packageFiles[declaration.PackageID] = append(packageFiles[declaration.PackageID], declaration.Path)
		}
	}
	for importIndex := range index.graph.Imports {
		fact := &index.graph.Imports[importIndex]
		fact.TargetPaths = index.resolveImportTargetPaths(*fact, packageFiles)
	}
}

func (index *navigationIndex) resolveImportTargetPaths(fact parser.NavigationImport, packageFiles map[string][]string) []string {
	targets := append([]string(nil), packageFiles[fact.ImportPath]...)
	if languageIndex := index.languageIndex(fact.Language); languageIndex != nil {
		resolved := languageIndex.importTargets(fact.Path, fact.Scope, fact.ImportPath, fact.Imported, fact.Kind)
		targets = append(targets, resolved.files...)
	}
	sort.Strings(targets)
	return compactSortedStrings(targets)
}

func (index *navigationIndex) languageIndex(language string) languageNavigationIndex {
	return index.languageIndexes[navigationLanguageFamily(language)]
}

func (index *navigationIndex) navigationDeclarationsByID() map[string]navigationDeclaration {
	byID := make(map[string]navigationDeclaration)
	for _, declarations := range index.declarations {
		for _, declaration := range declarations {
			byID[declaration.id] = declaration
		}
	}
	return byID
}

func (index *navigationIndex) navigationCallsByID() map[string]navigationCall {
	byID := make(map[string]navigationCall)
	for _, calls := range index.calls {
		for _, call := range calls {
			byID[call.id] = call
		}
	}
	return byID
}

func (index *navigationIndex) addExports(exports []parser.NavigationExport, language, sourcePath string) {
	key := navigationSymbolKey(language, sourcePath)
	for _, export := range exports {
		index.exports[key] = append(index.exports[key], navigationExport{name: export.Name, importedName: export.ImportedName, importPath: export.ImportPath, scope: export.Scope, language: language, file: sourcePath})
	}
}

func (index *navigationIndex) inferReExportTargets() {
	for location, calls := range index.calls {
		for callIndex := range calls {
			index.inferReExportTarget(&calls[callIndex])
		}
		index.calls[location] = calls
	}
}

func (index *navigationIndex) inferReExportTarget(call *navigationCall) {
	if call.importedIdentity == "" {
		call.importedIdentity = navigationImportedIdentity(*call)
	}
	identity := call.importedIdentity
	languageIndex := index.languageIndex(call.language)
	if languageIndex == nil {
		return
	}
	targets := languageIndex.reExportTargets(call.file, call.moduleScope, call.importPath, identity, map[string]bool{})
	call.importTargetFiles = targets.files
	call.importTargetScopes = targets.scopes
	call.importedReceiverTypes = index.importedTypeNames(call.importTargetFiles, identity, call.language)
	if len(call.importedReceiverTypes) != 1 {
		return
	}
	if call.receiverType != "" {
		call.receiverType = call.importedReceiverTypes[0]
	} else {
		call.resolvedName = call.importedReceiverTypes[0]
	}
}

func (index *navigationIndex) importedTypeNames(files []string, name, language string) []string {
	result := []string{}
	for _, file := range files {
		for _, export := range index.exports[navigationSymbolKey(language, file)] {
			if export.name == name && export.importedName != "" && export.importedName != "*" {
				result = append(result, terminalSymbolName(export.importedName))
			}
		}
	}
	sort.Strings(result)
	return compactSortedStrings(result)
}

func navigationImportedIdentity(call navigationCall) string {
	if call.receiverType != "" {
		return terminalSymbolName(call.receiverType)
	}
	if call.rootType != "" {
		return terminalSymbolName(call.rootType)
	}
	if call.resolvedName != "" {
		return terminalSymbolName(call.resolvedName)
	}
	return terminalSymbolName(call.name)
}

func compactSortedStrings(values []string) []string {
	if len(values) < 2 {
		return values
	}
	result := values[:1]
	for _, value := range values[1:] {
		if value != result[len(result)-1] {
			result = append(result, value)
		}
	}
	return result
}
func (index *navigationIndex) resolveGraphCall(call *parser.NavigationCall, callsByID map[string]navigationCall, declarationsByID map[string]navigationDeclaration) {
	indexed, ok := callsByID[call.ID]
	if !ok {
		call.Confidence = "candidate"
		return
	}
	call.ImportPath = indexed.importPath
	call.ReceiverType = indexed.receiverType
	call.ReceiverFactory = indexed.receiverFactory
	call.ReceiverFactoryImport = indexed.factoryImport
	call.ResolvedName = indexed.resolvedName
	targetName := navigationCallTargetName(indexed)
	matched := []navigationDeclaration(nil)
	if caller, exists := declarationsByID[indexed.callerID]; exists {
		matched = []navigationDeclaration{caller}
	}
	resolution := index.resolveNavigationCandidates(indexed, index.declarations[navigationSymbolKey(call.Language, targetName)], matched)
	call.Confidence = resolution.confidence
	if call.Confidence == "" {
		call.Confidence = "candidate"
	}
	call.CandidateTargetIDs = make([]string, len(resolution.candidates))
	for candidateIndex, candidate := range resolution.candidates {
		call.CandidateTargetIDs[candidateIndex] = candidate.id
	}
	if len(resolution.candidates) == 1 && resolution.confidence != "candidate" {
		call.TargetID = resolution.candidates[0].id
		call.CandidateTargetIDs = nil
	}
}
func (index *navigationIndex) addFile(path, cwd string, useCache bool) {
	index.sourceStats.Attempted++
	language := parser.LanguageFor(path)
	if !supportsNavigation(language) {
		index.sourceStats.Skipped++
		return
	}
	contentBytes, err := os.ReadFile(path)
	if err != nil {
		index.sourceStats.Failed++
		return
	}
	if bytes.IndexByte(contentBytes, 0) >= 0 {
		index.sourceStats.Skipped++
		return
	}
	content := string(contentBytes)
	cleanPath := filepath.Clean(path)
	displayPath := displayPathFrom(path, cwd)
	var graph parser.NavigationGraph
	var recovered bool
	if useCache {
		graph, recovered, _, err = parser.CachedNavigationGraph(content, language, displayPath)
	} else {
		var document *parser.Document
		document, err = parser.ParseDocument(language, content)
		if err == nil {
			recovered = document.Root().HasError()
			graph = parser.NavigationGraphFromDocument(document, displayPath)
			document.Close()
		}
	}
	if err != nil {
		index.sourceStats.Failed++
		return
	}
	index.addParsedGraph(cleanPath, displayPath, language, content, graph, recovered)
}

func (index *navigationIndex) addDocument(source DocumentSource, cwd string, useCache bool) {
	index.sourceStats.Attempted++
	if source.Document == nil || !supportsNavigation(source.Document.Language()) {
		index.sourceStats.Skipped++
		return
	}
	cleanPath := filepath.Clean(source.Path)
	displayPath := displayPathFrom(source.Path, cwd)
	recovered := source.Document.Root().HasError()
	var graph parser.NavigationGraph
	if useCache {
		graph, _ = parser.CachedNavigationGraphFromDocument(source.Document, displayPath)
	} else {
		graph = parser.NavigationGraphFromDocument(source.Document, displayPath)
	}
	index.addParsedGraph(cleanPath, displayPath, source.Document.Language(), source.Document.Source(), graph, recovered)
}

func (index *navigationIndex) addTextSource(source TextSource, cwd string, useCache bool) {
	index.sourceStats.Attempted++
	language := parser.LanguageFor(source.Path)
	if !supportsNavigation(language) {
		index.sourceStats.Skipped++
		return
	}
	cleanPath := filepath.Clean(source.Path)
	displayPath := displayPathFrom(source.Path, cwd)
	var graph parser.NavigationGraph
	var recovered bool
	var err error
	if useCache {
		graph, recovered, _, err = parser.CachedNavigationGraph(source.Text, language, displayPath)
	} else {
		var document *parser.Document
		document, err = parser.ParseDocument(language, source.Text)
		if err == nil {
			recovered = document.Root().HasError()
			graph = parser.NavigationGraphFromDocument(document, displayPath)
			document.Close()
		}
	}
	if err != nil {
		index.sourceStats.Failed++
		return
	}
	index.addParsedGraph(cleanPath, displayPath, language, source.Text, graph, recovered)
}

func (index *navigationIndex) addParsedGraph(cleanPath, displayPath, language, content string, graph parser.NavigationGraph, recovered bool) {
	if recovered {
		index.sourceStats.Recovered++
	}
	index.sourceStats.Parsed++
	enrichNavigationRepositoryIdentity(&graph, cleanPath)
	index.graph.Merge(graph)
	index.addExports(graph.Exports, language, cleanPath)
	index.addFields(graph.Fields, language, cleanPath)
	index.contents[cleanPath] = content
	if len(graph.Declarations) == 0 {
		return
	}
	displayPath = filepath.Clean(displayPath)
	indexed := index.addDeclarations(graph.Declarations, language, cleanPath, displayPath)
	index.addCalls(graph.Calls, indexed, language, cleanPath)
}

func (index *navigationIndex) addFields(fields []parser.NavigationField, language, sourcePath string) {
	for _, field := range fields {
		item := navigationField{ownerType: terminalSymbolName(field.OwnerType), name: field.Name, typeName: field.Type, importPath: field.ImportPath, language: language, packageName: field.Package, file: sourcePath, line: field.Line, embedded: field.Embedded}
		key := navigationFieldKey(language, item.ownerType, item.name)
		index.fields[key] = append(index.fields[key], item)
		if item.embedded {
			ownerKey := navigationSymbolKey(language, item.ownerType)
			index.embeddedFields[ownerKey] = append(index.embeddedFields[ownerKey], item)
		}
	}
}

func navigationFieldKey(language, owner, name string) string {
	return navigationSymbolKey(language, terminalSymbolName(owner)+"."+name)
}

func (index *navigationIndex) inferCrossFileFieldReceivers() {
	for location, calls := range index.calls {
		for callIndex := range calls {
			index.inferCrossFileFieldReceiver(&calls[callIndex])
		}
		index.calls[location] = calls
	}
}

func (index *navigationIndex) inferCrossFileFieldReceiver(call *navigationCall) {
	if call.receiverType != "" || call.rootType == "" || len(call.receiverMembers) == 0 {
		return
	}
	binding := navigationField{typeName: call.rootType, importPath: call.rootImport, language: call.language, packageName: call.packageName, file: call.file}
	if languageIndex := index.languageIndex(call.language); languageIndex != nil {
		binding.targetFiles = languageIndex.reExportTargets(call.file, "", call.rootImport, terminalSymbolName(call.rootType), map[string]bool{}).files
	}
	for _, member := range call.receiverMembers {
		candidates := index.fields[navigationFieldKey(call.language, binding.typeName, member)]
		matched := index.filterNavigationFieldsByOrigin(candidates, binding)
		if len(matched) != 1 || matched[0].typeName == "" {
			return
		}
		binding = matched[0]
	}
	call.receiverType = binding.typeName
	call.importPath = binding.importPath
	call.importedIdentity = ""
	call.importSourceFile = binding.file
}

func (index *navigationIndex) inferPromotedReceiverTypes() {
	for location, calls := range index.calls {
		for callIndex := range calls {
			call := &calls[callIndex]
			if call.receiverType == "" {
				continue
			}
			owner := navigationField{typeName: call.receiverType, importPath: call.importPath, language: call.language, packageName: call.packageName, file: call.importSourceFile}
			call.promotedReceiverTypes = index.promotedTypes(owner, map[string]bool{})
		}
		index.calls[location] = calls
	}
}

func (index *navigationIndex) promotedTypes(owner navigationField, seen map[string]bool) []string {
	key := navigationSymbolKey(owner.language, terminalSymbolName(owner.typeName))
	if seen[key] {
		return nil
	}
	seen[key] = true
	result := []string{}
	for _, field := range index.filterNavigationFieldsByOrigin(index.embeddedFields[key], owner) {
		typeName := terminalSymbolName(field.typeName)
		if typeName == "" || stringSliceContains(result, typeName) {
			continue
		}
		result = append(result, typeName)
		result = append(result, index.promotedTypes(field, seen)...)
	}
	return result
}

func stringSliceContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func (index *navigationIndex) filterNavigationFieldsByOrigin(candidates []navigationField, owner navigationField) []navigationField {
	result := make([]navigationField, 0, len(candidates))
	for _, candidate := range candidates {
		if index.navigationFieldOriginMatches(candidate, owner) {
			result = append(result, candidate)
		}
	}
	return result
}

func (index *navigationIndex) navigationFieldOriginMatches(candidate, owner navigationField) bool {
	if len(owner.targetFiles) > 0 {
		return stringSliceContains(owner.targetFiles, candidate.file)
	}
	languageIndex := index.languageIndex(candidate.language)
	if languageIndex == nil {
		return false
	}
	if owner.importPath != "" {
		call := navigationCall{importPath: owner.importPath, importSourceFile: owner.file, file: owner.file}
		declaration := navigationDeclaration{language: candidate.language, file: candidate.file, packageName: candidate.packageName}
		return languageIndex.importMatches(call, declaration)
	}
	return languageIndex.fieldOriginMatches(candidate, owner)
}

func (index *navigationIndex) addDeclarations(declarations []parser.NavigationDeclaration, language, path, _ string) []navigationDeclaration {
	indexed := make([]navigationDeclaration, 0, len(declarations))
	for _, declaration := range declarations {
		terminal := terminalSymbolName(declaration.Name)
		if terminal == "" {
			continue
		}
		item := navigationDeclaration{
			id: declaration.ID, name: declaration.Name, kind: declaration.Kind, terminal: terminal, container: navigationDeclarationContainer(declaration), returnType: declaration.ResultType, returnImportPath: declaration.ResultImportPath, packageName: declaration.Package, moduleScope: declaration.Scope, visibilityDetail: declaration.VisibilityDetail, language: language, file: path,
		}
		indexed = append(indexed, item)
		key := navigationSymbolKey(language, terminal)
		index.declarations[key] = append(index.declarations[key], item)
	}
	return indexed
}

func (index *navigationIndex) addCalls(calls []parser.NavigationCall, declarations []navigationDeclaration, language, sourcePath string) {
	byID := map[string]navigationDeclaration{}
	for _, declaration := range declarations {
		byID[declaration.id] = declaration
	}
	for _, call := range calls {
		caller, ok := byID[call.CallerID]
		if !ok {
			continue
		}
		location := caller.id
		indexedCall := navigationCall{
			id: call.ID, callerID: call.CallerID, name: call.Name, display: call.Display, resolvedName: call.ResolvedName, qualifier: call.Qualifier, importPath: call.ImportPath, moduleScope: caller.moduleScope, receiverType: call.ReceiverType,
			rootType: call.ReceiverRootType, rootImport: call.ReceiverRootImport, receiverMembers: append([]string(nil), call.ReceiverMembers...), packageName: caller.packageName,
			receiverFactory: call.ReceiverFactory, factoryImport: call.ReceiverFactoryImport, file: sourcePath, language: language, importSourceFile: sourcePath, line: call.Line,
		}
		index.calls[location] = append(index.calls[location], indexedCall)
	}
}

func (index *navigationIndex) inferCallReturnReceivers() {
	declarationsByID := index.navigationDeclarationsByID()
	for callerID, calls := range index.calls {
		caller, ok := declarationsByID[callerID]
		if !ok {
			continue
		}
		for callIndex := range calls {
			index.inferCallReturnReceiver(&calls[callIndex], caller.language)
		}
		index.calls[callerID] = calls
	}
}

func (index *navigationIndex) inferCallReturnReceiver(call *navigationCall, language string) {
	if call.receiverType != "" || call.receiverFactory == "" {
		return
	}
	declaration, ok := index.resolveReturnFactory(*call, language)
	if !ok || declaration.returnType == "" {
		return
	}
	call.receiverType = declaration.returnType
	call.importPath = declaration.returnImportPath
	call.importSourceFile = declaration.file
	if call.importPath == "" {
		call.importPath = call.factoryImport
		call.importSourceFile = call.file
	}
}

func (index *navigationIndex) resolveReturnFactory(call navigationCall, language string) (navigationDeclaration, bool) {
	factoryCall := navigationCall{
		name: call.receiverFactory, display: call.receiverFactory, importPath: call.factoryImport,
		file: call.file, importSourceFile: call.file,
	}
	key := navigationSymbolKey(language, call.receiverFactory)
	resolution := index.resolveNavigationCandidates(factoryCall, index.declarations[key], nil)
	if len(resolution.candidates) != 1 || resolution.confidence == "candidate" {
		return navigationDeclaration{}, false
	}
	return resolution.candidates[0], true
}

type navigationCandidateResolution struct {
	candidates []navigationDeclaration
	confidence string
}

func (index *navigationIndex) resolveNavigationCandidates(call navigationCall, candidates, matched []navigationDeclaration) navigationCandidateResolution {
	if languageIndex := index.languageIndex(call.language); languageIndex != nil {
		candidates = languageIndex.filterCandidates(call, candidates)
	}
	contextConfidence := ""
	if contextual, confidence := index.navigationContextCandidates(call, candidates); len(contextual) > 0 {
		candidates = contextual
		contextConfidence = confidence
	}
	if exact := exactNavigationCandidates(call, candidates); len(exact) == 1 {
		return navigationCandidateResolution{exact, "exact"}
	} else if len(exact) > 1 {
		candidates = exact
	}
	originalCount := len(candidates)
	if functions := unqualifiedFunctionCandidates(call, candidates); len(functions) > 0 {
		candidates = functions
		if len(candidates) == 1 && originalCount > 1 {
			return navigationCandidateResolution{candidates, navigationContextConfidence(contextConfidence)}
		}
	}
	beforeLocal := len(candidates)
	if local := candidatesInMatchedFiles(candidates, matched); len(local) > 0 {
		candidates = local
		if len(candidates) == 1 && beforeLocal > 1 {
			return navigationCandidateResolution{candidates, navigationContextConfidence(contextConfidence)}
		}
	}
	if len(candidates) == 1 {
		if contextConfidence != "" {
			return navigationCandidateResolution{candidates, contextConfidence}
		}
		return navigationCandidateResolution{candidates, "unique-terminal"}
	}
	return navigationCandidateResolution{candidates, "candidate"}
}

func navigationContextConfidence(confidence string) string {
	if confidence != "" {
		return confidence
	}
	return "context-resolved"
}

func navigationCallTargetName(call navigationCall) string {
	if call.resolvedName != "" {
		return terminalSymbolName(call.resolvedName)
	}
	return call.name
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

func (index *navigationIndex) navigationContextCandidates(call navigationCall, candidates []navigationDeclaration) ([]navigationDeclaration, string) {
	result := candidates
	confidence := ""
	if call.importPath != "" {
		result = filterNavigationCandidates(result, func(candidate navigationDeclaration) bool {
			return index.navigationImportMatches(call, candidate)
		})
		confidence = "import-resolved"
	}
	if call.receiverType != "" {
		receiverCandidates := filterNavigationCandidates(result, func(candidate navigationDeclaration) bool {
			return candidate.container == terminalSymbolName(call.receiverType) || stringSliceContains(call.importedReceiverTypes, candidate.container) || stringSliceContains(call.promotedReceiverTypes, candidate.container)
		})
		if len(receiverCandidates) > 0 {
			result = receiverCandidates
			if confidence == "" {
				confidence = "context-resolved"
			}
		} else if confidence == "" {
			return nil, ""
		}
	}
	if confidence == "" || len(result) == 0 {
		return nil, ""
	}
	return result, confidence
}

func (index *navigationIndex) navigationImportMatches(call navigationCall, candidate navigationDeclaration) bool {
	languageIndex := index.languageIndex(call.language)
	return languageIndex != nil && languageIndex.importMatches(call, candidate)
}

func exactNavigationCandidates(call navigationCall, candidates []navigationDeclaration) []navigationDeclaration {
	if !strings.ContainsAny(call.display, ".:#") {
		return nil
	}
	return filterNavigationCandidates(candidates, func(candidate navigationDeclaration) bool {
		return candidate.name == call.display
	})
}

func unqualifiedFunctionCandidates(call navigationCall, candidates []navigationDeclaration) []navigationDeclaration {
	if strings.Contains(call.display, ".") {
		return nil
	}
	return filterNavigationCandidates(candidates, func(candidate navigationDeclaration) bool {
		return candidate.kind == "func" || candidate.kind == "function"
	})
}

func filterNavigationCandidates(candidates []navigationDeclaration, keep func(navigationDeclaration) bool) []navigationDeclaration {
	result := make([]navigationDeclaration, 0, len(candidates))
	for _, candidate := range candidates {
		if keep(candidate) {
			result = append(result, candidate)
		}
	}
	return result
}

func navigationCandidatesContain(candidates []navigationDeclaration, target navigationDeclaration) bool {
	for _, candidate := range candidates {
		if candidate.id == target.id {
			return true
		}
	}
	return false
}

func candidatesInMatchedFiles(candidates, matched []navigationDeclaration) []navigationDeclaration {
	files := map[string]bool{}
	for _, declaration := range matched {
		files[declaration.file] = true
	}
	return filterNavigationCandidates(candidates, func(candidate navigationDeclaration) bool {
		return files[candidate.file]
	})
}

func terminalSymbolName(name string) string {
	parts := strings.FieldsFunc(name, func(r rune) bool { return r == '.' || r == ':' || r == '#' })
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}

func isTestSourcePath(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	return strings.HasSuffix(base, "_test.go") || strings.Contains(base, ".test.") || strings.Contains(base, "_test.") || strings.HasPrefix(base, "test_")
}

func navigationSymbolKey(language, name string) string {
	return navigationLanguageFamily(language) + "\x00" + name
}

func sameNavigationLanguage(left, right string) bool {
	return navigationLanguageFamily(left) == navigationLanguageFamily(right)
}

func navigationLanguageFamily(language string) string {
	if language == "tsx" {
		return "typescript"
	}
	return language
}

func displayPathFrom(file, cwd string) string {
	r, err := filepath.Rel(cwd, file)
	if err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
		return file
	}
	if r == "." {
		return filepath.Base(file)
	}
	return filepath.ToSlash(r)
}
