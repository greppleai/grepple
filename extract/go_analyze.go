package extract

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	sitter "github.com/tree-sitter/go-tree-sitter"
	golang "github.com/tree-sitter/tree-sitter-go/bindings/go"
)

type goSourceAnalyzer struct {
	result                 *Analysis
	source                 Source
	text                   []byte
	methods                map[string][]Member
	imports                map[string]string
	packageName, packageID string
}

func analyzeGoSource(source Source, result *Analysis, methods map[string][]Member) error {
	tree, err := parseGoSource(source)
	if err != nil {
		return err
	}
	defer tree.Close()
	packageName := goPackageName(tree.RootNode(), []byte(source.Text))
	if packageName == "" {
		return malformedSourceError(source.Path, tree.RootNode())
	}
	packageID := filepath.Clean(absolutePath(filepath.Dir(source.Path))) + ":" + packageName
	result.GoPackageNames[packageID] = packageName
	registerGoImportPath(result, source.Path, packageID)
	if result.GoPackageImports[packageID] == nil {
		result.GoPackageImports[packageID] = map[string]string{}
	}
	analyzer := goSourceAnalyzer{result: result, source: source, text: []byte(source.Text), methods: methods, imports: map[string]string{}, packageName: packageName, packageID: packageID}
	statements := namedChildren(tree.RootNode())
	// Imports are file-scoped and must be known before route-bearing methods are analyzed.
	for _, statement := range statements {
		if statement.Kind() == "import_declaration" {
			analyzer.analyzeImports(statement)
		}
	}
	for _, statement := range statements {
		if statement.Kind() != "import_declaration" {
			analyzer.analyzeStatement(statement)
		}
	}
	analyzer.collectGoTypeReferences(tree.RootNode())
	return nil
}

func registerGoImportPath(result *Analysis, sourcePath, packageID string) {
	importPath := goImportPath(sourcePath)
	if importPath != "" {
		result.GoPackagePaths[packageID] = importPath
	}
	if importPath == "" {
		return
	}
	if existing, ok := result.GoImportPathIndex[importPath]; ok && existing != packageID {
		result.GoImportPathIndex[importPath] = ""
		return
	}
	result.GoImportPathIndex[importPath] = packageID
}

func goImportPath(sourcePath string) string {
	directory := filepath.Dir(absolutePath(sourcePath))
	for current := directory; ; current = filepath.Dir(current) {
		content, err := os.ReadFile(filepath.Join(current, "go.mod"))
		if err == nil {
			module := goModulePath(string(content))
			relative, relErr := filepath.Rel(current, directory)
			if module == "" || relErr != nil {
				return ""
			}
			if relative == "." {
				return module
			}
			return strings.TrimSuffix(module, "/") + "/" + filepath.ToSlash(relative)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return ""
		}
	}
}

func goModulePath(content string) string {
	for _, line := range strings.Split(content, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "module" {
			return fields[1]
		}
	}
	return ""
}

func goPackageName(root *sitter.Node, source []byte) string {
	for _, child := range namedChildren(root) {
		if child.Kind() == "package_clause" && child.NamedChildCount() > 0 {
			return nodeText(child.NamedChild(0), source)
		}
	}
	return ""
}

func parseGoSource(source Source) (*sitter.Tree, error) {
	parser := sitter.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(sitter.NewLanguage(golang.Language())); err != nil {
		return nil, err
	}
	tree := parser.Parse([]byte(source.Text), nil)
	if tree == nil {
		return nil, malformedSourceError(source.Path, nil)
	}
	if tree.RootNode().HasError() {
		err := malformedSourceError(source.Path, tree.RootNode())
		tree.Close()
		return nil, err
	}
	return tree, nil
}

func (analyzer *goSourceAnalyzer) analyzeStatement(node *sitter.Node) {
	switch node.Kind() {
	case "type_declaration":
		analyzer.analyzeTypes(node)
	case "function_declaration":
		analyzer.analyzeFunction(node)
	case "method_declaration":
		analyzer.analyzeMethod(node)
	case "import_declaration":
		analyzer.analyzeImports(node)
	}
}

func (analyzer *goSourceAnalyzer) recordGoExport(name string) {
	analyzer.result.Exports[name] = true
	analyzer.result.ExportVariants["go:"+name] = true
}

