// Package extract validates and generates source-linked Mermaid structure and flow diagrams.
package extract

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	codeparser "github.com/greppleai/grepple/parser"
	sitter "github.com/tree-sitter/go-tree-sitter"
	typescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

// Source is one source file handled by a registered language adapter.
type Source struct{ Path, Text string }

// Member describes a class, interface, or function member signature.
type Member struct {
	Kind, Name, Visibility, Type, Language, Package, PackageID, ModuleID, File string
	Static, Async                                                              bool
	Parameters                                                                 []string
	Location                                                                   Location
}

// FiberRoute describes a route registered through a *fiber.App method parameter.
type FiberRoute struct {
	Method, Path, Handler, PackageID string
	Location                         Location
}

// GoStructTag preserves both a field tag's value and whether a tag was present.
// Presence is separate because an explicit empty tag (`""`) is meaningful.
type GoStructTag struct {
	Value   string
	Present bool
}

// Declaration describes a language-neutral type declaration.
type Declaration struct {
	Name, Kind, Language, Package, PackageID, ModuleID, File string
	Underlying                                               string
	Members                                                  []Member
	Extends, Implements                                      map[string]bool
	StructTags                                               map[string]GoStructTag
	FileLocal                                                bool
	Location                                                 Location
}

// Import describes one local import binding.
type Import struct {
	Source, Imported, Resolved, ModuleID, ImporterModuleID, File string
	Default, TypeOnly                                            bool
	Language, Package, PackageID                                 string
}

// Location identifies a syntax node in a source file.
type Location struct {
	Path               string
	Line, Column       int
	EndLine, EndColumn int
}

func syntaxLocation(path string, node *sitter.Node) Location {
	start, end := node.StartPosition(), node.EndPosition()
	return Location{Path: path, Line: int(start.Row) + 1, Column: int(start.Column) + 1, EndLine: int(end.Row) + 1, EndColumn: int(end.Column) + 1}
}

// Symbol describes a callable code symbol linked to its parser-owned navigation declaration.
type Symbol struct {
	Name, Kind, Language, Package, PackageID, ModuleID, Key string
	Owner, Receiver, NavigationID                           string
	// Calls and CallOrder are compatibility projections derived from Navigation.
	Calls     map[string]bool
	CallOrder []string
	Locations []Location
}

// Analysis contains the structure and call graph extracted from sources.
type Analysis struct {
	Declarations                          map[string]*Declaration
	DeclarationVariants                   map[string]*Declaration
	Functions                             map[string][]Member
	Imports                               map[string][]Import
	Exports, DefaultExports               map[string]bool
	ExportVariants, DefaultExportVariants map[string]bool
	Symbols                               map[string]*Symbol
	SymbolVariants                        map[string]*Symbol
	TSDeclarations                        map[string]*Declaration
	GoDeclarations                        map[string]*Declaration
	GoTypeReferences                      map[string][]Location
	GoFunctions                           map[string][]Member
	TSSymbolIndex                         map[string]*Symbol
	GoSymbolIndex                         map[string]*Symbol
	GoPackageNames                        map[string]string
	GoFiberRoutes                         []FiberRoute
	GoPackageImports                      map[string]map[string]string
	GoImportPathIndex                     map[string]string
	GoPackagePaths                        map[string]string
	GoFallbackScopes                      map[string]string
	TSModulePaths, SourcePaths            map[string]string
	TSModuleIndex                         map[string]string
	TSImportBindings                      map[string]map[string]Import
	TSDefaultExports                      map[string]string
	TSExports                             map[string]map[string]bool
	TSExportNames                         map[string]map[string]string
	Navigation                            codeparser.NavigationGraph
	navigationSymbols                     map[string]*Symbol
	navigationCalls                       map[string][]*Symbol
	duplicateErrors                       []string
}

type sourceAnalyzer struct {
	result   *Analysis
	source   Source
	text     []byte
	moduleID string
}

