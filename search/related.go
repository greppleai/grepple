package search

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"

	"github.com/greppleai/grepple/navigation"
	"github.com/greppleai/grepple/parser"
)

var relatedBuildInvocations atomic.Int64

const (
	maxRelatedPoints      = 5
	maxRelatedTypes       = 5
	maxRelatedTypeLines   = 240
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
	packageID        string
	moduleScope      string
	visibilityDetail string
	language         string
	file             string
	matchStart       int
	point            RelatedPoint
}

type navigationTypeDeclaration struct {
	terminal, packageName, packageID, language, file string
	point                                            RelatedPoint
}

type navigationTypeUsage struct {
	typeName, importPath, role string
	line                       int
}

type navigationCaller struct {
	declaration navigationDeclaration
	call        navigationCall
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
	targetID, confidence               string
	candidateTargetIDs                 []string
	line                               int
}

type navigationIndex struct {
	declarations      map[string][]navigationDeclaration
	types             map[string][]navigationTypeDeclaration
	typeUsages        map[string][]navigationTypeUsage
	typeImportTargets map[string][]string
	callersByTargetID map[string][]navigationCaller
	calls             map[string][]navigationCall
	byFile            map[string][]navigationDeclaration
	byLocation        map[string]navigationDeclaration
	byID              map[string]navigationDeclaration
	contents          map[string]string
	graph             parser.NavigationGraph
	sourceStats       NavigationSourceStats
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
	capabilities, supported := parser.CapabilitiesForLanguage(language)
	return supported && capabilities.Navigation
}

func attachRelated(matches []FileMatch, candidates []string, followDepth int) {
	if !hasNavigationMatch(matches) {
		return
	}
	relatedBuildInvocations.Add(1)
	navigation := buildNavigationIndex(candidates, true)
	if len(navigation.declarations) == 0 {
		return
	}
	for index := range matches {
		if !supportsNavigation(matches[index].Language) {
			continue
		}
		related, omittedCallers, omittedCallees, omittedTypes := relatedPoints(matches[index], navigation)
		if followDepth > 0 {
			seen := matchedLocations(matches[index], navigation)
			lineBudget := maxFollowedTotalLines
			related = expandRelated(related, navigation, followDepth, seen, &lineBudget)
		}
		matches[index].Related = related
		matches[index].OmittedRelatedCallers = omittedCallers
		matches[index].OmittedRelatedCallees = omittedCallees
		matches[index].OmittedRelatedTypes = omittedTypes
	}
}

func buildNavigationIndex(files []string, useCache bool) *navigationIndex {
	return buildRelatedNavigationIndexFromFiles(files, NavigationBuildOptions{DisableCache: !useCache})
}

func newNavigationIndex() *navigationIndex {
	return &navigationIndex{
		declarations: make(map[string][]navigationDeclaration), types: make(map[string][]navigationTypeDeclaration), typeUsages: make(map[string][]navigationTypeUsage), typeImportTargets: make(map[string][]string), callersByTargetID: make(map[string][]navigationCaller),
		calls: make(map[string][]navigationCall), byFile: make(map[string][]navigationDeclaration),
		byLocation: make(map[string]navigationDeclaration), byID: make(map[string]navigationDeclaration), contents: make(map[string]string),
	}
}

// NavigationSourceStats reports the completeness of one navigation graph build.
type NavigationSourceStats = navigation.SourceStats

// BuildNavigationGraph builds the resolved, deterministic navigation graph for local files.
func BuildNavigationGraph(files []string) parser.NavigationGraph {
	return navigation.BuildGraph(files)
}

// BuildNavigationGraphWithStats builds the graph and reports source completeness.
func BuildNavigationGraphWithStats(files []string) (parser.NavigationGraph, NavigationSourceStats) {
	return navigation.BuildGraphWithStats(files)
}

// NavigationBuildOptions controls optional performance behavior without changing graph facts.
type NavigationBuildOptions = navigation.BuildOptions

// BuildNavigationGraphWithOptions builds the graph with explicit cache behavior.
func BuildNavigationGraphWithOptions(files []string, options NavigationBuildOptions) (parser.NavigationGraph, NavigationSourceStats) {
	return navigation.BuildGraphWithOptions(files, options)
}

// NavigationDocumentSource pairs one caller-owned parsed document with its path.
type NavigationDocumentSource = navigation.DocumentSource

// NavigationTextSource pairs source text with its repository path.
type NavigationTextSource = navigation.TextSource

// NavigationAnalysis retains one resolved navigation index for repeated read-only projections.
// Documents used to build it remain owned by the caller.
type NavigationAnalysis struct {
	index *navigationIndex
}