func exportedGoName(name string) bool {
	first, _ := utf8.DecodeRuneInString(name)
	return unicode.IsUpper(first)
}
func goVisibility(name string) string {
	if exportedGoName(name) {
		return "public"
	}
	return "private"
}

func (analyzer *goSourceAnalyzer) analyzeTypes(declaration *sitter.Node) {
	for _, spec := range namedChildren(declaration) {
		analyzer.analyzeTypeSpec(spec)
	}
}

func (analyzer *goSourceAnalyzer) analyzeTypeSpec(spec *sitter.Node) {
	if spec.Kind() != "type_spec" && spec.Kind() != "type_alias" {
		return
	}
	name := nodeText(spec.ChildByFieldName("name"), analyzer.text)
	typeNode := spec.ChildByFieldName("type")
	kind := goDeclarationKind(spec, typeNode)
	if name == "" || kind == "" {
		return
	}
	position := spec.StartPosition()
	underlying, ok := normalizeGoUnderlyingType(nodeText(typeNode, analyzer.text))
	if !ok {
		return
	}
	result := &Declaration{Name: name, Kind: kind, Language: "go", Package: analyzer.packageName, PackageID: analyzer.packageID, File: analyzer.result.SourcePaths[absolutePath(analyzer.source.Path)], Underlying: underlying, FileLocal: hasGoFileLocalMarker(analyzer.source.Text, int(position.Row)), Location: syntaxLocation(analyzer.result.SourcePaths[absolutePath(analyzer.source.Path)], spec), Extends: map[string]bool{}, Implements: map[string]bool{}, StructTags: map[string]GoStructTag{}}
	if kind == "struct" || kind == "interface" {
		analyzer.collectGoMembers(typeNode, result)
	}
	analyzer.recordGoTypeDeclaration(spec, result)
}

func (analyzer *goSourceAnalyzer) recordGoTypeDeclaration(spec *sitter.Node, declaration *Declaration) {
	key := analyzer.packageID + ":" + declaration.Name
	if existing := analyzer.result.GoDeclarations[key]; existing != nil {
		analyzer.addDuplicateGoError("package-level type", declaration.Name, existing.Location, declaration.Location)
	} else if functions := analyzer.result.GoFunctions[key]; len(functions) > 0 {
		analyzer.addDuplicateGoError("package-level type/function", declaration.Name, functions[0].Location, declaration.Location)
	}
	analyzer.result.GoDeclarations[key] = declaration
	analyzer.addGoSymbol(declaration.Name, "class", spec, nil, "", declaration.Name)
	if exportedGoName(declaration.Name) {
		analyzer.recordGoExport(declaration.Name)
	}
}

func hasGoFileLocalMarker(source string, declarationRow int) bool {
	lines := strings.Split(source, "\n")
	for row := declarationRow - 1; row >= 0; row-- {
		comment := strings.TrimLeft(strings.TrimSuffix(lines[row], "\r"), " \t")
		if comment == "//grepple:filelocal" {
			return true
		}
		if !strings.HasPrefix(comment, "//") {
			return false
		}
	}
	return false
}

func (analyzer *goSourceAnalyzer) collectGoTypeReferences(root *sitter.Node) {
	file := analyzer.result.SourcePaths[absolutePath(analyzer.source.Path)]
	walkTree(root, func(node *sitter.Node) {
		if node.Kind() == "type_identifier" {
			analyzer.addGoTypeReference(nodeText(node, analyzer.text), node, file)
			return
		}
		if node.Kind() != "call_expression" {
			return
		}
		callee := node.ChildByFieldName("function")
		if callee != nil && callee.Kind() == "identifier" {
			analyzer.addGoTypeReference(nodeText(callee, analyzer.text), callee, file)
		}
	})
}

func (analyzer *goSourceAnalyzer) addGoTypeReference(name string, node *sitter.Node, file string) {
	if name == "" {
		return
	}
	key := analyzer.packageID + ":" + name
	analyzer.result.GoTypeReferences[key] = append(analyzer.result.GoTypeReferences[key], syntaxLocation(file, node))
}

func goDeclarationKind(spec, typeNode *sitter.Node) string {
	if spec == nil || typeNode == nil {
		return ""
	}
	if spec.Kind() == "type_alias" {
		return "alias"
	}
	switch typeNode.Kind() {
	case "struct_type":
		return "struct"
	case "interface_type":
		return "interface"
	default:
		return "type"
	}
}