func (analyzer *sourceAnalyzer) location(node *sitter.Node) Location {
	location := syntaxLocation(analyzer.source.Path, node)
	location.Path = analyzer.result.SourcePaths[absolutePath(analyzer.source.Path)]
	return location
}

func newAnalysis() *Analysis {
	return &Analysis{
		Declarations:          map[string]*Declaration{},
		DeclarationVariants:   map[string]*Declaration{},
		Functions:             map[string][]Member{},
		Imports:               map[string][]Import{},
		Exports:               map[string]bool{},
		DefaultExports:        map[string]bool{},
		ExportVariants:        map[string]bool{},
		DefaultExportVariants: map[string]bool{},
		Symbols:               map[string]*Symbol{},
		SymbolVariants:        map[string]*Symbol{},
		TSDeclarations:        map[string]*Declaration{},
		GoDeclarations:        map[string]*Declaration{},
		GoTypeReferences:      map[string][]Location{},
		GoFunctions:           map[string][]Member{},
		TSSymbolIndex:         map[string]*Symbol{},
		GoSymbolIndex:         map[string]*Symbol{},
		GoPackageNames:        map[string]string{},
		GoPackageImports:      map[string]map[string]string{},
		GoImportPathIndex:     map[string]string{},
		GoPackagePaths:        map[string]string{},
		GoFallbackScopes:      map[string]string{},
		TSModulePaths:         map[string]string{},
		SourcePaths:           map[string]string{},
		TSModuleIndex:         map[string]string{},
		TSImportBindings:      map[string]map[string]Import{},
		TSDefaultExports:      map[string]string{},
		TSExports:             map[string]map[string]bool{},
		TSExportNames:         map[string]map[string]string{},
		navigationSymbols:     map[string]*Symbol{},
		navigationCalls:       map[string][]*Symbol{},
	}
}

func malformedSourceError(path string, root *sitter.Node) error {
	errorNode := firstErrorNode(root)
	if errorNode == nil {
		return fmt.Errorf("parse %s:1:1: malformed syntax", path)
	}
	position := errorNode.StartPosition()
	return fmt.Errorf("parse %s:%d:%d: malformed syntax near %s", path, position.Row+1, position.Column+1, errorNode.Kind())
}

func firstErrorNode(node *sitter.Node) *sitter.Node {
	if node == nil {
		return nil
	}
	if node.IsError() || node.IsMissing() {
		return node
	}
	for index := uint(0); index < node.ChildCount(); index++ {
		if found := firstErrorNode(node.Child(index)); found != nil {
			return found
		}
	}
	return nil
}
func nodeText(node *sitter.Node, source []byte) string {
	if node == nil || node.EndByte() > uint(len(source)) {
		return ""
	}
	return string(source[node.StartByte():node.EndByte()])
}

func namedChildren(node *sitter.Node) []*sitter.Node {
	children := make([]*sitter.Node, 0, node.NamedChildCount())
	for index := uint(0); index < node.NamedChildCount(); index++ {
		children = append(children, node.NamedChild(index))
	}
	return children
}

func hasChildKind(node *sitter.Node, kind string) bool {
	for index := uint(0); index < node.ChildCount(); index++ {
		if node.Child(index).Kind() == kind {
			return true
		}
	}
	return false
}

func cleanType(value string) string {
	value = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(value), ":"))
	return normalizeType(value)
}

func nodeType(node *sitter.Node, source []byte) string {
	typeNode := node.ChildByFieldName("type")
	if typeNode == nil {
		typeNode = node.ChildByFieldName("return_type")
	}
	if typeNode != nil {
		return cleanType(nodeText(typeNode, source))
	}
	return childTypeAnnotation(node, source)
}

func childTypeAnnotation(node *sitter.Node, source []byte) string {
	for _, child := range namedChildren(node) {
		if child.Kind() == "type_annotation" {
			return cleanType(nodeText(child, source))
		}
	}
	return ""
}

