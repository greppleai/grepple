package search

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/greppleai/grepple/parser"
)

const (
	maxRelatedPoints      = 5
	maxFollowedPerLevel   = 2
	maxFollowedTotalLines = 400
)

type navigationDeclaration struct {
	id               string
	terminal         string
	container        string
	returnType       string
	returnImportPath string
	packageName      string
	language         string
	file             string
	matchStart       int
	point            RelatedPoint
}

type navigationCaller struct {
	declaration navigationDeclaration
	call        navigationCall
}

type navigationCall struct {
	id, callerID                      string
	name, display, resolvedName       string
	qualifier, importPath             string
	receiverType, receiverFactory     string
	factoryImport, file               string
	importSourceFile, importDirectory string
	moduleKnown                       bool
	line                              int
}

type navigationIndex struct {
	declarations map[string][]navigationDeclaration
	callers      map[string][]navigationCaller
	calls        map[string][]navigationCall
	byFile       map[string][]navigationDeclaration
	byLocation   map[string]navigationDeclaration
	contents     map[string]string
	graph        parser.NavigationGraph
	sourceStats  NavigationSourceStats
}

func hasNavigationMatch(matches []FileMatch) bool {
	for _, match := range matches {
		if supportsNavigation(match.Language) {
			return true
		}
	}
	return false
}

func supportsNavigation(language string) bool {
	switch navigationLanguageFamily(language) {
	case "go", "javascript", "typescript", "python", "java", "kotlin", "csharp", "c", "cpp", "rust", "shell":
		return true
	}
	return false
}

func attachRelated(matches []FileMatch, candidates []string, followDepth int) {
	if !hasNavigationMatch(matches) {
		return
	}
	navigation := buildNavigationIndex(candidates)
	if len(navigation.declarations) == 0 {
		return
	}
	for index := range matches {
		if !supportsNavigation(matches[index].Language) {
			continue
		}
		related, omittedCallers, omittedCallees := relatedPoints(matches[index], navigation)
		if followDepth > 0 {
			seen := matchedLocations(matches[index], navigation)
			lineBudget := maxFollowedTotalLines
			related = expandRelated(related, navigation, followDepth, seen, &lineBudget)
		}
		matches[index].Related = related
		matches[index].OmittedRelatedCallers = omittedCallers
		matches[index].OmittedRelatedCallees = omittedCallees
	}
}

func buildNavigationIndex(files []string) *navigationIndex {
	index := &navigationIndex{
		declarations: make(map[string][]navigationDeclaration), callers: make(map[string][]navigationCaller),
		calls: make(map[string][]navigationCall), byFile: make(map[string][]navigationDeclaration),
		byLocation: make(map[string]navigationDeclaration), contents: make(map[string]string),
	}
	cwd, _ := os.Getwd()
	paths := append([]string(nil), files...)
	sort.Strings(paths)
	for _, path := range paths {
		index.addFile(path, cwd)
	}
	index.inferCallReturnReceivers()
	index.indexNavigationCallers()
	index.resolveGraphCalls()
	return index
}

// NavigationSourceStats reports the completeness of one navigation graph build.
type NavigationSourceStats struct {
	Attempted int `json:"attempted"`
	Parsed    int `json:"parsed"`
	Skipped   int `json:"skipped"`
	Failed    int `json:"failed"`
	Recovered int `json:"recovered"`
}

// BuildNavigationGraph builds the resolved, deterministic navigation graph for local files.
func BuildNavigationGraph(files []string) parser.NavigationGraph {
	graph, _ := BuildNavigationGraphWithStats(files)
	return graph
}

// BuildNavigationGraphWithStats builds the graph and reports source completeness.
func BuildNavigationGraphWithStats(files []string) (parser.NavigationGraph, NavigationSourceStats) {
	index := buildNavigationIndex(files)
	return index.graph, index.sourceStats
}