func (analyzer *goSourceAnalyzer) collectGoMembers(typeNode *sitter.Node, declaration *Declaration) {
	container := directMemberContainer(typeNode)
	for _, node := range namedChildren(container) {
		switch node.Kind() {
		case "field_declaration":
			analyzer.addGoField(node, declaration)
		case "method_elem":
			analyzer.addGoInterfaceMethod(node, declaration)
		case "type_elem":
			analyzer.addGoEmbeddedType(node, declaration)
		}
	}
}

func directMemberContainer(typeNode *sitter.Node) *sitter.Node {
	for _, child := range namedChildren(typeNode) {
		if child.Kind() == "field_declaration_list" {
			return child
		}
	}
	return typeNode
}

func (analyzer *goSourceAnalyzer) addGoField(node *sitter.Node, declaration *Declaration) {
	typeNode := node.ChildByFieldName("type")
	names := childFieldTexts(node, "name", analyzer.text)
	if len(names) == 0 {
		analyzer.addEmbeddedNode(typeNode, declaration)
		return
	}
	tag := GoStructTag{}
	if tagNode := node.ChildByFieldName("tag"); tagNode != nil {
		tag.Present = true
		tag.Value, _ = strconv.Unquote(nodeText(tagNode, analyzer.text))
	}
	location := syntaxLocation(analyzer.result.SourcePaths[absolutePath(analyzer.source.Path)], node)
	for _, name := range names {
		member := Member{Kind: "property", Name: name, Visibility: goVisibility(name), Type: normalizeGoType(nodeText(typeNode, analyzer.text)), Language: "go", Package: analyzer.packageName, PackageID: analyzer.packageID, File: analyzer.result.SourcePaths[absolutePath(analyzer.source.Path)], Location: location}
		declaration.Members = append(declaration.Members, member)
		declaration.StructTags[name] = tag
	}
}

func childFieldTexts(node *sitter.Node, field string, source []byte) []string {
	var result []string
	for index := uint(0); index < node.ChildCount(); index++ {
		if node.FieldNameForChild(uint32(index)) == field {
			result = append(result, nodeText(node.Child(index), source))
		}
	}
	return result
}

func (analyzer *goSourceAnalyzer) addGoInterfaceMethod(node *sitter.Node, declaration *Declaration) {
	name := nodeText(node.ChildByFieldName("name"), analyzer.text)
	if name == "" {
		return
	}
	declaration.Members = append(declaration.Members, analyzer.goCallableMember(node, name))
}

func (analyzer *goSourceAnalyzer) addGoEmbeddedType(node *sitter.Node, declaration *Declaration) {
	typeNode := node.ChildByFieldName("type")
	if typeNode == nil && node.NamedChildCount() > 0 {
		typeNode = node.NamedChild(0)
	}
	analyzer.addEmbeddedNode(typeNode, declaration)
}

func (analyzer *goSourceAnalyzer) addEmbeddedNode(node *sitter.Node, declaration *Declaration) {
	name := goBaseType(node, analyzer.text)
	if name != "" {
		declaration.Extends[name] = true
	}
}

func goBaseType(node *sitter.Node, source []byte) string {
	if node == nil {
		return ""
	}
	switch node.Kind() {
	case "type_identifier":
		return nodeText(node, source)
	case "pointer_type":
		return goBaseType(node.NamedChild(0), source)
	case "generic_type":
		return goBaseType(node.ChildByFieldName("type"), source)
	case "qualified_type":
		return nodeText(node, source)
	default:
		return ""
	}
}

func (analyzer *goSourceAnalyzer) goCallableMember(node *sitter.Node, name string) Member {
	return Member{Kind: "method", Name: name, Visibility: goVisibility(name), Parameters: analyzer.goParameters(node.ChildByFieldName("parameters")), Type: analyzer.goResult(node.ChildByFieldName("result")), Language: "go", Package: analyzer.packageName, PackageID: analyzer.packageID, File: analyzer.result.SourcePaths[absolutePath(analyzer.source.Path)], Location: syntaxLocation(analyzer.result.SourcePaths[absolutePath(analyzer.source.Path)], node)}
}

