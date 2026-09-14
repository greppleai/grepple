package search

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/greppleai/grepple/parser"
)

// ResponsibilityBreadth summarizes the distinct consumers of a type's methods.
type ResponsibilityBreadth struct {
	Functions int `json:"functions"`
	Files     int `json:"files"`
	Packages  int `json:"packages"`
}

// ResponsibilitySurface summarizes externally used methods on a type.
type ResponsibilitySurface struct {
	External int `json:"external"`
	Declared int `json:"declared"`
}

// ResponsibilityConsumer identifies one callable containing a type interaction.
type ResponsibilityConsumer struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Path    string `json:"path"`
	Package string `json:"package,omitempty"`
	Line    int    `json:"line"`
}

// ResponsibilityPattern is a repeated method set or ordered method sequence.
type ResponsibilityPattern struct {
	Methods     []string                 `json:"methods"`
	Occurrences int                      `json:"occurrences"`
	Files       int                      `json:"files"`
	Packages    int                      `json:"packages"`
	Consumers   []ResponsibilityConsumer `json:"consumers"`
}

// ResponsibilityReport describes interaction topology around one type.
type ResponsibilityReport struct {
	Schema                   string                   `json:"schema"`
	Type                     string                   `json:"type"`
	Languages                []string                 `json:"languages"`
	Consumers                ResponsibilityBreadth    `json:"consumers"`
	ConsumerDetails          []ResponsibilityConsumer `json:"consumerDetails"`
	ExternalMethodSurface    ResponsibilitySurface    `json:"externalMethodSurface"`
	Methods                  []string                 `json:"methods"`
	MethodCoUsage            []ResponsibilityPattern  `json:"methodCoUsage"`
	OrderedSequences         []ResponsibilityPattern  `json:"orderedSequences"`
	MemberMethodCombinations []ResponsibilityPattern  `json:"memberMethodCombinations"`
	UnresolvedCalls          int                      `json:"unresolvedCalls"`
}

// AnalyzeResponsibilities finds repeated cross-callable interaction topology for
// a type. It consumes only normalized navigation facts, so the algorithm is the
// same for every parser-backed language.
func AnalyzeResponsibilities(graph parser.NavigationGraph, typeName string, minOccurrences int) (ResponsibilityReport, error) {
	typeName = strings.TrimSpace(typeName)
	if typeName == "" {
		return ResponsibilityReport{}, fmt.Errorf("responsibility type must not be empty")
	}
	if minOccurrences < 1 {
		return ResponsibilityReport{}, fmt.Errorf("minimum occurrences must be positive")
	}
	analyzer := newResponsibilityAnalyzer(graph, typeName)
	if len(analyzer.methodIDs) == 0 && len(analyzer.usageCallers) == 0 {
		return ResponsibilityReport{}, fmt.Errorf("no type interactions found for %q", typeName)
	}
	return analyzer.report(minOccurrences), nil
}

type responsibilityInteraction struct {
	label string
}

type responsibilityAnalyzer struct {
	typeName             string
	declarations         map[string]parser.NavigationDeclaration
	methodIDs            map[string]bool
	methods              map[string]bool
	languages            map[string]bool
	callsByCaller        map[string][]string
	interactionsByCaller map[string][]responsibilityInteraction
	usageCallers         map[string]bool
	unresolvedCall       int
}

func newResponsibilityAnalyzer(graph parser.NavigationGraph, typeName string) *responsibilityAnalyzer {
	analyzer := &responsibilityAnalyzer{
		typeName: typeName, declarations: make(map[string]parser.NavigationDeclaration, len(graph.Declarations)),
		methodIDs: make(map[string]bool), methods: make(map[string]bool), languages: make(map[string]bool), callsByCaller: make(map[string][]string), interactionsByCaller: make(map[string][]responsibilityInteraction), usageCallers: make(map[string]bool),
	}
	analyzer.indexResponsibilityDeclarations(graph.Declarations)
	analyzer.indexResponsibilityCalls(graph.Calls)
	analyzer.indexResponsibilityMemberAccesses(graph.MemberAccesses)
	analyzer.indexResponsibilityTypeUsages(graph.TypeUsages)
	return analyzer
}

func (analyzer *responsibilityAnalyzer) indexResponsibilityDeclarations(declarations []parser.NavigationDeclaration) {
	for _, declaration := range declarations {
		analyzer.declarations[declaration.ID] = declaration
		if responsibilityTypeMatches(declaration, analyzer.typeName) {
			analyzer.methodIDs[declaration.ID] = true
			analyzer.methods[responsibilityMemberName(declaration.Name)] = true
			analyzer.languages[declaration.Language] = true
		}
	}
}

