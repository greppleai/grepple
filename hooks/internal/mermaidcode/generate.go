package mermaidcode

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// GenerateOptions bounds diagram graph traversal.
type GenerateOptions struct {
	Depth, MaxNodes int
	// DepthSet distinguishes an explicit zero depth from the default.
	DepthSet bool
}

func classOptions(options GenerateOptions) (int, int) { return generationOptions(options, 1, 20) }
func flowOptions(options GenerateOptions) (int, int)  { return generationOptions(options, 6, 24) }
func generationOptions(options GenerateOptions, defaultDepth, defaultNodes int) (int, int) {
	depth, nodes := options.Depth, options.MaxNodes
	if !options.DepthSet && depth == 0 {
		depth = defaultDepth
	}
	if nodes == 0 {
		nodes = defaultNodes
	}
	return depth, nodes
}

func declarationDependencies(declaration *Declaration, all map[string]*Declaration) []string {
	result := map[string]bool{}
	copyKnownNames(declaration.Extends, all, result)
	copyKnownNames(declaration.Implements, all, result)
	for _, member := range declaration.Members {
		if member.Kind == "property" {
			addTypeDependencies(member.Type, declaration.Name, all, result)
		}
	}
	return sortedKeys(result)
}

func copyKnownNames(names map[string]bool, all map[string]*Declaration, result map[string]bool) {
	for _, name := range sortedKeys(names) {
		if all[name] != nil {
			result[name] = true
		}
	}
}

func addTypeDependencies(memberType, owner string, all map[string]*Declaration, result map[string]bool) {
	for _, name := range sortedKeys(all) {
		if name != owner && typeReferences(memberType, name) {
			result[name] = true
		}
	}
}

type traversalItem struct {
	name  string
	depth int
}

func selectClasses(entry string, all map[string]*Declaration, depth, nodeLimit int) []string {
	pending := []traversalItem{{entry, 0}}
	seen := map[string]bool{}
	var result []string
	for len(pending) > 0 && len(result) < nodeLimit {
		current := pending[0]
		pending = pending[1:]
		if seen[current.name] || all[current.name] == nil {
			continue
		}
		seen[current.name] = true
		result = append(result, current.name)
		if current.depth < depth {
			pending = appendDependencies(pending, declarationDependencies(all[current.name], all), current.depth, seen)
		}
	}
	return result
}

func appendDependencies(pending []traversalItem, names []string, depth int, seen map[string]bool) []traversalItem {
	for _, name := range names {
		if !seen[name] {
			pending = append(pending, traversalItem{name, depth + 1})
		}
	}
	return pending
}

func renderType(value string) string {
	if strings.Count(value, "<") == 1 && strings.Count(value, ">") == 1 {
		value = strings.Replace(strings.Replace(value, "<", "~", 1), ">", "~", 1)
	}
	return encodeMermaidType(value)
}

var renderableName = regexp.MustCompile(`^#?[A-Za-z_$][\w$]*$`)

func renderableType(value string, required bool) bool {
	if value == "" {
		return !required
	}
	return !strings.ContainsAny(value, "\n\r")
}

func encodeMermaidType(value string) string {
	return strings.NewReplacer(
		"&", "&amp;", "{", "&#123;", "}", "&#125;",
		"(", "&#40;", ")", "&#41;", ",", "&#44;",
		";", "&#59;", "\"", "&quot;",
	).Replace(value)
}

func renderMember(member Member) string {
	if !renderableName.MatchString(member.Name) || !renderableType(member.Type, member.Kind == "property") {
		return ""
	}
	visibility := map[string]string{"public": "+", "private": "-", "protected": "#", "package": "~"}[member.Visibility]
	if visibility == "" || !renderableParameters(member.Parameters) {
		return ""
	}
	static := marker(member.Static, "$")
	if member.Kind == "property" {
		return visibility + member.Name + ": " + renderType(member.Type) + static
	}
	async := marker(member.Async, "async ")
	return visibility + async + member.Name + "(" + renderParameters(member.Parameters) + ")" + renderReturnType(member.Type) + static
}

func marker(enabled bool, value string) string {
	if enabled {
		return value
	}
	return ""
}
func renderableParameters(parameters []string) bool {
	for _, parameter := range parameters {
		if !renderableType(parameter, true) {
			return false
		}
	}
	return true
}
func renderParameters(parameters []string) string {
	result := make([]string, 0, len(parameters))
	for _, parameter := range parameters {
		result = append(result, renderType(parameter))
	}
	return strings.Join(result, ", ")
}
func renderReturnType(value string) string {
	if value == "" {
		return ""
	}
	return ": " + renderType(value)
}