func (analyzer *goSourceAnalyzer) addDuplicateGoError(kind, name string, first, second Location) {
	analyzer.result.duplicateErrors = append(analyzer.result.duplicateErrors, fmt.Sprintf(
		"duplicate Go %s declaration %q at %s:%d:%d and %s:%d:%d",
		kind, name, first.Path, first.Line, first.Column, second.Path, second.Line, second.Column,
	))
}

func (analyzer *goSourceAnalyzer) goParameters(list *sitter.Node) []string {
	if list == nil {
		return nil
	}
	var result []string
	for _, parameter := range namedChildren(list) {
		if parameter.Kind() != "parameter_declaration" && parameter.Kind() != "variadic_parameter_declaration" {
			continue
		}
		parameterType := normalizeGoType(nodeText(parameter.ChildByFieldName("type"), analyzer.text))
		if parameter.Kind() == "variadic_parameter_declaration" {
			parameterType = "..." + parameterType
		}
		count := goParameterCount(parameter, analyzer.text)
		for index := 0; index < count; index++ {
			result = append(result, parameterType)
		}
	}
	return result
}

func goParameterCount(node *sitter.Node, source []byte) int {
	count := len(childFieldTexts(node, "name", source))
	if count == 0 {
		return 1
	}
	return count
}

func (analyzer *goSourceAnalyzer) goResult(node *sitter.Node) string {
	if node == nil {
		return ""
	}
	if node.Kind() != "parameter_list" {
		return normalizeGoType(nodeText(node, analyzer.text))
	}
	return "tuple<" + strings.Join(analyzer.goParameters(node), ",") + ">"
}

func (analyzer *goSourceAnalyzer) analyzeFunction(node *sitter.Node) {
	name := nodeText(node.ChildByFieldName("name"), analyzer.text)
	if name == "" {
		return
	}
	analyzer.collectFiberRoutes(node.ChildByFieldName("body"), node.ChildByFieldName("parameters"), "", "")
	member := analyzer.goCallableMember(node, name)
	key := analyzer.packageID + ":" + name
	if functions := analyzer.result.GoFunctions[key]; len(functions) > 0 {
		analyzer.addDuplicateGoError("package-level function", name, functions[0].Location, member.Location)
	} else if declaration := analyzer.result.GoDeclarations[key]; declaration != nil {
		analyzer.addDuplicateGoError("package-level type/function", name, declaration.Location, member.Location)
	}
	analyzer.result.GoFunctions[key] = append(analyzer.result.GoFunctions[key], member)
	analyzer.result.Functions[name] = append(analyzer.result.Functions[name], member)
	analyzer.addGoSymbol(name, "function", node, node.ChildByFieldName("body"), "", "")
	if exportedGoName(name) {
		analyzer.recordGoExport(name)
	}
}

func (analyzer *goSourceAnalyzer) analyzeMethod(node *sitter.Node) {
	name := nodeText(node.ChildByFieldName("name"), analyzer.text)
	owner, receiver := analyzer.receiver(node.ChildByFieldName("receiver"))
	if name == "" || owner == "" {
		return
	}
	member := analyzer.goCallableMember(node, name)
	methodKey := analyzer.packageID + ":" + owner
	for _, existing := range analyzer.methods[methodKey] {
		if existing.Name == name {
			analyzer.addDuplicateGoError("method", owner+"."+name, existing.Location, member.Location)
			break
		}
	}
	analyzer.methods[methodKey] = append(analyzer.methods[methodKey], member)
	analyzer.collectFiberRoutes(node.ChildByFieldName("body"), node.ChildByFieldName("parameters"), receiver, owner)
	analyzer.addGoSymbol(owner+"."+name, "method", node, node.ChildByFieldName("body"), receiver, owner)
}

func (analyzer *goSourceAnalyzer) receiver(node *sitter.Node) (string, string) {
	if node == nil {
		return "", ""
	}
	for _, parameter := range namedChildren(node) {
		if parameter.Kind() != "parameter_declaration" {
			continue
		}
		name := nodeText(parameter.ChildByFieldName("name"), analyzer.text)
		return goBaseType(parameter.ChildByFieldName("type"), analyzer.text), name
	}
	return "", ""
}

type fiberRouter struct {
	prefix string
}