func (analyzer *responsibilityAnalyzer) indexResponsibilityCalls(calls []parser.NavigationCall) {
	for _, call := range calls {
		if analyzer.methodIDs[call.CallerID] {
			continue
		}
		member, matched, unresolved := analyzer.matchCall(call)
		if unresolved {
			analyzer.unresolvedCall++
		}
		if matched {
			analyzer.usageCallers[call.CallerID] = true
			analyzer.callsByCaller[call.CallerID] = append(analyzer.callsByCaller[call.CallerID], member)
			analyzer.interactionsByCaller[call.CallerID] = append(analyzer.interactionsByCaller[call.CallerID], responsibilityInteraction{label: member + "()"})
		}
	}
}

func (analyzer *responsibilityAnalyzer) indexResponsibilityMemberAccesses(accesses []parser.NavigationMemberAccess) {
	for _, access := range accesses {
		if analyzer.methodIDs[access.CallerID] || terminalTypeName(access.ReceiverType) != analyzer.typeName {
			continue
		}
		analyzer.usageCallers[access.CallerID] = true
		analyzer.languages[access.Language] = true
		label := access.Member + "(" + access.Operation + ")"
		analyzer.interactionsByCaller[access.CallerID] = append(analyzer.interactionsByCaller[access.CallerID], responsibilityInteraction{label: label})
	}
}

func (analyzer *responsibilityAnalyzer) indexResponsibilityTypeUsages(usages []parser.NavigationTypeUsage) {
	for _, usage := range usages {
		if !analyzer.methodIDs[usage.CallerID] && terminalTypeName(usage.Type) == analyzer.typeName {
			analyzer.usageCallers[usage.CallerID] = true
			analyzer.languages[usage.Language] = true
		}
	}
}

func (analyzer *responsibilityAnalyzer) matchCall(call parser.NavigationCall) (string, bool, bool) {
	if analyzer.methodIDs[call.TargetID] {
		return responsibilityMemberName(analyzer.declarations[call.TargetID].Name), true, false
	}
	matchedMembers := make(map[string]bool)
	for _, id := range call.CandidateTargetIDs {
		if analyzer.methodIDs[id] {
			matchedMembers[responsibilityMemberName(analyzer.declarations[id].Name)] = true
		}
	}
	if len(matchedMembers) == 1 {
		for member := range matchedMembers {
			return member, true, true
		}
	}
	member := responsibilityMemberName(call.Name)
	if analyzer.methods[member] && (terminalTypeName(call.ReceiverType) == analyzer.typeName || terminalTypeName(call.Qualifier) == analyzer.typeName) {
		return member, true, call.TargetID == ""
	}
	return "", false, false
}

func (analyzer *responsibilityAnalyzer) report(minOccurrences int) ResponsibilityReport {
	methodNames := sortedResponsibilityKeys(analyzer.methods)
	languages := sortedResponsibilityKeys(analyzer.languages)
	consumers := make([]ResponsibilityConsumer, 0, len(analyzer.usageCallers))
	groups := responsibilityPatternGroups{externalMethods: make(map[string]bool), methodSets: make(map[string]*responsibilityPatternGroup), sequences: make(map[string]*responsibilityPatternGroup), combinations: make(map[string]*responsibilityPatternGroup)}
	for callerID := range analyzer.usageCallers {
		interactions := analyzer.interactionsByCaller[callerID]
		if consumer, ok := analyzer.addResponsibilityCaller(callerID, interactions, &groups); ok {
			consumers = append(consumers, consumer)
		}
	}
	sortResponsibilityConsumers(consumers)
	return ResponsibilityReport{
		Schema: "grepple-responsibilities-v1", Type: analyzer.typeName, Languages: languages,
		Consumers: responsibilityBreadth(consumers), ConsumerDetails: consumers, ExternalMethodSurface: ResponsibilitySurface{External: len(groups.externalMethods), Declared: len(methodNames)},
		Methods: methodNames, MethodCoUsage: responsibilityPatterns(groups.methodSets, minOccurrences), OrderedSequences: responsibilityPatterns(groups.sequences, minOccurrences),
		MemberMethodCombinations: responsibilityPatterns(groups.combinations, minOccurrences),
		UnresolvedCalls:          analyzer.unresolvedCall,
	}
}

type responsibilityPatternGroups struct {
	externalMethods map[string]bool
	methodSets      map[string]*responsibilityPatternGroup
	sequences       map[string]*responsibilityPatternGroup
	combinations    map[string]*responsibilityPatternGroup
}

func (analyzer *responsibilityAnalyzer) addResponsibilityCaller(callerID string, interactions []responsibilityInteraction, groups *responsibilityPatternGroups) (ResponsibilityConsumer, bool) {
	caller, ok := analyzer.declarations[callerID]
	if !ok {
		return ResponsibilityConsumer{}, false
	}
	consumer := responsibilityConsumer(caller)
	sequence := analyzer.callsByCaller[callerID]
	for _, method := range sequence {
		groups.externalMethods[method] = true
	}
	if set := uniqueSortedResponsibilityMethods(sequence); len(set) > 1 {
		addResponsibilityPattern(groups.methodSets, set, consumer)
	}
	if len(sequence) > 1 {
		addResponsibilityPattern(groups.sequences, sequence, consumer)
	}
	combination, hasMember, hasMethod := responsibilityCombination(interactions)
	if hasMember && hasMethod {
		addResponsibilityPattern(groups.combinations, combination, consumer)
	}
	return consumer, true
}