func childHasText(node *sitter.Node, source []byte, value string) bool {
	for index := uint(0); index < node.ChildCount(); index++ {
		if nodeText(node.Child(index), source) == value {
			return true
		}
	}
	return false
}

func memberVisibility(node *sitter.Node, source []byte) string {
	for index := uint(0); index < node.ChildCount(); index++ {
		child := node.Child(index)
		if child.Kind() != "accessibility_modifier" {
			continue
		}
		if value := nodeText(child, source); value == "private" || value == "protected" {
			return value
		}
	}
	return "public"
}

func parameterTypes(node *sitter.Node, source []byte) []string {
	parameters := node.ChildByFieldName("parameters")
	if parameters == nil {
		return nil
	}
	var result []string
	for _, parameter := range namedChildren(parameters) {
		if parameter.Kind() == "required_parameter" || parameter.Kind() == "optional_parameter" {
			result = append(result, nodeType(parameter, source))
		}
	}
	return result
}

func memberFromNode(node *sitter.Node, source []byte) (Member, bool) {
	kind, ok := memberKind(node.Kind())
	if !ok {
		return Member{}, false
	}
	nameNode := node.ChildByFieldName("name")
	name := nodeText(nameNode, source)
	if name == "" {
		return Member{}, false
	}
	member := Member{
		Kind:       kind,
		Language:   "typescript",
		Name:       name,
		Visibility: memberVisibility(node, source),
		Type:       nodeType(node, source),
		Static:     childHasText(node, source, "static"),
		Async:      childHasText(node, source, "async"),
	}
	if nameNode.Kind() == "private_property_identifier" {
		member.Visibility = "private"
	}
	if kind == "method" {
		member.Parameters = parameterTypes(node, source)
	}
	return member, true
}

func memberKind(kind string) (string, bool) {
	switch kind {
	case "method_definition", "method_signature", "abstract_method_signature":
		return "method", true
	case "public_field_definition", "property_signature", "abstract_property_signature":
		return "property", true
	default:
		return "", false
	}
}

func walkTree(node *sitter.Node, visit func(*sitter.Node)) {
	visit(node)
	for _, child := range namedChildren(node) {
		walkTree(child, visit)
	}
}

func (analyzer *sourceAnalyzer) addSymbol(name, kind string, node, _ *sitter.Node, owner string) {
	key := analyzer.moduleID + ":" + name
	symbol := analyzer.result.TSSymbolIndex[key]
	if symbol == nil {
		symbol = &Symbol{Name: name, Kind: kind, Language: "typescript", ModuleID: analyzer.moduleID, Key: key, Owner: owner, Calls: map[string]bool{}}
		analyzer.result.TSSymbolIndex[key] = symbol
	}
	symbol.Locations = append(symbol.Locations, syntaxLocation(analyzer.source.Path, node))
}

func (analyzer *sourceAnalyzer) addImport(name, imported, module string, defaultImport, typeOnly bool) {
	if name == "" {
		return
	}
	item := Import{Source: module, Imported: imported, Default: defaultImport, TypeOnly: typeOnly, Language: "typescript", ImporterModuleID: analyzer.moduleID, File: analyzer.result.SourcePaths[absolutePath(analyzer.source.Path)]}
	analyzer.result.Imports[name] = append(analyzer.result.Imports[name], item)
	if analyzer.result.TSImportBindings[analyzer.moduleID] == nil {
		analyzer.result.TSImportBindings[analyzer.moduleID] = map[string]Import{}
	}
	analyzer.result.TSImportBindings[analyzer.moduleID][name] = item
}