func (analyzer *goSourceAnalyzer) collectFiberRoutes(body, parameters *sitter.Node, receiver, owner string) {
	if body == nil {
		return
	}
	routers := map[string]fiberRouter{}
	for name := range fiberAppParameters(parameters, analyzer.text, analyzer.imports) {
		routers[name] = fiberRouter{}
	}
	handlers := goTypedParameters(parameters, analyzer.text)
	if receiver != "" && owner != "" {
		handlers[receiver] = owner
	}
	analyzer.processFiberNode(body, routers, handlers)
}

func (analyzer *goSourceAnalyzer) processFiberNode(node *sitter.Node, routers map[string]fiberRouter, handlers map[string]string) {
	if node == nil || node.Kind() == "function_literal" {
		return
	}
	switch node.Kind() {
	case "block", "statement_list":
		for _, child := range namedChildren(node) {
			analyzer.processFiberNode(child, routers, handlers)
		}
	case "short_var_declaration", "assignment_statement":
		analyzer.processFiberAssignment(node.ChildByFieldName("left"), node.ChildByFieldName("right"), routers)
	case "var_declaration":
		for _, spec := range namedChildren(node) {
			if spec.Kind() == "var_spec" {
				analyzer.processFiberAssignment(spec, spec.ChildByFieldName("value"), routers)
			}
		}
	case "call_expression":
		analyzer.recordFiberRoute(node, routers, handlers)
	default:
		// Control-flow bodies get a private provenance state. This finds routes
		// inside them without allowing conditional assignments to establish
		// provenance after the control-flow statement.
		nested := cloneFiberRouters(routers)
		for _, child := range namedChildren(node) {
			analyzer.processFiberNode(child, nested, handlers)
		}
		for name := range fiberAssignedNames(node, analyzer.text) {
			delete(routers, name)
		}
	}
}

func (analyzer *goSourceAnalyzer) processFiberAssignment(left, right *sitter.Node, routers map[string]fiberRouter) {
	names := goAssignmentNames(left, analyzer.text)
	var values []*sitter.Node
	if right != nil {
		values = namedChildren(right)
		if right.Kind() != "expression_list" {
			values = []*sitter.Node{right}
		}
	}
	resolved := make([]fiberRouter, len(names))
	valid := make([]bool, len(names))
	for index := range names {
		if index < len(values) {
			resolved[index], valid[index] = analyzer.fiberRouterExpression(values[index], routers)
		}
	}
	for index, name := range names {
		delete(routers, name)
		if valid[index] {
			routers[name] = resolved[index]
		}
	}
}

func goAssignmentNames(left *sitter.Node, source []byte) []string {
	if left == nil {
		return nil
	}
	if left.Kind() == "identifier" {
		return []string{nodeText(left, source)}
	}
	var names []string
	for _, child := range namedChildren(left) {
		if child.Kind() == "identifier" {
			names = append(names, nodeText(child, source))
		}
	}
	return names
}

func (analyzer *goSourceAnalyzer) fiberRouterExpression(expression *sitter.Node, routers map[string]fiberRouter) (fiberRouter, bool) {
	if expression == nil {
		return fiberRouter{}, false
	}
	if expression.Kind() == "identifier" {
		router, ok := routers[nodeText(expression, analyzer.text)]
		return router, ok
	}
	if expression.Kind() != "call_expression" {
		return fiberRouter{}, false
	}
	callee := expression.ChildByFieldName("function")
	if callee == nil || callee.Kind() != "selector_expression" {
		return fiberRouter{}, false
	}
	operand := nodeText(callee.ChildByFieldName("operand"), analyzer.text)
	method := nodeText(callee.ChildByFieldName("field"), analyzer.text)
	if method == "New" && supportedFiberModule(analyzer.imports[operand]) {
		return fiberRouter{}, true
	}
	parent, ok := routers[operand]
	if method != "Group" || !ok {
		return fiberRouter{}, false
	}
	arguments := expression.ChildByFieldName("arguments")
	if arguments == nil || arguments.NamedChildCount() == 0 {
		return fiberRouter{}, false
	}
	prefix, ok := goStringLiteral(arguments.NamedChild(0), analyzer.text)
	if !ok {
		return fiberRouter{}, false
	}
	return fiberRouter{prefix: parent.prefix + prefix}, true
}

