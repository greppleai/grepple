package extract

import (
	"fmt"
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

func selectClasses(entry string, all map[string]*Declaration, depth, nodeLimit int) ([]string, bool) {
	pending := []traversalItem{{entry, 0}}
	seen := map[string]bool{}
	var result []string
	for len(pending) > 0 {
		current := pending[0]
		pending = pending[1:]
		if seen[current.name] || all[current.name] == nil {
			continue
		}
		if len(result) >= nodeLimit {
			return result, true
		}
		seen[current.name] = true
		result = append(result, current.name)
		if current.depth < depth {
			pending = appendDependencies(pending, declarationDependencies(all[current.name], all), current.depth, seen)
		}
	}
	return result, false
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

// mermaidNoteText keeps generated notes single-line and inert.
func mermaidNoteText(value string) string {
	var result strings.Builder
	for _, character := range value {
		switch character {
		case '&':
			result.WriteString("&amp;")
		case '"':
			result.WriteString("&quot;")
		case '<':
			result.WriteString("&lt;")
		case '>':
			result.WriteString("&gt;")
		case '\\':
			result.WriteString("&#92;")
		case '\n':
			result.WriteString("&#10;")
		case '\r':
			result.WriteString("&#13;")
		default:
			if character < 0x20 || character == 0x7f || character == '\u2028' || character == '\u2029' {
				fmt.Fprintf(&result, "&#%d;", character)
			} else {
				result.WriteRune(character)
			}
		}
	}
	return result.String()
}

func renderClass(name string, declaration *Declaration, analysis *Analysis) ([]string, error) {
	members, err := renderedMembers(name, declaration.Members)
	if err != nil {
		return nil, err
	}
	lines := renderClassBody(name, members)
	if note := declarationLocationNote(declaration); note != "" {
		lines = append(lines, "    note for "+name+" \""+mermaidNoteText(note)+"\"")
	}
	if declaration.Language == "swift" && declaration.Kind == "struct" {
		lines = append(lines, "    <<swift>> "+name)
		lines = append(lines, renderDeclarationKind(name, declaration)...)
	} else {
		lines = append(lines, renderDeclarationKind(name, declaration)...)
		lines = append(lines, "    <<"+declaration.Language+">> "+name)
	}
	lines = append(lines, renderDeclarationMetadata(name, declaration, analysis)...)
	lines = append(lines, renderDeclarationExports(name, declaration, analysis)...)
	return append(lines, ""), nil
}

func renderDeclarationKind(name string, declaration *Declaration) []string {
	if declaration.Kind == "interface" {
		return []string{"    <<interface>> " + name}
	}
	if isExplicitDeclarationKind(declaration.Kind) {
		return []string{"    <<" + declaration.Kind + ">> " + name}
	}
	return nil
}

func renderDeclarationMetadata(name string, declaration *Declaration, analysis *Analysis) []string {
	lines := []string{}
	if declaration.PackageID != "" {
		scope := packageDisplayScope(declaration.PackageID, declaration.Package, analysis)
		lines = append(lines, "    %% grepple:package "+name+" "+scope)
	} else if declaration.ModuleID != "" {
		lines = append(lines, "    %% grepple:module "+name+" "+analysis.ModulePaths[declaration.ModuleID])
	}
	if focusedSemanticsFor(declaration.Language).underlyingTypes && (declaration.Kind == "alias" || declaration.Kind == "type") {
		lines = append(lines, "    %% grepple:underlying "+name+" "+declaration.Underlying)
	}
	if declaration.File != "" {
		lines = append(lines, "    %% grepple:file "+name+" "+declaration.File)
	}
	lines = append(lines, renderDeclarationStructTags(name, declaration)...)
	if focusedSemanticsFor(declaration.Language).fileLocalTypes && declaration.FileLocal {
		lines = append(lines, "    %% grepple:filelocal "+name)
	}
	return lines
}

func renderDeclarationStructTags(name string, declaration *Declaration) []string {
	if !focusedSemanticsFor(declaration.Language).structTags || declaration.Kind != "struct" {
		return nil
	}
	lines := []string{}
	for _, field := range sortedKeys(declaration.StructTags) {
		tag := declaration.StructTags[field]
		if tag.Present {
			lines = append(lines, "    %% grepple:struct-tag "+name+" "+field+" "+strconv.Quote(tag.Value))
		}
	}
	return lines
}

func renderDeclarationExports(name string, declaration *Declaration, analysis *Analysis) []string {
	exported := analysis.Exports[name]
	defaultExported := analysis.DefaultExports[name]
	if focusedSemanticsFor(declaration.Language).moduleExports {
		exported = analysis.ModuleExports[declaration.ModuleID][name]
		defaultExported = analysis.ModuleDefaultExports[declaration.ModuleID] == name
	}
	lines := []string{}
	if exported {
		lines = append(lines, "    <<export>> "+name)
	}
	if defaultExported {
		if !exported {
			lines = append(lines, "    <<export>> "+name)
		}
		lines = append(lines, "    %% grepple:default-export "+name)
	}
	return lines
}

func declarationLocationNote(declaration *Declaration) string {
	if declaration.Location.Path == "" || declaration.Location.Line == 0 {
		return ""
	}
	parts := []string{"defined: " + sourceRange(declaration.Location)}
	for _, member := range declaration.Members {
		if member.Location.Line > 0 {
			parts = append(parts, member.Name+"@"+sourceRange(member.Location))
		}
	}
	return strings.Join(parts, "; ")
}

func sourceRange(location Location) string {
	value := location.Path + ":" + strconv.Itoa(location.Line)
	if location.EndLine > location.Line {
		value += "-" + strconv.Itoa(location.EndLine)
	}
	return value
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
	definition, ok := languageDefinitionForPath(entrySource.Path)
	if !ok || !definition.acceptsSource(entrySource.Path) {
		return "", unsupportedLanguageError(entrySource.Path)
	}
	return definition.generateStructure(entry, entrySource, sources, options)
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

// GenerateFlowchart deterministically generates and validates a call flowchart.
func GenerateFlowchart(entry, entryPath string, sources []Source, options GenerateOptions) (string, error) {
	definition, ok := languageDefinitionForPath(entryPath)
	if !ok || !definition.acceptsSource(entryPath) {
		return "", unsupportedLanguageError(entryPath)
	}
	entrySource := findSourceByPath(sources, entryPath)
	if entrySource == nil {
		return "", fmt.Errorf("entry file is outside supplied sources: %s", entryPath)
	}
	depth, nodeLimit := flowOptions(options)
	return definition.generateFlow(entry, *entrySource, sources, depth, nodeLimit)
}

func findSourceByPath(sources []Source, path string) *Source {
	for index := range sources {
		if absolutePath(sources[index].Path) == absolutePath(path) {
			return &sources[index]
		}
	}
	return nil
}
