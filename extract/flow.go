package extract

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// FlowNode is a Mermaid node mapped to a code symbol or concept.
type FlowNode struct {
	ID, Symbol, Language, Package, Module string
	Line                                  int
	Concept                               bool
	symbolLine, languageLine              int
	packageLine, moduleLine, conceptLine  int
}

// FlowEdge is a directed Mermaid flowchart edge.
type FlowEdge struct {
	Source, Target, Operator, Label string
	Line                            int
}

// Flowchart is the parsed flow schema.
type Flowchart struct {
	Nodes map[string]*FlowNode
	Order []string
	Edges []FlowEdge
}

var flowHeaderRE = regexp.MustCompile(`^flowchart\s+(TB|TD|BT|RL|LR)$`)
var directionRE = regexp.MustCompile(`^direction\s+(TB|TD|BT|RL|LR)$`)
var edgeRE = regexp.MustCompile(`^([A-Za-z_][\w-]*)\s*(-->|==>|-\.->)\s*(?:\|([^|]*)\|\s*)?([A-Za-z_][\w-]*)$`)
var nodeRE = regexp.MustCompile(`^([A-Za-z_][\w-]*)\s*(?:\[.*\]|\{.*\}|\(.*\))$`)
var symbolRE = regexp.MustCompile(`^%%\s*(?:grepple|pi):symbol\s+([A-Za-z_][\w-]*)\s+(\S+)\s*$`)
var packageRE = regexp.MustCompile(`^%%\s*(?:grepple|pi):package\s+([A-Za-z_][\w-]*)\s+(\S+)\s*$`)
var languageRE = regexp.MustCompile(`^%%\s*(?:grepple|pi):language\s+([A-Za-z_][\w-]*)\s+([A-Za-z_][\w-]*)\s*$`)
var moduleRE = regexp.MustCompile(`^%%\s*(?:grepple|pi):module\s+([A-Za-z_][\w-]*)\s+(\S+)\s*$`)
var conceptRE = regexp.MustCompile(`^%%\s*(?:grepple|pi):concept\s+([A-Za-z_][\w-]*)\s*$`)

type flowParser struct {
	flowchart *Flowchart
	sawHeader bool
}

func newFlowParser() *flowParser {
	return &flowParser{flowchart: &Flowchart{Nodes: map[string]*FlowNode{}}}
}

func (parser *flowParser) node(identifier string, line int) *FlowNode {
	node := parser.flowchart.Nodes[identifier]
	if node == nil {
		node = &FlowNode{ID: identifier, Symbol: identifier, Line: line}
		parser.flowchart.Nodes[identifier] = node
		parser.flowchart.Order = append(parser.flowchart.Order, identifier)
	}
	return node
}

func (parser *flowParser) parseMetadata(value string, line int) error {
	if generatedMetadataRE.MatchString(value) || truncatedMetadataRE.MatchString(value) {
		return nil
	}
	if !parser.sawHeader && recognizedFlowMetadata(value) {
		return nil
	}
	if match := symbolRE.FindStringSubmatch(value); match != nil {
		return parser.applySymbol(match[1], match[2], line)
	}
	if match := packageRE.FindStringSubmatch(value); match != nil {
		return parser.applyPackage(match[1], match[2], line)
	}
	if match := moduleRE.FindStringSubmatch(value); match != nil {
		return parser.applyModule(match[1], match[2], line)
	}
	if match := languageRE.FindStringSubmatch(value); match != nil {
		return parser.applyLanguage(match[1], match[2], line)
	}
	if match := conceptRE.FindStringSubmatch(value); match != nil {
		return parser.applyConcept(match[1], line)
	}
	return validateNamespacedFlowMetadata(value, line)
}

func recognizedFlowMetadata(value string) bool {
	patterns := []*regexp.Regexp{symbolRE, packageRE, moduleRE, languageRE, conceptRE}
	for _, pattern := range patterns {
		if pattern.MatchString(value) {
			return true
		}
	}
	return false
}