func (analyzer *goSourceAnalyzer) recordFiberRoute(node *sitter.Node, routers map[string]fiberRouter, handlers map[string]string) {
	callee := node.ChildByFieldName("function")
	if callee == nil || callee.Kind() != "selector_expression" {
		return
	}
	router, ok := routers[nodeText(callee.ChildByFieldName("operand"), analyzer.text)]
	methodName := nodeText(callee.ChildByFieldName("field"), analyzer.text)
	if !ok || !fiberRouteMethod(methodName) {
		return
	}
	method := strings.ToUpper(methodName)
	arguments := node.ChildByFieldName("arguments")
	if arguments == nil || arguments.NamedChildCount() < 2 {
		return
	}
	routePath, ok := goStringLiteral(arguments.NamedChild(0), analyzer.text)
	if !ok {
		return
	}
	for index := uint(1); index < arguments.NamedChildCount(); index++ {
		handler := arguments.NamedChild(index)
		if handler.Kind() != "selector_expression" {
			continue
		}
		handlerOwner := handlers[nodeText(handler.ChildByFieldName("operand"), analyzer.text)]
		name := nodeText(handler.ChildByFieldName("field"), analyzer.text)
		if handlerOwner == "" || name == "" {
			continue
		}
		analyzer.result.GoFiberRoutes = append(analyzer.result.GoFiberRoutes, FiberRoute{Method: method, Path: router.prefix + routePath, Handler: handlerOwner + "." + name, PackageID: analyzer.packageID, Location: syntaxLocation(analyzer.source.Path, node)})
	}
}

func goStringLiteral(node *sitter.Node, source []byte) (string, bool) {
	if node == nil || (node.Kind() != "interpreted_string_literal" && node.Kind() != "raw_string_literal") {
		return "", false
	}
	value, err := strconv.Unquote(nodeText(node, source))
	return value, err == nil
}

func goTypedParameters(parameters *sitter.Node, source []byte) map[string]string {
	result := map[string]string{}
	if parameters == nil {
		return result
	}
	for _, parameter := range namedChildren(parameters) {
		if parameter.Kind() != "parameter_declaration" {
			continue
		}
		owner := goBaseType(parameter.ChildByFieldName("type"), source)
		if owner == "" {
			continue
		}
		for _, name := range childFieldTexts(parameter, "name", source) {
			result[name] = owner
		}
	}
	return result
}

func cloneFiberRouters(routers map[string]fiberRouter) map[string]fiberRouter {
	result := make(map[string]fiberRouter, len(routers))
	for name, router := range routers {
		result[name] = router
	}
	return result
}

func fiberAssignedNames(node *sitter.Node, source []byte) map[string]bool {
	result := map[string]bool{}
	walkTree(node, func(child *sitter.Node) {
		if child != node && child.Kind() == "function_literal" {
			return
		}
		switch child.Kind() {
		case "short_var_declaration", "assignment_statement":
			for _, name := range goAssignmentNames(child.ChildByFieldName("left"), source) {
				result[name] = true
			}
		case "var_spec":
			for _, name := range goAssignmentNames(child, source) {
				result[name] = true
			}
		}
	})
	return result
}

func fiberAppParameters(parameters *sitter.Node, source []byte, imports map[string]string) map[string]bool {
	result := map[string]bool{}
	if parameters == nil {
		return result
	}
	for _, parameter := range namedChildren(parameters) {
		if parameter.Kind() != "parameter_declaration" {
			continue
		}
		typeName := normalizeGoType(nodeText(parameter.ChildByFieldName("type"), source))
		if !strings.HasPrefix(typeName, "*") || !strings.HasSuffix(typeName, ".App") {
			continue
		}
		qualifier := strings.TrimSuffix(strings.TrimPrefix(typeName, "*"), ".App")
		module := imports[qualifier]
		if !supportedFiberModule(module) {
			continue
		}
		for _, name := range childFieldTexts(parameter, "name", source) {
			result[name] = true
		}
	}
	return result
}
func supportedFiberModule(module string) bool {
	return module == "github.com/gofiber/fiber/v2" || module == "github.com/gofiber/fiber/v3"
}

func fiberRouteMethod(method string) bool {
	switch method {
	case "Get", "Head", "Post", "Put", "Delete", "Connect", "Options", "Trace", "Patch", "All":
		return true
	default:
		return false
	}
}