// BuildNavigationAnalysisFromDocuments builds one reusable analysis from caller-owned documents.
func BuildNavigationAnalysisFromDocuments(sources []NavigationDocumentSource, options NavigationBuildOptions) (*NavigationAnalysis, NavigationSourceStats) {
	shared, stats := navigation.BuildAnalysisFromDocuments(sources, options)
	contents := make(map[string]string, len(sources))
	for _, source := range sources {
		if source.Document != nil {
			contents[source.Path] = source.Document.Source()
		}
	}
	return &NavigationAnalysis{index: newResolvedNavigationIndex(shared.Graph(), contents, stats)}, stats
}

// BuildNavigationAnalysisFromTextSources builds one reusable analysis from in-memory sources.
func BuildNavigationAnalysisFromTextSources(sources []NavigationTextSource, options NavigationBuildOptions) (*NavigationAnalysis, NavigationSourceStats) {
	shared, stats := navigation.BuildAnalysisFromTextSources(sources, options)
	contents := make(map[string]string, len(sources))
	for _, source := range sources {
		contents[source.Path] = source.Text
	}
	return &NavigationAnalysis{index: newResolvedNavigationIndex(shared.Graph(), contents, stats)}, stats
}

// Graph returns the analysis's immutable resolved navigation graph projection.
func (analysis *NavigationAnalysis) Graph() parser.NavigationGraph {
	if analysis == nil || analysis.index == nil {
		return parser.NavigationGraph{}
	}
	return analysis.index.graph
}

// AttachRelatedFromAnalysis adds bounded navigation evidence without parsing or rebuilding the graph.
func AttachRelatedFromAnalysis(match *FileMatch, analysis *NavigationAnalysis, followDepth int) {
	if match == nil || analysis == nil || analysis.index == nil || !match.CallableDeclaration || !supportsNavigation(match.Language) {
		return
	}
	related, omittedCallers, omittedCallees, omittedTypes := relatedPoints(*match, analysis.index)
	if followDepth > 0 {
		seen := matchedLocations(*match, analysis.index)
		lineBudget := maxFollowedTotalLines
		related = expandRelated(related, analysis.index, followDepth, seen, &lineBudget)
	}
	match.Related = related
	match.OmittedRelatedCallers = omittedCallers
	match.OmittedRelatedCallees = omittedCallees
	match.OmittedRelatedTypes = omittedTypes
}

// BuildNavigationGraphFromDocuments resolves a graph from already parsed documents.
func BuildNavigationGraphFromDocuments(sources []NavigationDocumentSource, options NavigationBuildOptions) (parser.NavigationGraph, NavigationSourceStats) {
	return navigation.BuildGraphFromDocuments(sources, options)
}

// BuildNavigationGraphFromTextSources resolves a graph from in-memory sources.
func BuildNavigationGraphFromTextSources(sources []NavigationTextSource, options NavigationBuildOptions) (parser.NavigationGraph, NavigationSourceStats) {
	return navigation.BuildGraphFromTextSources(sources, options)
}

func relatedPoints(match FileMatch, navigation *navigationIndex) ([]RelatedPoint, int, int, int) {
	declarations := matchedDeclarations(match, navigation)
	types, omittedTypes := relatedTypes(declarations, navigation)
	callees, omittedCallees := relatedCallees(match, declarations, navigation)
	callers, omittedCallers := navigationCallers(declarations, navigation)
	return append(append(types, callees...), callers...), omittedCallers, omittedCallees, omittedTypes
}

func relatedTypes(declarations []navigationDeclaration, navigation *navigationIndex) ([]RelatedPoint, int) {
	var related []RelatedPoint
	lineBudget := maxRelatedTypeLines
	for _, declaration := range declarations {
		related = append(related, relatedTypesForDeclaration(declaration, navigation, &lineBudget)...)
	}
	return limitRelatedTypePoints(uniqueRelatedPoints(related))
}

