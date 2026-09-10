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
	maxRelatedPoints            = 5
	maxRelatedCandidatesPerCall = 3
	maxFollowedPerLevel         = 2
	maxFollowedTotalLines       = 400
)

type navigationDeclaration struct {
	id         string
	terminal   string
	language   string
	file       string
	matchStart int
	point      RelatedPoint
}

type navigationCaller struct {
	declaration navigationDeclaration
	display     string
	callLine    int
}

type navigationCall struct {
	name, display string
	line          int
}

type navigationIndex struct {
	declarations map[string][]navigationDeclaration
	callers      map[string][]navigationCaller
	calls        map[string][]navigationCall
	byFile       map[string][]navigationDeclaration
	byLocation   map[string]navigationDeclaration
	contents     map[string]string
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
		related := relatedPoints(matches[index], navigation)
		if followDepth > 0 {
			seen := matchedLocations(matches[index], navigation)
			lineBudget := maxFollowedTotalLines
			related = expandRelated(related, navigation, followDepth, seen, &lineBudget)
		}
		matches[index].Related = related
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
	return index
}
func (index *navigationIndex) addFile(path, cwd string) {
	language := parser.LanguageFor(path)
	if !supportsNavigation(language) {
		return
	}
	contentBytes, err := os.ReadFile(path)
	if err != nil || bytes.IndexByte(contentBytes, 0) >= 0 {
		return
	}
	content := strings.ToValidUTF8(string(contentBytes), "\uFFFD")
	cleanPath := filepath.Clean(path)
	graph := parser.BuildNavigationGraph(content, language, cleanPath)
	if len(graph.Declarations) == 0 {
		return
	}
	displayPath := displayPathFrom(path, cwd)
	index.contents[cleanPath] = content
	indexed := index.addDeclarations(graph.Declarations, language, cleanPath, displayPath)
	index.addCalls(graph.Calls, indexed, language)
}

func (index *navigationIndex) addDeclarations(declarations []parser.NavigationDeclaration, language, path, displayPath string) []navigationDeclaration {
	indexed := make([]navigationDeclaration, 0, len(declarations))
	for _, declaration := range declarations {
		terminal := terminalSymbolName(declaration.Name)
		if terminal == "" {
			continue
		}
		item := navigationDeclaration{
			id: declaration.ID, terminal: terminal, language: language, file: path, matchStart: declaration.Start,
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

func (index *navigationIndex) addCalls(calls []parser.NavigationCall, declarations []navigationDeclaration, language string) {
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
		index.calls[location] = append(index.calls[location], navigationCall{name: call.Name, display: call.Display, line: call.Line})
		key := navigationSymbolKey(language, call.Name)
		index.callers[key] = append(index.callers[key], navigationCaller{declaration: caller, display: call.Display, callLine: call.Line})
	}
}

func relatedPoints(match FileMatch, navigation *navigationIndex) []RelatedPoint {
	declarations := matchedDeclarations(match, navigation)
	callees := relatedCallees(match, declarations, navigation)
	callers := navigationCallers(declarations, navigation)
	return append(callees, callers...)
}

func relatedCallees(match FileMatch, declarations []navigationDeclaration, navigation *navigationIndex) []RelatedPoint {
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
	resolution := resolveNavigationCandidates(call, navigation.declarations[navigationSymbolKey(language, call.name)], matched)
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
		if resolution.confidence == "candidate" && len(resolved) >= maxRelatedCandidatesPerCall {
			break
		}
	}
	return resolved
}

type navigationCandidateResolution struct {
	candidates []navigationDeclaration
	confidence string
}

func resolveNavigationCandidates(call navigationCall, candidates, matched []navigationDeclaration) navigationCandidateResolution {
	if exact := exactNavigationCandidates(call, candidates); len(exact) == 1 {
		return navigationCandidateResolution{exact, "exact"}
	} else if len(exact) > 1 {
		candidates = exact
	}
	originalCount := len(candidates)
	if functions := unqualifiedFunctionCandidates(call, candidates); len(functions) > 0 {
		candidates = functions
		if len(candidates) == 1 && originalCount > 1 {
			return navigationCandidateResolution{candidates, "context-resolved"}
		}
	}
	beforeLocal := len(candidates)
	if local := candidatesInMatchedFiles(candidates, matched); len(local) > 0 {
		candidates = local
		if len(candidates) == 1 && beforeLocal > 1 {
			return navigationCandidateResolution{candidates, "context-resolved"}
		}
	}
	if len(candidates) == 1 {
		return navigationCandidateResolution{candidates, "unique-terminal"}
	}
	return navigationCandidateResolution{candidates, "candidate"}
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

func candidatesInMatchedFiles(candidates, matched []navigationDeclaration) []navigationDeclaration {
	files := map[string]bool{}
	for _, declaration := range matched {
		files[declaration.file] = true
	}
	return filterNavigationCandidates(candidates, func(candidate navigationDeclaration) bool {
		return files[candidate.file]
	})
}

func navigationCallers(targets []navigationDeclaration, navigation *navigationIndex) []RelatedPoint {
	var related []RelatedPoint
	for _, target := range targets {
		terminalCandidates := navigation.declarations[navigationSymbolKey(target.language, target.terminal)]
		for _, caller := range navigation.callers[navigationSymbolKey(target.language, target.terminal)] {
			if !sameNavigationLanguage(caller.declaration.language, target.language) || relatedLocationKey(caller.declaration.point) == relatedLocationKey(target.point) {
				continue
			}
			point := caller.declaration.point
			point.Direction = "caller"
			point.CallLine = caller.callLine
			point.Confidence = navigationCallerConfidence(target, caller, len(terminalCandidates))
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

func navigationCallerConfidence(target navigationDeclaration, caller navigationCaller, terminalCandidates int) string {
	if strings.ContainsAny(caller.display, ".:#") && caller.display == target.point.Name {
		return "exact"
	}
	if terminalCandidates == 1 {
		return "unique-terminal"
	}
	return "candidate"
}

func resolvedNavigationConfidence(confidence string) bool {
	return confidence == "exact" || confidence == "context-resolved" || confidence == "unique-terminal" || confidence == "unique"
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
		nested := relatedPoints(match, navigation)
		nested = expandRelated(nested, navigation, depth-1, copyLocations(seen), lineBudget)
		point.Preview = &RelatedPreview{Content: content, Start: declaration.matchStart, End: declaration.point.End, Related: nested}
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

func limitRelatedPoints(points []RelatedPoint) []RelatedPoint {
	if len(points) > maxRelatedPoints {
		return points[:maxRelatedPoints]
	}
	return points
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