func (analyzer *goSourceAnalyzer) addGoSymbol(name, kind string, node, body *sitter.Node, receiver, owner string) {
	key := analyzer.packageID + ":" + name
	symbol := analyzer.result.GoSymbolIndex[key]
	if symbol == nil {
		symbol = &Symbol{Name: name, Kind: kind, Language: "go", Package: analyzer.packageName, PackageID: analyzer.packageID, Key: analyzer.packageID + ":" + name, Calls: map[string]bool{}}
		analyzer.result.GoSymbolIndex[symbol.Key] = symbol
	}
	symbol.Locations = append(symbol.Locations, syntaxLocation(analyzer.source.Path, node))
	if body == nil {
		return
	}
	order, calls := collectGoCalls(body, analyzer.text, receiver, owner)
	symbol.CallOrder = append(symbol.CallOrder, order...)
	for _, called := range sortedKeys(calls) {
		symbol.Calls[called] = true
	}
}

func collectGoCalls(body *sitter.Node, source []byte, receiver, owner string) ([]string, map[string]bool) {
	var order []string
	calls := map[string]bool{}
	walkTree(body, func(node *sitter.Node) {
		if node.Kind() != "call_expression" {
			return
		}
		name := goCalledSymbol(node.ChildByFieldName("function"), source, receiver, owner)
		if name != "" {
			order = append(order, name)
			calls[name] = true
		}
	})
	return order, calls
}

func goCalledSymbol(expression *sitter.Node, source []byte, receiver, owner string) string {
	if expression == nil {
		return ""
	}
	if expression.Kind() == "identifier" {
		return nodeText(expression, source)
	}
	if expression.Kind() != "selector_expression" {
		return ""
	}
	operand := expression.ChildByFieldName("operand")
	field := expression.ChildByFieldName("field")
	object := nodeText(operand, source)
	if object == receiver && owner != "" {
		return owner + "." + nodeText(field, source)
	}
	if object != "" {
		return object + "." + nodeText(field, source)
	}
	return ""
}

func (analyzer *goSourceAnalyzer) analyzeImports(node *sitter.Node) {
	walkTree(node, func(spec *sitter.Node) {
		analyzer.analyzeImportSpec(spec)
	})
}

func (analyzer *goSourceAnalyzer) analyzeImportSpec(spec *sitter.Node) {
	if spec.Kind() != "import_spec" {
		return
	}
	pathNode := spec.ChildByFieldName("path")
	if pathNode == nil && spec.NamedChildCount() > 0 {
		pathNode = spec.NamedChild(spec.NamedChildCount() - 1)
	}
	module, err := strconv.Unquote(nodeText(pathNode, analyzer.text))
	if err != nil {
		return
	}
	name := nodeText(spec.ChildByFieldName("name"), analyzer.text)
	if name == "" {
		name = defaultGoImportName(module)
	}
	if name == "_" || name == "." {
		return
	}
	analyzer.imports[name] = module
	analyzer.result.Imports[name] = append(analyzer.result.Imports[name], Import{Source: module, Language: "go", Package: analyzer.packageName, PackageID: analyzer.packageID, File: analyzer.result.SourcePaths[absolutePath(analyzer.source.Path)]})
	analyzer.result.GoPackageImports[analyzer.packageID][name] = module
}

func defaultGoImportName(module string) string {
	if module == "github.com/gofiber/fiber/v2" || module == "github.com/gofiber/fiber/v3" {
		return "fiber"
	}
	return path.Base(module)
}

func mergeGoMethods(result *Analysis, methods map[string][]Member) {
	for _, key := range sortedKeys(methods) {
		if declaration := result.GoDeclarations[key]; declaration != nil {
			declaration.Members = append(declaration.Members, methods[key]...)
		}
	}
	sort.Slice(result.GoFiberRoutes, func(i, j int) bool {
		left, right := result.GoFiberRoutes[i], result.GoFiberRoutes[j]
		leftKey := left.PackageID + "\x00" + left.Method + "\x00" + left.Path + "\x00" + left.Handler + "\x00" + left.Location.Path
		rightKey := right.PackageID + "\x00" + right.Method + "\x00" + right.Path + "\x00" + right.Handler + "\x00" + right.Location.Path
		if leftKey != rightKey {
			return leftKey < rightKey
		}
		if left.Location.Line != right.Location.Line {
			return left.Location.Line < right.Location.Line
		}
		return left.Location.Column < right.Location.Column
	})
}
