package extract

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	codeparser "github.com/greppleai/grepple/internal/parser"
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
	document, err := codeparser.ParseDocument("go", source.Text)
	if err != nil {
		return err
	}
	defer document.Close()
	if document.Root().HasError() {
		return malformedSourceError(source.Path, document)
	}
	var packageName string
	if err := document.Read(func(view codeparser.DocumentView) error {
		packageName = goPackageName(view.Root(), []byte(source.Text))
		return nil
	}); err != nil {
		return err
	}
	if packageName == "" {
		return malformedSourceError(source.Path, document)
	}
	packageID := filepath.Clean(absolutePath(filepath.Dir(source.Path))) + ":" + packageName
	result.PackageNames[packageID] = packageName
	registerGoImportPath(result, source.Path, packageID)
	if result.PackageImports[packageID] == nil {
		result.PackageImports[packageID] = map[string]string{}
	}
	analyzer := goSourceAnalyzer{result: result, source: source, text: []byte(source.Text), methods: methods, imports: map[string]string{}, packageName: packageName, packageID: packageID}
	if err := document.Read(func(view codeparser.DocumentView) error {
		root := view.Root()
		statements := root.NamedChildren()
		// Imports are file-scoped and must be known before declarations are analyzed.
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
		analyzer.collectTypeReferencesByPackage(root)
		return nil
	}); err != nil {
		return err
	}
	graph, _ := codeparser.CachedNavigationGraphFromDocument(document, source.Path)
	result.Navigation.Merge(graph)
	return nil
}