func (analyzer *sourceAnalyzer) analyzeImport(statement *sitter.Node) {
	module := strings.Trim(nodeText(statement.ChildByFieldName("source"), analyzer.text), "'\"")
	clause := statement.NamedChild(0)
	if clause == nil || clause.Kind() != "import_clause" {
		return
	}
	typeOnly := hasChildKind(statement, "type") || childHasText(statement, analyzer.text, "type")
	analyzer.addDefaultImport(clause, module, typeOnly)
	walkTree(clause, func(node *sitter.Node) {
		analyzer.addStructuredImport(node, module, typeOnly)
	})
}

func (analyzer *sourceAnalyzer) addDefaultImport(clause *sitter.Node, module string, typeOnly bool) {
	for _, child := range namedChildren(clause) {
		if child.Kind() == "identifier" {
			analyzer.addImport(nodeText(child, analyzer.text), "", module, true, typeOnly)
		}
	}
}

func (analyzer *sourceAnalyzer) addStructuredImport(node *sitter.Node, module string, typeOnly bool) {
	if node.Kind() != "import_specifier" && node.Kind() != "namespace_import" {
		return
	}
	alias := node.ChildByFieldName("alias")
	importedNode := node.ChildByFieldName("name")
	nameNode := alias
	if nameNode == nil {
		nameNode = importedNode
	}
	if nameNode == nil && node.NamedChildCount() > 0 {
		nameNode = node.NamedChild(node.NamedChildCount() - 1)
	}
	elementTypeOnly := hasChildKind(node, "type") || childHasText(node, analyzer.text, "type")
	analyzer.addImport(nodeText(nameNode, analyzer.text), nodeText(importedNode, analyzer.text), module, false, typeOnly || elementTypeOnly)
}

func (analyzer *sourceAnalyzer) analyzeExportSpecifiers(statement *sitter.Node) {
	walkTree(statement, func(node *sitter.Node) {
		if node.Kind() != "export_specifier" {
			return
		}
		local := node.ChildByFieldName("name")
		exported := node.ChildByFieldName("alias")
		if exported == nil {
			exported = local
		}
		localName, exportedName := nodeText(local, analyzer.text), nodeText(exported, analyzer.text)
		if exportedName == "default" {
			analyzer.result.DefaultExports[localName] = true
			analyzer.result.DefaultExportVariants["typescript:"+localName] = true
			analyzer.result.TSDefaultExports[analyzer.moduleID] = localName
			analyzer.recordTypeScriptExport("default", localName)
		} else if localName != "" {
			analyzer.result.Exports[localName] = true
			analyzer.result.ExportVariants["typescript:"+localName] = true
			analyzer.recordTypeScriptExport(exportedName, localName)
		}
	})
}

func declarationKind(kind string) (string, bool) {
	switch kind {
	case "class_declaration", "abstract_class_declaration":
		return "class", true
	case "interface_declaration":
		return "interface", true
	default:
		return "", false
	}
}

func directHeritageName(node *sitter.Node, source []byte) string {
	candidate := node.ChildByFieldName("value")
	if candidate == nil {
		candidate = node.ChildByFieldName("name")
	}
	if candidate == nil && node.NamedChildCount() > 0 {
		candidate = node.NamedChild(0)
	}
	if candidate != nil && candidate.Kind() == "generic_type" {
		candidate = candidate.ChildByFieldName("name")
	}
	return strings.TrimSpace(nodeText(candidate, source))
}

func collectHeritage(node *sitter.Node, source []byte, declaration *Declaration) {
	walkTree(node, func(child *sitter.Node) { processHeritageNode(child, source, declaration) })
}

func processHeritageNode(node *sitter.Node, source []byte, declaration *Declaration) {
	switch node.Kind() {
	case "extends_clause", "extends_type_clause":
		collectHeritageNames(node, source, declaration.Extends)
	case "implements_clause":
		collectHeritageNames(node, source, declaration.Implements)
	}
}

func collectHeritageNames(clause *sitter.Node, source []byte, result map[string]bool) {
	for _, heritageType := range namedChildren(clause) {
		name := directHeritageName(heritageType, source)
		if name == "" {
			name = strings.TrimSpace(nodeText(heritageType, source))
		}
		if name != "" {
			result[name] = true
		}
	}
}