func validateNamespacedFlowMetadata(value string, line int) error {
	if !metadataNamespaceRE.MatchString(value) {
		return nil
	}
	remainder := metadataNamespaceRE.ReplaceAllString(value, "")
	fields := strings.Fields(remainder)
	if len(fields) == 0 {
		return fmt.Errorf("Line %d: malformed metadata directive; expected a directive name after the namespace", line)
	}
	directive := fields[0]
	expected := map[string]string{
		"symbol":    "%% grepple:symbol <target> <symbol>",
		"package":   "%% grepple:package <target> <exact-go-import-path>",
		"module":    "%% grepple:module <target> <module-path>",
		"language":  "%% grepple:language <target> <language>",
		"concept":   "%% grepple:concept <target>",
		"generated": "%% grepple:generated entry <symbol> depth <depth> max-nodes <limit>",
		"truncated": "%% grepple:truncated max-nodes <limit>",
	}
	if syntax, known := expected[directive]; known {
		return fmt.Errorf("Line %d: malformed %s directive; expected '%s'", line, directive, syntax)
	}
	return fmt.Errorf("Line %d: unknown metadata directive '%s'", line, directive)
}

func (parser *flowParser) applySymbol(identifier, symbol string, line int) error {
	node := parser.flowchart.Nodes[identifier]
	if node == nil {
		return fmt.Errorf("Line %d: flow metadata target '%s' is not declared", line, identifier)
	}
	if node.symbolLine != 0 {
		return fmt.Errorf("Line %d: duplicate symbol directive for flow node '%s'", line, identifier)
	}
	node.Symbol, node.symbolLine = symbol, line
	return nil
}

func (parser *flowParser) applyPackage(identifier, packageName string, line int) error {
	node := parser.flowchart.Nodes[identifier]
	if node == nil {
		return fmt.Errorf("Line %d: flow metadata target '%s' is not declared", line, identifier)
	}
	if node.packageLine != 0 {
		return fmt.Errorf("Line %d: duplicate package directive for flow node '%s'", line, identifier)
	}
	if node.Module != "" {
		return fmt.Errorf("Line %d: flow node '%s' cannot have both package and module metadata", line, identifier)
	}
	if node.languageLine != 0 && node.Language != "go" {
		return fmt.Errorf("Line %d: package metadata conflicts with language metadata on flow node '%s'", line, identifier)
	}
	node.Package, node.packageLine = packageName, line
	node.Language = "go"
	return nil
}

func (parser *flowParser) applyModule(identifier, module string, line int) error {
	node := parser.flowchart.Nodes[identifier]
	if node == nil {
		return fmt.Errorf("Line %d: flow metadata target '%s' is not declared", line, identifier)
	}
	if node.moduleLine != 0 {
		return fmt.Errorf("Line %d: duplicate module directive for flow node '%s'", line, identifier)
	}
	if node.Package != "" {
		return fmt.Errorf("Line %d: flow node '%s' cannot have both package and module metadata", line, identifier)
	}
	if node.languageLine != 0 && node.Language != "typescript" {
		return fmt.Errorf("Line %d: module metadata conflicts with language metadata on flow node '%s'", line, identifier)
	}
	node.Module, node.moduleLine, node.Language = module, line, "typescript"
	return nil
}

func (parser *flowParser) applyLanguage(identifier, language string, line int) error {
	if !isSupportedLanguage(language) {
		return fmt.Errorf("Line %d: malformed language directive: unsupported language %q", line, language)
	}
	node := parser.flowchart.Nodes[identifier]
	if node == nil {
		return fmt.Errorf("Line %d: flow metadata target '%s' is not declared", line, identifier)
	}
	if node.languageLine != 0 {
		return fmt.Errorf("Line %d: duplicate language directive for flow node '%s'", line, identifier)
	}
	if node.Package != "" && language != "go" || node.Module != "" && language != "typescript" {
		return fmt.Errorf("Line %d: language metadata conflicts with scope metadata on flow node '%s'", line, identifier)
	}
	node.Language, node.languageLine = language, line
	return nil
}

func (parser *flowParser) applyConcept(identifier string, line int) error {
	node := parser.flowchart.Nodes[identifier]
	if node == nil {
		return fmt.Errorf("Line %d: flow metadata target '%s' is not declared", line, identifier)
	}
	if node.conceptLine != 0 {
		return fmt.Errorf("Line %d: duplicate concept directive for flow node '%s'", line, identifier)
	}
	node.Concept, node.conceptLine = true, line
	return nil
}