func relatedTypesForDeclaration(declaration navigationDeclaration, navigation *navigationIndex, lineBudget *int) []RelatedPoint {
	usages := append([]navigationTypeUsage(nil), navigation.typeUsages[declaration.id]...)
	sort.SliceStable(usages, func(i, j int) bool { return relatedTypeUsageLess(usages[i], usages[j]) })
	var related []RelatedPoint
	for _, usage := range usages {
		candidates, confidence := resolveRelatedType(usage, declaration, navigation)
		if len(candidates) == 0 && externalNavigationEligible(declaration.language, usage.importPath) {
			reference := newExternalNavigationReference(declaration.language, usage.importPath, usage.typeName, declaration.packageID, "", "type", usage.line)
			related = append(related, RelatedPoint{Name: usage.typeName, Path: "dependency:" + usage.importPath, Kind: "external", Direction: "type", Confidence: "dependency-unresolved", Role: usage.role, CallLine: usage.line, External: reference})
			continue
		}
		for _, candidate := range candidates {
			point := candidate.point
			point.Name, point.Direction, point.Role = usage.typeName, "type", usage.role
			point.CallLine, point.Confidence = usage.line, confidence
			lines := point.End - point.Start + 1
			if lines > 0 && lines <= *lineBudget {
				point.Preview = &RelatedPreview{Content: navigation.contents[candidate.file], Start: point.Start, End: point.End}
				*lineBudget -= lines
			}
			related = append(related, point)
		}
	}
	return related
}

func relatedTypeUsageLess(left, right navigationTypeUsage) bool {
	leftRole, rightRole := relatedTypeRolePriority(left.role), relatedTypeRolePriority(right.role)
	if leftRole != rightRole {
		return leftRole < rightRole
	}
	if left.typeName != right.typeName {
		return left.typeName < right.typeName
	}
	return left.line < right.line
}

func relatedTypeRolePriority(role string) int {
	switch role {
	case "receiver":
		return 0
	case "parameter":
		return 1
	case "result":
		return 2
	default:
		return 3
	}
}

func resolveRelatedType(usage navigationTypeUsage, declaration navigationDeclaration, navigation *navigationIndex) ([]navigationTypeDeclaration, string) {
	terminal := terminalSymbolName(usage.typeName)
	candidates := append([]navigationTypeDeclaration(nil), navigation.types[navigationSymbolKey(declaration.language, terminal)]...)
	if usage.importPath != "" {
		return resolveImportedRelatedType(candidates, usage, declaration, navigation)
	}
	return resolveContextualRelatedType(candidates, declaration)
}

func resolveImportedRelatedType(candidates []navigationTypeDeclaration, usage navigationTypeUsage, declaration navigationDeclaration, navigation *navigationIndex) ([]navigationTypeDeclaration, string) {
	targets := navigation.typeImportTargets[typeImportTargetKey(declaration.file, usage.importPath)]
	selected := filterRelatedTypes(candidates, declaration.language, func(candidate navigationTypeDeclaration) bool {
		return candidate.packageID == usage.importPath || stringSliceContains(targets, candidate.file)
	})
	if len(selected) == 0 {
		return nil, ""
	}
	return selected, relatedTypeConfidence(selected, "import-resolved")
}

func resolveContextualRelatedType(candidates []navigationTypeDeclaration, declaration navigationDeclaration) ([]navigationTypeDeclaration, string) {
	filter := func(matches func(navigationTypeDeclaration) bool) []navigationTypeDeclaration {
		return filterRelatedTypes(candidates, declaration.language, matches)
	}
	if selected := filter(func(candidate navigationTypeDeclaration) bool { return candidate.file == declaration.file }); len(selected) > 0 {
		return selected, relatedTypeConfidence(selected, "context-resolved")
	}
	if declaration.packageID != "" {
		if selected := filter(func(candidate navigationTypeDeclaration) bool { return candidate.packageID == declaration.packageID }); len(selected) > 0 {
			return selected, relatedTypeConfidence(selected, "context-resolved")
		}
	}
	selected := filter(func(candidate navigationTypeDeclaration) bool {
		return declaration.packageName != "" && candidate.packageName == declaration.packageName && filepath.Dir(candidate.file) == filepath.Dir(declaration.file)
	})
	if len(selected) > 0 {
		return selected, relatedTypeConfidence(selected, "context-resolved")
	}
	selected = filter(func(navigationTypeDeclaration) bool { return true })
	if len(selected) == 1 {
		return selected, "unique-terminal"
	}
	return selected, "candidate"
}

func filterRelatedTypes(candidates []navigationTypeDeclaration, language string, matches func(navigationTypeDeclaration) bool) []navigationTypeDeclaration {
	var selected []navigationTypeDeclaration
	for _, candidate := range candidates {
		if sameNavigationLanguage(candidate.language, language) && matches(candidate) {
			selected = append(selected, candidate)
		}
	}
	return selected
}

func relatedTypeConfidence(candidates []navigationTypeDeclaration, resolved string) string {
	if len(candidates) == 1 {
		return resolved
	}
	return "candidate"
}

func stringSliceContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func typeImportTargetKey(path, importPath string) string {
	return filepath.Clean(path) + "\x00" + importPath
}