func renderClass(name string, declaration *Declaration, analysis *Analysis) ([]string, error) {
	members, err := renderedMembers(name, declaration.Members)
	if err != nil {
		return nil, err
	}
	lines := renderClassBody(name, members)
	if declaration.Kind == "interface" {
		lines = append(lines, "    <<interface>> "+name)
	} else if isExplicitGoDeclarationKind(declaration.Kind) {
		lines = append(lines, "    <<"+declaration.Kind+">> "+name)
	}
	lines = append(lines, "    <<"+declaration.Language+">> "+name)
	if declaration.Language == "go" {
		scope := goPackageScope(declaration.PackageID, declaration.Package, analysis)
		lines = append(lines, "    %% grepple:package "+name+" "+scope)
	} else if declaration.Language == "typescript" {
		lines = append(lines, "    %% grepple:module "+name+" "+analysis.TSModulePaths[declaration.ModuleID])
	}
	if declaration.Language == "go" && (declaration.Kind == "alias" || declaration.Kind == "type") {
		lines = append(lines, "    %% grepple:underlying "+name+" "+declaration.Underlying)
	}
	if declaration.File != "" {
		lines = append(lines, "    %% grepple:file "+name+" "+declaration.File)
	}
	if declaration.Language == "go" && declaration.Kind == "struct" {
		for _, field := range sortedKeys(declaration.StructTags) {
			tag := declaration.StructTags[field]
			if tag.Present {
				lines = append(lines, "    %% grepple:struct-tag "+name+" "+field+" "+strconv.Quote(tag.Value))
			}
		}
	}
	if declaration.Language == "go" && declaration.FileLocal {
		lines = append(lines, "    %% grepple:filelocal "+name)
	}
	exported := analysis.Exports[name]
	defaultExported := analysis.DefaultExports[name]
	if declaration.Language == "typescript" {
		exported = analysis.TSExports[declaration.ModuleID][name]
		defaultExported = analysis.TSDefaultExports[declaration.ModuleID] == name
	}
	if exported {
		lines = append(lines, "    <<export>> "+name)
	}
	if defaultExported {
		if !exported {
			lines = append(lines, "    <<export>> "+name)
		}
		lines = append(lines, "    %% grepple:default-export "+name)
	}
	return append(lines, ""), nil
}

func renderedMembers(owner string, members []Member) ([]string, error) {
	result := make([]string, 0, len(members))
	for _, member := range members {
		rendered := renderMember(member)
		if rendered == "" {
			return nil, fmt.Errorf("cannot render selected member %s.%s with type %q", owner, member.Name, member.Type)
		}
		result = append(result, rendered)
	}
	return result, nil
}

func renderClassBody(name string, members []string) []string {
	if len(members) == 0 {
		return []string{"    class " + name}
	}
	lines := []string{"    class " + name + " {"}
	for _, member := range members {
		lines = append(lines, "        "+member)
	}
	return append(lines, "    }")
}

func classRelations(selected []string, declarations map[string]*Declaration) []string {
	chosen, relations := stringSet(selected), map[string]bool{}
	for _, name := range selected {
		addHeritageRelations(name, declarations[name], chosen, relations)
		addAssociationRelations(name, declarations[name], selected, relations)
	}
	return sortedKeys(relations)
}

func stringSet(values []string) map[string]bool {
	result := map[string]bool{}
	for _, value := range values {
		result[value] = true
	}
	return result
}

func addHeritageRelations(name string, declaration *Declaration, chosen, result map[string]bool) {
	for _, parent := range sortedKeys(declaration.Extends) {
		if chosen[parent] {
			result[fmt.Sprintf("    %s <|-- %s", parent, name)] = true
		}
	}
	for _, parent := range sortedKeys(declaration.Implements) {
		if chosen[parent] {
			result[fmt.Sprintf("    %s <|.. %s", parent, name)] = true
		}
	}
}

func addAssociationRelations(name string, declaration *Declaration, selected []string, result map[string]bool) {
	for _, target := range selected {
		if target == name || declaration.Extends[target] || declaration.Implements[target] {
			continue
		}
		found, many := propertyReference(declaration.Members, target)
		if found {
			result[associationRelation(name, target, many)] = true
		}
	}
}

func propertyReference(members []Member, target string) (bool, bool) {
	found, many := false, false
	for _, member := range members {
		if member.Kind == "property" && typeReferences(member.Type, target) {
			found = true
			many = many || collectionReferences(member.Type, target)
		}
	}
	return found, many
}