func (parser *flowParser) parseContent(value string, line int) error {
	if directionRE.MatchString(value) {
		return nil
	}
	if match := edgeRE.FindStringSubmatch(value); match != nil {
		parser.node(match[1], line)
		parser.node(match[4], line)
		parser.flowchart.Edges = append(parser.flowchart.Edges, FlowEdge{match[1], match[4], match[2], match[3], line})
		return nil
	}
	if match := nodeRE.FindStringSubmatch(value); match != nil {
		parser.node(match[1], line)
		return nil
	}
	return fmt.Errorf("Line %d: unsupported flowchart syntax: %s", line, value)
}

func (parser *flowParser) parseLine(value string, line int) error {
	if value == "" {
		return nil
	}
	if strings.HasPrefix(value, "%%") {
		return parser.parseMetadata(value, line)
	}
	if !parser.sawHeader {
		if !flowHeaderRE.MatchString(value) {
			return fmt.Errorf("Line %d: expected a Mermaid flowchart direction header", line)
		}
		parser.sawHeader = true
		return nil
	}
	return parser.parseContent(value, line)
}

// ParseFlowchart parses the supported Mermaid flowchart schema.
func ParseFlowchart(source string) (*Flowchart, error) {
	parser := newFlowParser()
	for index, raw := range strings.Split(strings.ReplaceAll(source, "\r\n", "\n"), "\n") {
		if err := parser.parseLine(strings.TrimSpace(raw), index+1); err != nil {
			return nil, err
		}
	}
	if !parser.sawHeader {
		return nil, fmt.Errorf("Flowchart is empty")
	}
	return parser.flowchart, nil
}

func checkFlowNodes(flowchart *Flowchart, analysis *Analysis) []Diagnostic {
	var diagnostics []Diagnostic
	for _, identifier := range flowchart.Order {
		node := flowchart.Nodes[identifier]
		if node.Concept {
			continue
		}
		if flowSymbolAmbiguous(node, analysis) {
			diagnostics = append(diagnostics, Diagnostic{Line: node.Line, Message: ambiguityMessage("symbol", node.Symbol)})
		} else if flowSymbol(node, analysis) == nil {
			diagnostics = append(diagnostics, Diagnostic{Line: node.Line, Message: fmt.Sprintf("Missing code symbol '%s' for flow node '%s'.", node.Symbol, node.ID)})
		}
	}
	return diagnostics
}

func flowSymbol(node *FlowNode, analysis *Analysis) *Symbol {
	if node.Language == "go" || node.Package != "" {
		return uniqueGoFlowSymbol(node, analysis)
	}
	if node.Language == "typescript" {
		return uniqueTypeScriptFlowSymbol(node, analysis)
	}
	if node.Language != "" {
		return analysis.SymbolVariants[node.Language+":"+node.Symbol]
	}
	if flowSymbolAmbiguous(node, analysis) {
		return nil
	}
	return analysis.Symbols[node.Symbol]
}

func uniqueGoFlowSymbol(node *FlowNode, analysis *Analysis) *Symbol {
	var result *Symbol
	for _, symbol := range analysis.GoSymbolIndex {
		if symbol.Name != node.Symbol || !goScopeMatches(node.Package, symbol.Package, symbol.PackageID, analysis) {
			continue
		}
		if result != nil {
			return nil
		}
		result = symbol
	}
	return result
}

func uniqueTypeScriptFlowSymbol(node *FlowNode, analysis *Analysis) *Symbol {
	if node.Module != "" {
		moduleID := resolveTypeScriptScope(node.Module, analysis)
		if moduleID == "" {
			return nil
		}
		return analysis.TSSymbolIndex[moduleID+":"+node.Symbol]
	}
	var result *Symbol
	for _, symbol := range analysis.TSSymbolIndex {
		if symbol.Name != node.Symbol {
			continue
		}
		if result != nil {
			return nil
		}
		result = symbol
	}
	return result
}

func flowSymbolAmbiguous(node *FlowNode, analysis *Analysis) bool {
	if node.Module != "" && typeScriptScopeAmbiguous(node.Module, analysis) {
		return true
	}
	tsCount := typeScriptFlowSymbolCount(node, analysis)
	if node.Language == "typescript" {
		return tsCount > 1
	}
	if node.Language != "" && node.Language != "go" {
		return false
	}
	goCount := goFlowSymbolCount(node, analysis)
	if node.Language == "go" || node.Package != "" {
		return goCount > 1
	}
	return goCount > 1 || tsCount > 1 || goCount == 1 && tsCount == 1
}