func (analyzer *sourceAnalyzer) analyzeDeclaration(node *sitter.Node, exported, defaultExport bool) {
	kind, ok := declarationKind(node.Kind())
	if !ok {
		return
	}
	name := nodeText(node.ChildByFieldName("name"), analyzer.text)
	if name == "" {
		return
	}
	declaration := &Declaration{Name: name, Kind: kind, Language: "typescript", ModuleID: analyzer.moduleID, File: analyzer.result.SourcePaths[absolutePath(analyzer.source.Path)], Location: analyzer.location(node), Extends: map[string]bool{}, Implements: map[string]bool{}}
	analyzer.collectMembers(node, declaration)
	collectHeritage(node, analyzer.text, declaration)
	key := analyzer.moduleID + ":" + name
	if existing := analyzer.result.TSDeclarations[key]; existing != nil {
		if existing.Kind == "interface" && declaration.Kind == "interface" {
			mergeTypeScriptInterfaces(existing, declaration)
		} else {
			analyzer.result.duplicateErrors = append(analyzer.result.duplicateErrors, fmt.Sprintf("incompatible TypeScript declarations %s in %s", name, analyzer.source.Path))
		}
		declaration = existing
	} else {
		analyzer.result.TSDeclarations[key] = declaration
	}
	collectHeritage(node, analyzer.text, declaration)
	analyzer.result.TSDeclarations[analyzer.moduleID+":"+name] = declaration
	analyzer.addSymbol(name, "class", node, nil, name)
	analyzer.recordExport(name, exported, defaultExport)
}

func (analyzer *sourceAnalyzer) collectMembers(node *sitter.Node, declaration *Declaration) {
	body := node.ChildByFieldName("body")
	if body == nil {
		return
	}
	for _, child := range namedChildren(body) {
		member, ok := memberFromNode(child, analyzer.text)
		if !ok {
			continue
		}
		member.ModuleID = analyzer.moduleID
		member.File = analyzer.result.SourcePaths[absolutePath(analyzer.source.Path)]
		member.Location = analyzer.location(child)
		declaration.Members = append(declaration.Members, member)
		if member.Kind == "method" {
			analyzer.addSymbol(declaration.Name+"."+member.Name, "method", child, child.ChildByFieldName("body"), declaration.Name)
		}
	}
}

func (analyzer *sourceAnalyzer) analyzeFunction(node *sitter.Node, exported, defaultExport bool) {
	if node.Kind() != "function_declaration" && node.Kind() != "function_signature" {
		return
	}
	name := nodeText(node.ChildByFieldName("name"), analyzer.text)
	if name == "" {
		return
	}
	function := Member{Kind: "method", Name: name, Visibility: "public", Language: "typescript", ModuleID: analyzer.moduleID, File: analyzer.result.SourcePaths[absolutePath(analyzer.source.Path)], Location: analyzer.location(node), Async: childHasText(node, analyzer.text, "async"), Parameters: parameterTypes(node, analyzer.text), Type: nodeType(node, analyzer.text)}
	analyzer.result.Functions[name] = append(analyzer.result.Functions[name], function)
	analyzer.addSymbol(name, "function", node, node.ChildByFieldName("body"), "")
	analyzer.recordExport(name, exported, defaultExport)
}

func (analyzer *sourceAnalyzer) recordExport(name string, exported, defaultExport bool) {
	if exported {
		analyzer.result.Exports[name] = true
		analyzer.result.ExportVariants["typescript:"+name] = true
		analyzer.recordTypeScriptExport(name, name)
	}
	if defaultExport {
		analyzer.result.DefaultExports[name] = true
		analyzer.result.DefaultExportVariants["typescript:"+name] = true
		analyzer.result.TSDefaultExports[analyzer.moduleID] = name
		analyzer.recordTypeScriptExport("default", name)
	}
}