func associationRelation(owner, target string, many bool) string {
	if many {
		return fmt.Sprintf("    %s \"1\" --> \"*\" %s", owner, target)
	}
	return fmt.Sprintf("    %s --> %s", owner, target)
}

// GenerateClassDiagram deterministically generates and validates a class diagram.
func GenerateClassDiagram(entry string, entrySource Source, sources []Source, options GenerateOptions) (string, error) {
	if languageForPath(entrySource.Path) == "typescript" {
		return generateTypeScriptClass(entry, entrySource, sources, options)
	}
	allSources := sources
	depth, nodeLimit := classOptions(options)
	entryAnalysis, err := Analyze([]Source{entrySource})
	if err != nil {
		return "", err
	}
	if entryAnalysis.Declarations[entry] == nil {
		return "", fmt.Errorf("Go declaration '%s' was not found in entry file %s", entry, entrySource.Path)
	}
	entryDeclaration := entryAnalysis.Declarations[entry]
	language := entryDeclaration.Language
	identity := entryDeclaration.PackageID
	if language == "typescript" {
		identity = entryDeclaration.ModuleID
	}
	analysis, err := Analyze(allSources)
	if err != nil {
		return "", err
	}
	declarations := map[string]*Declaration{}
	for _, declaration := range analysis.GoDeclarations {
		if declaration.PackageID == identity {
			declarations[declaration.Name] = declaration
		}
	}
	inferGoImplementations(declarations)
	selected := selectClasses(entry, declarations, depth, nodeLimit)
	lines := []string{"classDiagram", fmt.Sprintf("    %%%% grepple:generated entry %s depth %d max-nodes %d", entry, depth, nodeLimit)}
	for _, name := range selected {
		rendered, renderErr := renderClass(name, declarations[name], analysis)
		if renderErr != nil {
			return "", renderErr
		}
		lines = append(lines, rendered...)
	}
	lines = append(lines, classRelations(selected, declarations)...)
	diagram := strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n"
	return validateGeneratedClass(diagram, allSources)
}

func inferGoImplementations(declarations map[string]*Declaration) {
	for _, name := range sortedKeys(declarations) {
		declaration := declarations[name]
		if declaration.Language != "go" || declaration.Kind != "struct" {
			continue
		}
		for _, candidateName := range sortedKeys(declarations) {
			candidate := declarations[candidateName]
			if candidate.Language == "go" && candidate.Kind == "interface" && len(candidate.Members) > 0 && memberSetSatisfies(declaration.Members, candidate.Members) {
				declaration.Implements[candidateName] = true
			}
		}
	}
}