func typeScriptFlowSymbolCount(node *FlowNode, analysis *Analysis) int {
	count, moduleID := 0, resolveTypeScriptScope(node.Module, analysis)
	for _, symbol := range analysis.TSSymbolIndex {
		if symbol.Name == node.Symbol && (node.Module == "" || symbol.ModuleID == moduleID) {
			count++
		}
	}
	return count
}

func goFlowSymbolCount(node *FlowNode, analysis *Analysis) int {
	count := 0
	for _, symbol := range analysis.GoSymbolIndex {
		if symbol.Name == node.Symbol && goScopeMatches(node.Package, symbol.Package, symbol.PackageID, analysis) {
			count++
		}
	}
	return count
}

func symbolsForLanguage(language string, analysis *Analysis) map[string]*Symbol {
	if language == "" {
		return analysis.Symbols
	}
	result := map[string]*Symbol{}
	for _, symbol := range analysis.SymbolVariants {
		if symbol.Language == language {
			result[symbol.Name] = symbol
		}
	}
	return result
}

func checkFlowEdge(edge FlowEdge, nodes map[string]*FlowNode, analysis *Analysis) *Diagnostic {
	source, target := nodes[edge.Source], nodes[edge.Target]
	if source.Concept || target.Concept {
		return nil
	}
	sourceSymbol, targetSymbol := flowSymbol(source, analysis), flowSymbol(target, analysis)
	if sourceSymbol == nil || targetSymbol == nil {
		return nil
	}
	if sourceSymbol.Language != targetSymbol.Language {
		return unsupportedFlowEdge(edge, source, target)
	}
	definition, ok := languageDefinitionForID(sourceSymbol.Language)
	if ok && definition.validFlowEdge(analysis, sourceSymbol, targetSymbol) {
		return nil
	}
	return unsupportedFlowEdge(edge, source, target)
}

func unsupportedFlowEdge(edge FlowEdge, source, target *FlowNode) *Diagnostic {
	message := fmt.Sprintf("No static call or ordered phase path from '%s' to '%s' for flow edge '%s' -> '%s'.", source.Symbol, target.Symbol, edge.Source, edge.Target)
	return &Diagnostic{Line: edge.Line, Message: message}
}

// CheckFlowchart validates a Mermaid flowchart against supported sources.
func CheckFlowchart(flow string, sources []Source) ([]Diagnostic, error) {
	analysis, err := Analyze(sources)
	if err != nil {
		return nil, err
	}
	return CheckFlowchartWithAnalysis(flow, analysis)
}

// CheckFlowchartWithAnalysis validates a flowchart using a shared source analysis.
func CheckFlowchartWithAnalysis(flow string, analysis *Analysis) ([]Diagnostic, error) {
	flowchart, err := ParseFlowchart(flow)
	if err != nil {
		return nil, err
	}
	diagnostics := checkFlowNodes(flowchart, analysis)
	for _, edge := range flowchart.Edges {
		if diagnostic := checkFlowEdge(edge, flowchart.Nodes, analysis); diagnostic != nil {
			diagnostics = append(diagnostics, *diagnostic)
		}
	}
	sortDiagnostics(diagnostics)
	return diagnostics, nil
}

func sortDiagnostics(diagnostics []Diagnostic) {
	sort.SliceStable(diagnostics, func(left, right int) bool {
		if diagnostics[left].Line != diagnostics[right].Line {
			return diagnostics[left].Line < diagnostics[right].Line
		}
		leftCompleteness, rightCompleteness := diagnostics[left].sortPath != "", diagnostics[right].sortPath != ""
		if leftCompleteness != rightCompleteness {
			return !leftCompleteness
		}
		if leftCompleteness {
			leftDiagnostic, rightDiagnostic := diagnostics[left], diagnostics[right]
			return leftDiagnostic.sortPath < rightDiagnostic.sortPath || leftDiagnostic.sortPath == rightDiagnostic.sortPath && (leftDiagnostic.sortLine < rightDiagnostic.sortLine || leftDiagnostic.sortLine == rightDiagnostic.sortLine && leftDiagnostic.sortName < rightDiagnostic.sortName)
		}
		return diagnostics[left].Message < diagnostics[right].Message
	})
}