func (index *navigationIndex) resolveGraphCalls() {
	declarationsByID := index.navigationDeclarationsByID()
	callsByID := index.navigationCallsByID()
	for callIndex := range index.graph.Calls {
		index.resolveGraphCall(&index.graph.Calls[callIndex], callsByID, declarationsByID)
	}
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
	resolution := resolveNavigationCandidates(indexed, index.declarations[navigationSymbolKey(call.Language, targetName)], matched)
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
func (index *navigationIndex) addFile(path, cwd string) {
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
	document, err := parser.ParseDocument(language, content)
	if err != nil {
		index.sourceStats.Failed++
		return
	}
	if document.Root().HasError() {
		index.sourceStats.Recovered++
	}
	graph := parser.NavigationGraphFromDocument(document, displayPath)
	document.Close()
	index.sourceStats.Parsed++
	index.graph.Merge(graph)
	if len(graph.Declarations) == 0 {
		return
	}
	displayPath = filepath.Clean(displayPath)
	index.contents[cleanPath] = content
	indexed := index.addDeclarations(graph.Declarations, language, cleanPath, displayPath)
	index.addCalls(graph.Calls, indexed, language, cleanPath)
}

func (index *navigationIndex) addDeclarations(declarations []parser.NavigationDeclaration, language, path, displayPath string) []navigationDeclaration {
	indexed := make([]navigationDeclaration, 0, len(declarations))
	for _, declaration := range declarations {
		terminal := terminalSymbolName(declaration.Name)
		if terminal == "" {
			continue
		}
		item := navigationDeclaration{
			id: declaration.ID, terminal: terminal, container: navigationDeclarationContainer(declaration), returnType: declaration.ResultType, returnImportPath: declaration.ResultImportPath, packageName: declaration.Package, language: language, file: path, matchStart: declaration.Start,
			point: RelatedPoint{Name: declaration.Name, Path: displayPath, File: path, Kind: declaration.Kind, Start: declaration.Start, End: declaration.End},
		}
		indexed = append(indexed, item)
		key := navigationSymbolKey(language, terminal)
		index.declarations[key] = append(index.declarations[key], item)
		index.byFile[path] = append(index.byFile[path], item)
		index.byLocation[relatedLocationKey(item.point)] = item
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
		location := relatedLocationKey(caller.point)
		indexedCall := navigationCall{
			id: call.ID, callerID: call.CallerID, name: call.Name, display: call.Display, resolvedName: call.ResolvedName, qualifier: call.Qualifier, importPath: call.ImportPath, receiverType: call.ReceiverType,
			receiverFactory: call.ReceiverFactory, factoryImport: call.ReceiverFactoryImport, file: sourcePath, importSourceFile: sourcePath, line: call.Line,
		}
		resolveNavigationCallImport(&indexedCall, language)
		index.calls[location] = append(index.calls[location], indexedCall)
	}
}

func (index *navigationIndex) inferCallReturnReceivers() {
	for location, calls := range index.calls {
		caller, ok := index.byLocation[location]
		if !ok {
			continue
		}
		for callIndex := range calls {
			index.inferCallReturnReceiver(&calls[callIndex], caller.language)
		}
		index.calls[location] = calls
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
	resolveNavigationCallImport(call, declaration.language)
}

func (index *navigationIndex) resolveReturnFactory(call navigationCall, language string) (navigationDeclaration, bool) {
	factoryCall := navigationCall{
		name: call.receiverFactory, display: call.receiverFactory, importPath: call.factoryImport,
		file: call.file, importSourceFile: call.file,
	}
	resolveNavigationCallImport(&factoryCall, language)
	key := navigationSymbolKey(language, call.receiverFactory)
	resolution := resolveNavigationCandidates(factoryCall, index.declarations[key], nil)
	if len(resolution.candidates) != 1 || resolution.confidence == "candidate" {
		return navigationDeclaration{}, false
	}
	return resolution.candidates[0], true
}

func (index *navigationIndex) indexNavigationCallers() {
	locations := make([]string, 0, len(index.calls))
	for location := range index.calls {
		locations = append(locations, location)
	}
	sort.Strings(locations)
	for _, location := range locations {
		caller, ok := index.byLocation[location]
		if !ok {
			continue
		}
		for _, call := range index.calls[location] {
			key := navigationSymbolKey(caller.language, navigationCallTargetName(call))
			index.callers[key] = append(index.callers[key], navigationCaller{declaration: caller, call: call})
		}
	}
}

func resolveNavigationCallImport(call *navigationCall, language string) {
	if navigationLanguageFamily(language) != "go" || call.importPath == "" {
		return
	}
	call.importDirectory, call.moduleKnown = localGoImportDirectory(call.importSourceFile, call.importPath)
}

func relatedPoints(match FileMatch, navigation *navigationIndex) ([]RelatedPoint, int, int) {
	declarations := matchedDeclarations(match, navigation)
	callees, omittedCallees := relatedCallees(match, declarations, navigation)
	callers, omittedCallers := navigationCallers(declarations, navigation)
	return append(callees, callers...), omittedCallers, omittedCallees
}

func relatedCallees(match FileMatch, declarations []navigationDeclaration, navigation *navigationIndex) ([]RelatedPoint, int) {
	var calls []navigationCall
	for _, declaration := range declarations {
		calls = append(calls, navigation.calls[relatedLocationKey(declaration.point)]...)
	}
	sort.SliceStable(calls, func(i, j int) bool {
		left := nearestHitDistance(calls[i].line, match.MatchLines)
		right := nearestHitDistance(calls[j].line, match.MatchLines)
		if left != right {
			return left < right
		}
		return calls[i].line < calls[j].line
	})

	var related []RelatedPoint
	for _, call := range calls {
		related = append(related, resolveCallee(call, match.Language, declarations, navigation)...)
	}
	return limitRelatedPoints(uniqueRelatedPoints(related))
}
func resolveCallee(call navigationCall, language string, matched []navigationDeclaration, navigation *navigationIndex) []RelatedPoint {
	targetName := navigationCallTargetName(call)
	resolution := resolveNavigationCandidates(call, navigation.declarations[navigationSymbolKey(language, targetName)], matched)
	resolved := make([]RelatedPoint, 0, len(resolution.candidates))
	for _, declaration := range resolution.candidates {
		if !sameNavigationLanguage(declaration.language, language) || declarationIsMatched(declaration, matched) {
			continue
		}
		point := declaration.point
		point.Direction = "callee"
		point.CallLine = call.line
		point.Confidence = resolution.confidence
		if call.display != "" && call.display != point.Name {
			point.Name = call.display + " → " + point.Name
		}
		resolved = append(resolved, point)
	}
	return resolved
}

type navigationCandidateResolution struct {
	candidates []navigationDeclaration
	confidence string
}

func resolveNavigationCandidates(call navigationCall, candidates, matched []navigationDeclaration) navigationCandidateResolution {
	contextConfidence := ""
	if contextual, confidence := navigationContextCandidates(call, candidates); len(contextual) > 0 {
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

func navigationContextCandidates(call navigationCall, candidates []navigationDeclaration) ([]navigationDeclaration, string) {
	result := candidates
	confidence := ""
	if call.importPath != "" {
		result = filterNavigationCandidates(result, func(candidate navigationDeclaration) bool {
			return navigationImportMatches(call, candidate)
		})
		confidence = "import-resolved"
	}
	if call.receiverType != "" {
		result = filterNavigationCandidates(result, func(candidate navigationDeclaration) bool {
			return candidate.container == terminalSymbolName(call.receiverType)
		})
		if confidence == "" {
			confidence = "context-resolved"
		}
	}
	if confidence == "" || len(result) == 0 {
		return nil, ""
	}
	return result, confidence
}

func navigationImportMatches(call navigationCall, candidate navigationDeclaration) bool {
	if navigationLanguageFamily(candidate.language) == "go" {
		if call.moduleKnown {
			return call.importDirectory != "" && filepath.Clean(filepath.Dir(candidate.file)) == call.importDirectory
		}
		return candidate.packageName == filepath.Base(filepath.FromSlash(call.importPath))
	}
	if !strings.HasPrefix(call.importPath, ".") {
		return false
	}
	imported := filepath.Clean(filepath.Join(filepath.Dir(call.importSourceFile), filepath.FromSlash(call.importPath)))
	candidatePath := strings.TrimSuffix(filepath.Clean(candidate.file), filepath.Ext(candidate.file))
	imported = strings.TrimSuffix(imported, filepath.Ext(imported))
	return candidatePath == imported || filepath.Base(candidatePath) == "index" && filepath.Dir(candidatePath) == imported
}

func localGoImportDirectory(sourceFile, importPath string) (string, bool) {
	directory := filepath.Dir(sourceFile)
	for {
		modulePath, ok := goModulePath(filepath.Join(directory, "go.mod"))
		if ok {
			if importPath == modulePath {
				return filepath.Clean(directory), true
			}
			prefix := modulePath + "/"
			if strings.HasPrefix(importPath, prefix) {
				relative := strings.TrimPrefix(importPath, prefix)
				return filepath.Clean(filepath.Join(directory, filepath.FromSlash(relative))), true
			}
			return "", true
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return "", false
		}
		directory = parent
	}
}

func goModulePath(path string) (string, bool) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	for _, line := range strings.Split(string(content), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "module" {
			return strings.TrimSpace(fields[1]), true
		}
	}
	return "", false
}

func exactNavigationCandidates(call navigationCall, candidates []navigationDeclaration) []navigationDeclaration {
	if !strings.ContainsAny(call.display, ".:#") {
		return nil
	}
	return filterNavigationCandidates(candidates, func(candidate navigationDeclaration) bool {
		return candidate.point.Name == call.display
	})
}

func unqualifiedFunctionCandidates(call navigationCall, candidates []navigationDeclaration) []navigationDeclaration {
	if strings.Contains(call.display, ".") {
		return nil
	}
	return filterNavigationCandidates(candidates, func(candidate navigationDeclaration) bool {
		return candidate.point.Kind == "func" || candidate.point.Kind == "function"
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

func navigationCallers(targets []navigationDeclaration, navigation *navigationIndex) ([]RelatedPoint, int) {
	var related []RelatedPoint
	for _, target := range targets {
		terminalCandidates := navigation.declarations[navigationSymbolKey(target.language, target.terminal)]
		for _, caller := range navigation.callers[navigationSymbolKey(target.language, target.terminal)] {
			if !sameNavigationLanguage(caller.declaration.language, target.language) || relatedLocationKey(caller.declaration.point) == relatedLocationKey(target.point) {
				continue
			}
			point := caller.declaration.point
			point.Direction = "caller"
			point.CallLine = caller.call.line
			point.Confidence = navigationCallerConfidence(target, caller, terminalCandidates)
			if point.Confidence == "" {
				continue
			}
			related = append(related, point)
		}
	}
	sort.SliceStable(related, func(i, j int) bool {
		leftTest := isTestSourcePath(related[i].Path)
		rightTest := isTestSourcePath(related[j].Path)
		if leftTest != rightTest {
			return !leftTest
		}
		if related[i].Path != related[j].Path {
			return related[i].Path < related[j].Path
		}
		return related[i].CallLine < related[j].CallLine
	})
	return limitRelatedPoints(uniqueRelatedPoints(related))
}

func navigationCallerConfidence(target navigationDeclaration, caller navigationCaller, terminalCandidates []navigationDeclaration) string {
	if strings.ContainsAny(caller.call.display, ".:#") && caller.call.display == target.point.Name {
		return "exact"
	}
	if contextual, confidence := navigationContextCandidates(caller.call, terminalCandidates); len(contextual) > 0 {
		if !navigationCandidatesContain(contextual, target) {
			return ""
		}
		if len(contextual) == 1 {
			return confidence
		}
		return "candidate"
	}
	if len(terminalCandidates) == 1 {
		return "unique-terminal"
	}
	return "candidate"
}

func resolvedNavigationConfidence(confidence string) bool {
	return confidence == "exact" || confidence == "context-resolved" || confidence == "import-resolved" || confidence == "unique-terminal" || confidence == "unique"
}

func matchedDeclarations(match FileMatch, navigation *navigationIndex) []navigationDeclaration {
	var matched []navigationDeclaration
	for _, declaration := range navigation.byFile[filepath.Clean(match.File)] {
		if rangeHasHit(declaration.matchStart, declaration.point.End, match.MatchLines) {
			matched = append(matched, declaration)
		}
	}
	return narrowestDeclarations(matched, match.MatchLines)
}
func narrowestDeclarations(declarations []navigationDeclaration, hits map[int]bool) []navigationDeclaration {
	filtered := make([]navigationDeclaration, 0, len(declarations))
	for _, candidate := range declarations {
		if navigationDeclarationContainsMatchedChild(candidate, declarations, hits) {
			continue
		}
		filtered = append(filtered, candidate)
	}
	return filtered
}

func navigationDeclarationContainsMatchedChild(candidate navigationDeclaration, declarations []navigationDeclaration, hits map[int]bool) bool {
	for _, other := range declarations {
		strictlyInside := other.matchStart >= candidate.matchStart && other.point.End <= candidate.point.End && (other.matchStart > candidate.matchStart || other.point.End < candidate.point.End)
		if strictlyInside && rangeHasHit(other.matchStart, other.point.End, hits) {
			return true
		}
	}
	return false
}

func matchedLocations(match FileMatch, navigation *navigationIndex) map[string]bool {
	seen := make(map[string]bool)
	for _, declaration := range matchedDeclarations(match, navigation) {
		seen[relatedLocationKey(declaration.point)] = true
	}
	return seen
}

func expandRelated(points []RelatedPoint, navigation *navigationIndex, depth int, seen map[string]bool, lineBudget *int) []RelatedPoint {
	if depth <= 0 {
		return points
	}
	followed := 0
	for index := range points {
		point := &points[index]
		if followed >= maxFollowedPerLevel || point.Direction != "callee" || !resolvedNavigationConfidence(point.Confidence) {
			continue
		}
		key := relatedLocationKey(*point)
		declaration, ok := navigation.byLocation[key]
		if !ok || seen[key] {
			continue
		}
		lines := declaration.point.End - declaration.matchStart + 1
		if lines > *lineBudget {
			continue
		}
		*lineBudget -= lines
		seen[key] = true
		content := navigation.contents[declaration.file]
		match := FileMatch{
			File: declaration.file, DisplayPath: point.Path, Content: content, Language: declaration.language,
			MatchLines: map[int]bool{point.Start: true},
		}
		nested, omittedCallers, omittedCallees := relatedPoints(match, navigation)
		nested = expandRelated(nested, navigation, depth-1, copyLocations(seen), lineBudget)
		point.Preview = &RelatedPreview{
			Content: content, Start: declaration.matchStart, End: declaration.point.End, Related: nested,
			OmittedCallers: omittedCallers, OmittedCallees: omittedCallees,
		}
		followed++
	}
	return points
}

func declarationIsMatched(candidate navigationDeclaration, matched []navigationDeclaration) bool {
	for _, declaration := range matched {
		if relatedLocationKey(candidate.point) == relatedLocationKey(declaration.point) {
			return true
		}
	}
	return false
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

func uniqueRelatedPoints(points []RelatedPoint) []RelatedPoint {
	seen := make(map[string]bool)
	unique := points[:0]
	for _, point := range points {
		key := relatedLocationKey(point)
		if seen[key] {
			continue
		}
		seen[key] = true
		unique = append(unique, point)
	}
	return unique
}

func limitRelatedPoints(points []RelatedPoint) ([]RelatedPoint, int) {
	if len(points) > maxRelatedPoints {
		return points[:maxRelatedPoints], len(points) - maxRelatedPoints
	}
	return points, 0
}

func rangeHasHit(start, end int, hits map[int]bool) bool {
	for line := range hits {
		if line >= start && line <= end {
			return true
		}
	}
	return false
}

func nearestHitDistance(line int, hits map[int]bool) int {
	distance := int(^uint(0) >> 1)
	for hit := range hits {
		candidate := line - hit
		if candidate < 0 {
			candidate = -candidate
		}
		if candidate < distance {
			distance = candidate
		}
	}
	return distance
}

func copyLocations(source map[string]bool) map[string]bool {
	cloned := make(map[string]bool, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}

func relatedLocationKey(point RelatedPoint) string {
	return fmt.Sprintf("%s:%d:%d", filepath.Clean(point.File), point.Start, point.End)
}