func (analyzer *sourceAnalyzer) recordTypeScriptExport(exported, local string) {
	if analyzer.result.TSExports[analyzer.moduleID] == nil {
		analyzer.result.TSExports[analyzer.moduleID] = map[string]bool{}
	}
	if analyzer.result.TSExportNames[analyzer.moduleID] == nil {
		analyzer.result.TSExportNames[analyzer.moduleID] = map[string]string{}
	}
	analyzer.result.TSExports[analyzer.moduleID][local] = true
	analyzer.result.TSExportNames[analyzer.moduleID][exported] = local
}

func (analyzer *sourceAnalyzer) analyzeTopLevel(statement *sitter.Node) {
	if statement.Kind() == "import_statement" {
		analyzer.analyzeImport(statement)
		return
	}
	exported, defaultExport := false, false
	if statement.Kind() == "export_statement" {
		analyzer.analyzeExportSpecifiers(statement)
		exported = true
		defaultExport = hasChildKind(statement, "default")
		statement = statement.ChildByFieldName("declaration")
		if statement == nil {
			return
		}
	}
	analyzer.analyzeDeclaration(statement, exported, defaultExport)
	analyzer.analyzeFunction(statement, exported, defaultExport)
}

func finalizeTypeScriptIndexes(result *Analysis) {
	finalizeTypeScriptDeclarations(result)
	finalizeTypeScriptSymbols(result)
}

func finalizeTypeScriptDeclarations(result *Analysis) {
	groups := map[string][]*Declaration{}
	for _, key := range sortedKeys(result.TSDeclarations) {
		declaration := result.TSDeclarations[key]
		groups[declaration.Name] = append(groups[declaration.Name], declaration)
	}
	for _, name := range sortedKeys(groups) {
		if len(groups[name]) == 1 {
			result.Declarations[name] = groups[name][0]
			result.DeclarationVariants["typescript:"+name] = groups[name][0]
		} else {
			delete(result.Declarations, name)
			delete(result.DeclarationVariants, "typescript:"+name)
		}
	}
}

func finalizeTypeScriptSymbols(result *Analysis) {
	groups := map[string][]*Symbol{}
	for _, key := range sortedKeys(result.TSSymbolIndex) {
		symbol := result.TSSymbolIndex[key]
		groups[symbol.Name] = append(groups[symbol.Name], symbol)
	}
	for _, name := range sortedKeys(groups) {
		if len(groups[name]) == 1 {
			result.Symbols[name] = groups[name][0]
			result.SymbolVariants["typescript:"+name] = groups[name][0]
		} else {
			delete(result.Symbols, name)
			delete(result.SymbolVariants, "typescript:"+name)
		}
	}
}

func parseSource(source Source) (*sitter.Tree, error) {
	parser := sitter.NewParser()
	defer parser.Close()
	language := typescript.LanguageTypescript()
	if strings.HasSuffix(source.Path, ".tsx") {
		language = typescript.LanguageTSX()
	}
	if err := parser.SetLanguage(sitter.NewLanguage(language)); err != nil {
		return nil, fmt.Errorf("set TypeScript language: %w", err)
	}
	tree := parser.Parse([]byte(source.Text), nil)
	if tree == nil {
		return nil, fmt.Errorf("parse %s: tree-sitter returned no tree", source.Path)
	}
	if tree.RootNode().HasError() {
		err := malformedSourceError(source.Path, tree.RootNode())
		tree.Close()
		return nil, err
	}
	return tree, nil
}