func registerGoImportPath(result *Analysis, sourcePath, packageID string) {
	importPath := goImportPath(sourcePath)
	if importPath != "" {
		result.PackagePaths[packageID] = importPath
	}
	if importPath == "" {
		return
	}
	if existing, ok := result.ImportPathPackages[importPath]; ok && existing != packageID {
		result.ImportPathPackages[importPath] = ""
		return
	}
	result.ImportPathPackages[importPath] = packageID
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

func goPackageName(root codeparser.ViewNode, source []byte) string {
	for _, child := range root.NamedChildren() {
		if child.Kind() == "package_clause" && child.NamedChildCount() > 0 {
			return nodeText(child.NamedChild(0), source)
		}
	}
	return ""
}

func (analyzer *goSourceAnalyzer) analyzeStatement(node codeparser.ViewNode) {
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

func (analyzer *goSourceAnalyzer) analyzeTypes(declaration codeparser.ViewNode) {
	for _, spec := range declaration.NamedChildren() {
		analyzer.analyzeTypeSpec(spec)
	}
}

func (analyzer *goSourceAnalyzer) analyzeTypeSpec(spec codeparser.ViewNode) {
	if spec.Kind() != "type_spec" && spec.Kind() != "type_alias" {
		return
	}
	name := nodeText(spec.ChildByFieldName("name"), analyzer.text)
	typeNode := spec.ChildByFieldName("type")
	kind := goDeclarationKind(spec, typeNode)
	if name == "" || kind == "" {
		return
	}
	position := spec.Range().Start
	underlying, ok := normalizeGoUnderlyingType(nodeText(typeNode, analyzer.text))
	if !ok {
		return
	}
	result := &Declaration{Name: name, Kind: kind, Language: "go", Package: analyzer.packageName, PackageID: analyzer.packageID, File: analyzer.result.SourcePaths[absolutePath(analyzer.source.Path)], Underlying: underlying, FileLocal: hasGoFileLocalMarker(analyzer.source.Text, position.Line-1), Location: syntaxLocation(analyzer.result.SourcePaths[absolutePath(analyzer.source.Path)], spec), Extends: map[string]bool{}, Implements: map[string]bool{}, StructTags: map[string]GoStructTag{}}
	if kind == "struct" || kind == "interface" {
		analyzer.collectGoMembers(typeNode, result)
	}
	analyzer.recordGoTypeDeclaration(spec, result)
}

func (analyzer *goSourceAnalyzer) recordGoTypeDeclaration(spec codeparser.ViewNode, declaration *Declaration) {
	key := analyzer.packageID + ":" + declaration.Name
	if existing := analyzer.result.PackageDeclarations[key]; existing != nil {
		analyzer.addDuplicateGoError("package-level type", declaration.Name, existing.Location, declaration.Location)
	} else if functions := analyzer.result.FunctionsByPackage[key]; len(functions) > 0 {
		analyzer.addDuplicateGoError("package-level type/function", declaration.Name, functions[0].Location, declaration.Location)
	}
	analyzer.result.PackageDeclarations[key] = declaration
	analyzer.addGoSymbol(declaration.Name, "class", spec, codeparser.ViewNode{}, "", declaration.Name)
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

func (analyzer *goSourceAnalyzer) collectTypeReferencesByPackage(root codeparser.ViewNode) {
	file := analyzer.result.SourcePaths[absolutePath(analyzer.source.Path)]
	codeparser.WalkNamedView(root, func(node codeparser.ViewNode) {
		if node.Kind() == "type_identifier" {
			analyzer.addGoTypeReference(nodeText(node, analyzer.text), node, file)
			return
		}
		if node.Kind() != "call_expression" {
			return
		}
		callee := node.ChildByFieldName("function")
		if callee.Valid() && callee.Kind() == "identifier" {
			analyzer.addGoTypeReference(nodeText(callee, analyzer.text), callee, file)
		}
	})
}

func (analyzer *goSourceAnalyzer) addGoTypeReference(name string, node codeparser.ViewNode, file string) {
	if name == "" {
		return
	}
	key := analyzer.packageID + ":" + name
	analyzer.result.TypeReferencesByPackage[key] = append(analyzer.result.TypeReferencesByPackage[key], syntaxLocation(file, node))
}

func goDeclarationKind(spec, typeNode codeparser.ViewNode) string {
	if !spec.Valid() || !typeNode.Valid() {
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

func (analyzer *goSourceAnalyzer) collectGoMembers(typeNode codeparser.ViewNode, declaration *Declaration) {
	container := directMemberContainer(typeNode)
	for _, node := range container.NamedChildren() {
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

func directMemberContainer(typeNode codeparser.ViewNode) codeparser.ViewNode {
	for _, child := range typeNode.NamedChildren() {
		if child.Kind() == "field_declaration_list" {
			return child
		}
	}
	return typeNode
}

func (analyzer *goSourceAnalyzer) addGoField(node codeparser.ViewNode, declaration *Declaration) {
	typeNode := node.ChildByFieldName("type")
	names := childFieldTexts(node, "name", analyzer.text)
	if len(names) == 0 {
		analyzer.addEmbeddedNode(typeNode, declaration)
		return
	}
	tag := GoStructTag{}
	if tagNode := node.ChildByFieldName("tag"); tagNode.Valid() {
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

func childFieldTexts(node codeparser.ViewNode, field string, source []byte) []string {
	var result []string
	for index := 0; index < node.ChildCount(); index++ {
		if node.FieldNameForChild(index) == field {
			result = append(result, nodeText(node.Child(index), source))
		}
	}
	return result
}

func (analyzer *goSourceAnalyzer) addGoInterfaceMethod(node codeparser.ViewNode, declaration *Declaration) {
	name := nodeText(node.ChildByFieldName("name"), analyzer.text)
	if name == "" {
		return
	}
	declaration.Members = append(declaration.Members, analyzer.goCallableMember(node, name))
}

func (analyzer *goSourceAnalyzer) addGoEmbeddedType(node codeparser.ViewNode, declaration *Declaration) {
	typeNode := node.ChildByFieldName("type")
	if !typeNode.Valid() && node.NamedChildCount() > 0 {
		typeNode = node.NamedChild(0)
	}
	analyzer.addEmbeddedNode(typeNode, declaration)
}

func (analyzer *goSourceAnalyzer) addEmbeddedNode(node codeparser.ViewNode, declaration *Declaration) {
	name := goBaseType(node, analyzer.text)
	if name != "" {
		declaration.Extends[name] = true
	}
}

func goBaseType(node codeparser.ViewNode, source []byte) string {
	if !node.Valid() {
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

func (analyzer *goSourceAnalyzer) goCallableMember(node codeparser.ViewNode, name string) Member {
	return Member{Kind: "method", Name: name, Visibility: goVisibility(name), Parameters: analyzer.goParameters(node.ChildByFieldName("parameters")), Type: analyzer.goResult(node.ChildByFieldName("result")), Language: "go", Package: analyzer.packageName, PackageID: analyzer.packageID, File: analyzer.result.SourcePaths[absolutePath(analyzer.source.Path)], Location: syntaxLocation(analyzer.result.SourcePaths[absolutePath(analyzer.source.Path)], node)}
}

func (analyzer *goSourceAnalyzer) addDuplicateGoError(kind, name string, first, second Location) {
	analyzer.result.duplicateErrors = append(analyzer.result.duplicateErrors, fmt.Sprintf(
		"duplicate Go %s declaration %q at %s:%d:%d and %s:%d:%d",
		kind, name, first.Path, first.Line, first.Column, second.Path, second.Line, second.Column,
	))
}

func (analyzer *goSourceAnalyzer) goParameters(list codeparser.ViewNode) []string {
	if !list.Valid() {
		return nil
	}
	var result []string
	for _, parameter := range list.NamedChildren() {
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

func goParameterCount(node codeparser.ViewNode, source []byte) int {
	count := len(childFieldTexts(node, "name", source))
	if count == 0 {
		return 1
	}
	return count
}

func (analyzer *goSourceAnalyzer) goResult(node codeparser.ViewNode) string {
	if !node.Valid() {
		return ""
	}
	if node.Kind() != "parameter_list" {
		return normalizeGoType(nodeText(node, analyzer.text))
	}
	return "tuple<" + strings.Join(analyzer.goParameters(node), ",") + ">"
}

func (analyzer *goSourceAnalyzer) analyzeFunction(node codeparser.ViewNode) {
	name := nodeText(node.ChildByFieldName("name"), analyzer.text)
	if name == "" {
		return
	}
	member := analyzer.goCallableMember(node, name)
	key := analyzer.packageID + ":" + name
	if functions := analyzer.result.FunctionsByPackage[key]; len(functions) > 0 {
		analyzer.addDuplicateGoError("package-level function", name, functions[0].Location, member.Location)
	} else if declaration := analyzer.result.PackageDeclarations[key]; declaration != nil {
		analyzer.addDuplicateGoError("package-level type/function", name, declaration.Location, member.Location)
	}
	analyzer.result.FunctionsByPackage[key] = append(analyzer.result.FunctionsByPackage[key], member)
	analyzer.result.Functions[name] = append(analyzer.result.Functions[name], member)
	analyzer.addGoSymbol(name, "function", node, node.ChildByFieldName("body"), "", "")
	if exportedGoName(name) {
		analyzer.recordGoExport(name)
	}
}

func (analyzer *goSourceAnalyzer) analyzeMethod(node codeparser.ViewNode) {
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
	analyzer.addGoSymbol(owner+"."+name, "method", node, node.ChildByFieldName("body"), receiver, owner)
}

func (analyzer *goSourceAnalyzer) receiver(node codeparser.ViewNode) (string, string) {
	if !node.Valid() {
		return "", ""
	}
	for _, parameter := range node.NamedChildren() {
		if parameter.Kind() != "parameter_declaration" {
			continue
		}
		name := nodeText(parameter.ChildByFieldName("name"), analyzer.text)
		return goBaseType(parameter.ChildByFieldName("type"), analyzer.text), name
	}
	return "", ""
}

func (analyzer *goSourceAnalyzer) addGoSymbol(name, kind string, node, _ codeparser.ViewNode, receiver, owner string) {
	key := analyzer.packageID + ":" + name
	symbol := analyzer.result.PackageSymbols[key]
	if symbol == nil {
		symbol = &Symbol{Name: name, Kind: kind, Language: "go", Package: analyzer.packageName, PackageID: analyzer.packageID, Key: analyzer.packageID + ":" + name, Owner: owner, Receiver: receiver, Calls: map[string]bool{}}
		analyzer.result.PackageSymbols[symbol.Key] = symbol
	}
	symbol.Locations = append(symbol.Locations, syntaxLocation(analyzer.source.Path, node))
}

func (analyzer *goSourceAnalyzer) analyzeImports(node codeparser.ViewNode) {
	codeparser.WalkNamedView(node, func(spec codeparser.ViewNode) {
		analyzer.analyzeImportSpec(spec)
	})
}

func (analyzer *goSourceAnalyzer) analyzeImportSpec(spec codeparser.ViewNode) {
	if spec.Kind() != "import_spec" {
		return
	}
	pathNode := spec.ChildByFieldName("path")
	if !pathNode.Valid() && spec.NamedChildCount() > 0 {
		pathNode = spec.NamedChild(spec.NamedChildCount() - 1)
	}
	module, err := strconv.Unquote(nodeText(pathNode, analyzer.text))
	if err != nil {
		return
	}
	name := nodeText(spec.ChildByFieldName("name"), analyzer.text)
	if name == "" {
		name = path.Base(module)
	}
	if name == "_" || name == "." {
		return
	}
	analyzer.imports[name] = module
	analyzer.result.Imports[name] = append(analyzer.result.Imports[name], Import{Source: module, Language: "go", Package: analyzer.packageName, PackageID: analyzer.packageID, File: analyzer.result.SourcePaths[absolutePath(analyzer.source.Path)]})
	analyzer.result.PackageImports[analyzer.packageID][name] = module
}

func mergeGoMethods(result *Analysis, methods map[string][]Member) {
	for _, key := range sortedKeys(methods) {
		if declaration := result.PackageDeclarations[key]; declaration != nil {
			declaration.Members = append(declaration.Members, methods[key]...)
		}
	}
}

func finalizeGoIndexes(result *Analysis) {
	declarations := map[string][]*Declaration{}
	for _, key := range sortedKeys(result.PackageDeclarations) {
		declaration := result.PackageDeclarations[key]
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
	for _, key := range sortedKeys(result.PackageSymbols) {
		symbol := result.PackageSymbols[key]
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