func memberSetSatisfies(actual, required []Member) bool {
	for _, expected := range required {
		matched := false
		for _, candidate := range actual {
			if structuralMembersMatch(expected, candidate) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

func validateGeneratedClass(diagram string, sources []Source) (string, error) {
	diagnostics, err := CheckClassDiagram(diagram, sources)
	if err != nil {
		return "", err
	}
	if len(diagnostics) > 0 {
		return "", fmt.Errorf("Generated class diagram failed validation:\n%s", formatDiagnostics(diagnostics))
	}
	return diagram, nil
}

func formatDiagnostics(diagnostics []Diagnostic) string {
	result := make([]string, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		result = append(result, fmt.Sprintf("line %d: %s", diagnostic.Line, diagnostic.Message))
	}
	return strings.Join(result, "\n")
}

func selectSymbols(entry string, all map[string]*Symbol, depth, nodeLimit int) []string {
	pending := []traversalItem{{entry, 0}}
	seen := map[string]bool{}
	var result []string
	for len(pending) > 0 && len(result) < nodeLimit {
		current := pending[0]
		pending = pending[1:]
		if seen[current.name] || all[current.name] == nil {
			continue
		}
		seen[current.name] = true
		result = append(result, current.name)
		if current.depth < depth {
			pending = appendDependencies(pending, uniqueKnownCalls(all[current.name], all), current.depth, seen)
		}
	}
	return result
}

func uniqueKnownCalls(symbol *Symbol, all map[string]*Symbol) []string {
	seen := map[string]bool{}
	var result []string
	for _, called := range symbol.CallOrder {
		if all[called] != nil && !seen[called] {
			seen[called] = true
			result = append(result, called)
		}
	}
	return result
}

func validateFlowEntry(entry, entryPath string, symbol *Symbol) error {
	if symbol == nil || symbol.Kind == "class" {
		return fmt.Errorf("Function or method '%s' was not found", entry)
	}
	files := map[string]bool{}
	for _, location := range symbol.Locations {
		files[absolutePath(location.Path)] = true
	}
	if !files[absolutePath(entryPath)] {
		return fmt.Errorf("Function or method '%s' was not found in entry file %s", entry, entryPath)
	}
	if len(files) > 1 {
		return fmt.Errorf("Function or method '%s' is ambiguous across: %s", entry, strings.Join(sortedKeys(files), ", "))
	}
	return nil
}

var nonIdentifierCharacter = regexp.MustCompile(`[^A-Za-z0-9_]`)

func flowNodeIDs(selected []string) map[string]string {
	result, used := map[string]string{}, map[string]bool{}
	for _, name := range selected {
		base := flowIDBase(name)
		identifier := base
		for suffix := 2; used[identifier]; suffix++ {
			identifier = fmt.Sprintf("%s_%d", base, suffix)
		}
		used[identifier] = true
		result[name] = identifier
	}
	return result
}
func flowIDBase(name string) string {
	base := nonIdentifierCharacter.ReplaceAllString(name, "_")
	if base == "" || base[0] >= '0' && base[0] <= '9' {
		return "_" + base
	}
	return base
}
func flowLabel(name string) string {
	return strings.ReplaceAll(strings.ReplaceAll(name+"()", "&", "&amp;"), "\"", "&quot;")
}

func renderFlow(entry string, depth, nodeLimit int, selected []string, identifiers map[string]string, symbols map[string]*Symbol) string {
	lines := []string{"flowchart TD", fmt.Sprintf("    %%%% grepple:generated entry %s depth %d max-nodes %d", entry, depth, nodeLimit)}
	for _, name := range selected {
		lines = append(lines, fmt.Sprintf("    %s[\"%s\"]", identifiers[name], flowLabel(name)))
	}
	lines = append(lines, "")
	for _, name := range selected {
		lines = append(lines, fmt.Sprintf("    %%%% grepple:symbol %s %s", identifiers[name], name))
		lines = append(lines, fmt.Sprintf("    %%%% grepple:language %s %s", identifiers[name], symbols[name].Language))
		if symbols[name].Language == "go" {
			lines = append(lines, fmt.Sprintf("    %%%% grepple:package %s %s", identifiers[name], symbols[name].Package))
		}
	}
	lines = append(lines, "")
	return strings.TrimRight(strings.Join(append(lines, flowEdges(selected, identifiers, symbols)...), "\n"), "\n") + "\n"
}

func flowEdges(selected []string, identifiers map[string]string, symbols map[string]*Symbol) []string {
	chosen, edges := stringSet(selected), map[string]bool{}
	for _, name := range selected {
		for _, target := range symbols[name].CallOrder {
			if chosen[target] {
				edges[fmt.Sprintf("    %s --> %s", identifiers[name], identifiers[target])] = true
			}
		}
	}
	return sortedKeys(edges)
}

// GenerateFlowchart deterministically generates and validates a call flowchart.
func GenerateFlowchart(entry, entryPath string, sources []Source, options GenerateOptions) (string, error) {
	depth, nodeLimit := flowOptions(options)
	language := languageForPath(entryPath)
	entrySource := findSourceByPath(sources, entryPath)
	if entrySource == nil {
		return "", fmt.Errorf("entry file is outside supplied sources: %s", entryPath)
	}
	if language == "go" {
		return generateGoFlowchart(entry, *entrySource, sources, depth, nodeLimit)
	}
	return generateTypeScriptFlowchart(entry, *entrySource, sources, depth, nodeLimit)
}

func languageForPath(path string) string {
	if strings.HasSuffix(path, ".go") {
		return "go"
	}
	return "typescript"
}

func findSourceByPath(sources []Source, path string) *Source {
	for index := range sources {
		if absolutePath(sources[index].Path) == absolutePath(path) {
			return &sources[index]
		}
	}
	return nil
}

func sourcesForEntry(sources []Source, language, identity string) ([]Source, error) {
	var result []Source
	for _, source := range sources {
		matches, err := sourceMatchesEntry(source, language, identity)
		if err != nil {
			return nil, err
		}
		if matches {
			result = append(result, source)
		}
	}
	return result, nil
}

func sourceMatchesEntry(source Source, language, identity string) (bool, error) {
	if languageForPath(source.Path) != language {
		return false, nil
	}
	if language == "typescript" {
		return absolutePath(source.Path) == identity, nil
	}
	tree, err := parseGoSource(source)
	if err != nil {
		return false, err
	}
	name := goPackageName(tree.RootNode(), []byte(source.Text))
	tree.Close()
	sourcePackageID := filepath.Clean(absolutePath(filepath.Dir(source.Path))) + ":" + name
	return sourcePackageID == identity, nil
}