func limitRelatedTypePoints(points []RelatedPoint) ([]RelatedPoint, int) {
	if len(points) > maxRelatedTypes {
		return points[:maxRelatedTypes], len(points) - maxRelatedTypes
	}
	return points, 0
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
	targetIDs := append([]string(nil), call.candidateTargetIDs...)
	confidence := call.confidence
	if call.targetID != "" {
		targetIDs = []string{call.targetID}
	}
	resolved := make([]RelatedPoint, 0, len(targetIDs))
	for _, targetID := range targetIDs {
		declaration, ok := navigation.byID[targetID]
		if !ok || !sameNavigationLanguage(declaration.language, language) || declarationIsMatched(declaration, matched) {
			continue
		}
		point := declaration.point
		point.Direction = "callee"
		point.CallLine = call.line
		point.Confidence = confidence
		if point.Confidence == "" {
			point.Confidence = "candidate"
		}
		if call.display != "" && call.display != point.Name {
			point.Name = call.display + " → " + point.Name
		}
		resolved = append(resolved, point)
	}
	if len(resolved) == 0 {
		if external := externalCalleePoint(call, language, matched); external != nil {
			resolved = append(resolved, *external)
		}
	}
	return resolved
}

func externalCalleePoint(call navigationCall, language string, matched []navigationDeclaration) *RelatedPoint {
	if !externalNavigationEligible(language, call.importPath) {
		return nil
	}
	symbol := call.resolvedName
	if symbol == "" {
		symbol = call.name
	}
	name := call.display
	if name == "" {
		name = symbol
	}
	consumerPackage := ""
	if len(matched) > 0 {
		consumerPackage = matched[0].packageID
	}
	reference := newExternalNavigationReference(language, call.importPath, symbol, consumerPackage, call.receiverType, "call", call.line)
	return &RelatedPoint{Name: name, Path: "dependency:" + call.importPath, Kind: "external", Direction: "callee", Confidence: "dependency-unresolved", CallLine: call.line, External: reference}
}

func externalNavigationEligible(language, importPath string) bool {
	if importPath == "" {
		return false
	}
	switch language {
	case "go":
		root, _, _ := strings.Cut(importPath, "/")
		return strings.Contains(root, ".")
	case "javascript", "typescript", "tsx":
		return !strings.HasPrefix(importPath, ".") && !strings.HasPrefix(importPath, "/") && !strings.HasPrefix(importPath, "node:")
	case "rust":
		root, _, _ := strings.Cut(importPath, "::")
		switch root {
		case "crate", "self", "super", "std", "core", "alloc":
			return false
		default:
			return root != ""
		}
	case "java", "kotlin":
		return true
	default:
		return false
	}
}

func newExternalNavigationReference(language, importPath, symbol, consumerPackage, receiverType, kind string, line int) *navigation.ExternalReference {
	digest := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%d", language, importPath, symbol, consumerPackage, receiverType, kind, line)))
	return &navigation.ExternalReference{ID: hex.EncodeToString(digest[:]), Language: language, ImportPath: importPath, Symbol: symbol, ConsumerPackage: consumerPackage, ReceiverType: receiverType, Kind: kind}
}

func navigationCallers(targets []navigationDeclaration, navigation *navigationIndex) ([]RelatedPoint, int) {
	var related []RelatedPoint
	for _, target := range targets {
		for _, caller := range navigation.callersByTargetID[target.id] {
			if !sameNavigationLanguage(caller.declaration.language, target.language) || relatedLocationKey(caller.declaration.point) == relatedLocationKey(target.point) {
				continue
			}
			point := caller.declaration.point
			point.Direction = "caller"
			point.CallLine = caller.call.line
			point.Confidence = caller.call.confidence
			if !resolvedNavigationConfidence(point.Confidence) {
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
	followed := map[string]int{"caller": 0, "callee": 0}
	for index := range points {
		point := &points[index]
		if (point.Direction != "caller" && point.Direction != "callee") || followed[point.Direction] >= maxFollowedPerLevel || !resolvedNavigationConfidence(point.Confidence) {
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
		nested, omittedCallers, omittedCallees, omittedTypes := relatedPoints(match, navigation)
		nested = expandRelated(nested, navigation, depth-1, copyLocations(seen), lineBudget)
		point.Preview = &RelatedPreview{
			Content: content, Start: declaration.matchStart, End: declaration.point.End, Related: nested,
			OmittedCallers: omittedCallers, OmittedCallees: omittedCallees, OmittedTypes: omittedTypes,
		}
		followed[point.Direction]++
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
		if point.Direction == "type" {
			key += "\x00" + point.Role
		}
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