// Analyze parses supported sources with their Tree-sitter language adapters and builds a shared model.
func Analyze(sources []Source) (*Analysis, error) {
	result := newAnalysis()
	definitions := registeredLanguages()
	sessions := make(map[string]languageAnalysis, len(definitions))
	for _, definition := range definitions {
		sessions[definition.info.ID] = definition.newAnalysis(result, sources)
	}
	prepareSourcePaths(result, sources)
	for _, source := range sources {
		definition, ok := languageDefinitionForPath(source.Path)
		if !ok || !definition.acceptsSource(source.Path) {
			return nil, unsupportedLanguageError(source.Path)
		}
		if err := sessions[definition.info.ID].Analyze(source); err != nil {
			return nil, err
		}
	}
	if len(result.duplicateErrors) > 0 {
		sort.Strings(result.duplicateErrors)
		return nil, fmt.Errorf("%s", strings.Join(result.duplicateErrors, "; "))
	}
	for _, definition := range definitions {
		if err := sessions[definition.info.ID].Finalize(); err != nil {
			return nil, err
		}
	}
	removeAmbiguousGenericEntries(result)
	enrichNavigationGraph(result)
	return result, nil
}

func finalizeGoIndexes(result *Analysis) {
	declarations := map[string][]*Declaration{}
	for _, key := range sortedKeys(result.GoDeclarations) {
		declaration := result.GoDeclarations[key]
		declarations[declaration.Name] = append(declarations[declaration.Name], declaration)
	}
	for _, name := range sortedKeys(declarations) {
		if len(declarations[name]) == 1 {
			result.Declarations[name] = declarations[name][0]
			result.DeclarationVariants["go:"+name] = declarations[name][0]
		} else {
			delete(result.Declarations, name)
			delete(result.DeclarationVariants, "go:"+name)
		}
	}
	finalizeGoSymbols(result)
}

func finalizeGoSymbols(result *Analysis) {
	symbols := map[string][]*Symbol{}
	for _, key := range sortedKeys(result.GoSymbolIndex) {
		symbol := result.GoSymbolIndex[key]
		symbols[symbol.Name] = append(symbols[symbol.Name], symbol)
	}
	for _, name := range sortedKeys(symbols) {
		if len(symbols[name]) == 1 {
			result.Symbols[name] = symbols[name][0]
			result.SymbolVariants["go:"+name] = symbols[name][0]
		} else {
			delete(result.Symbols, name)
			delete(result.SymbolVariants, "go:"+name)
		}
	}
}

func removeAmbiguousGenericEntries(result *Analysis) {
	declarationLanguages := map[string]map[string]bool{}
	for _, declaration := range result.DeclarationVariants {
		if declarationLanguages[declaration.Name] == nil {
			declarationLanguages[declaration.Name] = map[string]bool{}
		}
		declarationLanguages[declaration.Name][declaration.Language] = true
	}
	for name, languages := range declarationLanguages {
		if len(languages) > 1 {
			delete(result.Declarations, name)
		}
	}
	symbolLanguages := map[string]map[string]bool{}
	for _, symbol := range result.SymbolVariants {
		if symbolLanguages[symbol.Name] == nil {
			symbolLanguages[symbol.Name] = map[string]bool{}
		}
		symbolLanguages[symbol.Name][symbol.Language] = true
	}
	for name, languages := range symbolLanguages {
		if len(languages) > 1 {
			delete(result.Symbols, name)
		}
	}
}

func analyzeTypeScriptSource(source Source, result *Analysis) error {
	tree, err := parseSource(source)
	if err != nil {
		return err
	}
	defer tree.Close()
	analyzer := sourceAnalyzer{result: result, source: source, text: []byte(source.Text), moduleID: absolutePath(source.Path)}
	for _, statement := range namedChildren(tree.RootNode()) {
		analyzer.analyzeTopLevel(statement)
	}
	language := codeparser.LanguageFor(source.Path)
	result.Navigation.Merge(codeparser.NavigationGraphFromTree(tree.RootNode(), source.Text, language, source.Path))
	return nil
}

func sortedKeys[V any](values map[string]V) []string {
	result := make([]string, 0, len(values))
	for key := range values {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}

func absolutePath(path string) string {
	result, _ := filepath.Abs(path)
	return result
}