func responsibilityCombination(interactions []responsibilityInteraction) ([]string, bool, bool) {
	combination := make([]string, 0, len(interactions))
	hasMember, hasMethod := false, false
	for _, interaction := range interactions {
		combination = append(combination, interaction.label)
		if strings.HasSuffix(interaction.label, "()") {
			hasMethod = true
		} else {
			hasMember = true
		}
	}
	return uniqueSortedResponsibilityMethods(combination), hasMember, hasMethod
}

func responsibilityTypeMatches(declaration parser.NavigationDeclaration, typeName string) bool {
	return responsibilityDeclarationType(declaration) == typeName
}

func responsibilityDeclarationType(declaration parser.NavigationDeclaration) string {
	if receiver := terminalTypeName(declaration.Receiver); receiver != "" {
		return receiver
	}
	if container := terminalTypeName(declaration.Container); container != "" {
		return container
	}
	if declaration.Kind != "method" && declaration.Kind != "constructor" {
		return ""
	}
	separator := strings.LastIndex(declaration.Name, ".")
	if separator <= 0 {
		return ""
	}
	return terminalTypeName(declaration.Name[:separator])
}

func terminalTypeName(value string) string {
	value = strings.TrimSpace(strings.TrimPrefix(value, "*"))
	if index := strings.LastIndexAny(value, ".:/"); index >= 0 {
		value = value[index+1:]
	}
	if index := strings.IndexByte(value, '['); index >= 0 {
		value = value[:index]
	}
	return value
}

func responsibilityMemberName(name string) string {
	if index := strings.LastIndex(name, "."); index >= 0 {
		return name[index+1:]
	}
	return name
}

func responsibilityConsumer(declaration parser.NavigationDeclaration) ResponsibilityConsumer {
	return ResponsibilityConsumer{ID: declaration.ID, Name: declaration.Name, Path: declaration.Path, Package: responsibilityPackage(declaration), Line: declaration.Start}
}

func responsibilityPackage(declaration parser.NavigationDeclaration) string {
	if declaration.Package != "" {
		return declaration.Package
	}
	directory := filepath.ToSlash(filepath.Dir(declaration.Path))
	if directory == "" {
		return "."
	}
	return directory
}

func responsibilityBreadth(consumers []ResponsibilityConsumer) ResponsibilityBreadth {
	files := make(map[string]bool)
	packages := make(map[string]bool)
	for _, consumer := range consumers {
		files[consumer.Path] = true
		packages[consumer.Package] = true
	}
	return ResponsibilityBreadth{Functions: len(consumers), Files: len(files), Packages: len(packages)}
}

type responsibilityPatternGroup struct {
	methods   []string
	consumers []ResponsibilityConsumer
}

func addResponsibilityPattern(groups map[string]*responsibilityPatternGroup, methods []string, consumer ResponsibilityConsumer) {
	key := strings.Join(methods, "\x00")
	group := groups[key]
	if group == nil {
		group = &responsibilityPatternGroup{methods: append([]string(nil), methods...)}
		groups[key] = group
	}
	group.consumers = append(group.consumers, consumer)
}

func responsibilityPatterns(groups map[string]*responsibilityPatternGroup, minimum int) []ResponsibilityPattern {
	patterns := make([]ResponsibilityPattern, 0, len(groups))
	for _, group := range groups {
		if len(group.consumers) < minimum {
			continue
		}
		sortResponsibilityConsumers(group.consumers)
		breadth := responsibilityBreadth(group.consumers)
		patterns = append(patterns, ResponsibilityPattern{Methods: group.methods, Occurrences: len(group.consumers), Files: breadth.Files, Packages: breadth.Packages, Consumers: group.consumers})
	}
	sort.Slice(patterns, func(i, j int) bool {
		if patterns[i].Occurrences != patterns[j].Occurrences {
			return patterns[i].Occurrences > patterns[j].Occurrences
		}
		return strings.Join(patterns[i].Methods, "\x00") < strings.Join(patterns[j].Methods, "\x00")
	})
	return patterns
}

func uniqueSortedResponsibilityMethods(methods []string) []string {
	set := make(map[string]bool, len(methods))
	for _, method := range methods {
		set[method] = true
	}
	return sortedResponsibilityKeys(set)
}

func sortedResponsibilityKeys(values map[string]bool) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func sortResponsibilityConsumers(consumers []ResponsibilityConsumer) {
	sort.Slice(consumers, func(i, j int) bool {
		if consumers[i].Path != consumers[j].Path {
			return consumers[i].Path < consumers[j].Path
		}
		if consumers[i].Line != consumers[j].Line {
			return consumers[i].Line < consumers[j].Line
		}
		return consumers[i].ID < consumers[j].ID
	})
}
