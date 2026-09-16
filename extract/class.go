package extract

import (
	"fmt"
	"html"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Diagnostic reports one diagram mismatch at a Mermaid source line.
type Diagnostic struct {
	Line    int
	Message string

	sortPath, sortName string
	sortLine           int
}

// DiagramMember is a parsed Mermaid member and its source line.
type DiagramMember struct {
	Member
	Line int
}

// StructTagRequirement is an opt-in exact Go field-tag requirement.
type StructTagRequirement struct {
	Value string
	Line  int
}

// DiagramClass is a parsed Mermaid class-like node and its metadata.
type DiagramClass struct {
	Name, DisplayLabel, Kind, Language, Package, Module, File, Underlying string
	Members                                                               []DiagramMember
	StructTags                                                            map[string]StructTagRequirement
	Line, FileLine, FileLocalLine, UnderlyingLine                         int
	PackageLine, ModuleLine, ImportLine                                   int
	DefaultExportLine                                                     int
	Function, Exported, DefaultExport, Async, Import                      bool
	Exact, FileLocal                                                      bool
	ImportSource                                                          string
}

// Relation is a parsed Mermaid class relationship.
type Relation struct {
	Left, Right, Operator, LeftMultiplicity, RightMultiplicity string
	Label                                                      string
	Line                                                       int
}

// DiagramNote is a parsed single-line Mermaid note. Target is empty for an
// unscoped diagram note.
type DiagramNote struct {
	Text   string
	Target string
	Line   int
}

// ClassDiagram is the parsed class schema in declaration order.
type ClassDiagram struct {
	Classes             map[string]*DiagramClass
	Order               []string
	Relations           []Relation
	Notes               []DiagramNote
	Direction           string
	CompletePackage     string
	CompletePackageLine int
	PackageDefault      string
	PackageDefaultLine  int
	LanguageDefault     string
	LanguageDefaultLine int
	ExactDefault        bool
	ExactDefaultLine    int
}

var classRE = regexp.MustCompile(`^class\s+(\w+)(?:\["([^"\\]*)"\])?(?:\s*\{)?$`)
var namespaceRE = regexp.MustCompile(`^namespace\s+([A-Za-z_]\w*)\s*\{$`)
var classDirectionRE = regexp.MustCompile(`^direction\s+(LR|RL|TB|BT)$`)
var stereotypeRE = regexp.MustCompile(`^<<(.+)>>(?:\s+(\w+))?$`)
var relationRE = regexp.MustCompile(`^(\w+)\s*(?:"([^"]+)")?\s*(<\|\.\.|\.\.\|>|<\|--|--\|>|\.\.>|<\.\.|-->|<--|\*--|o--|--\*|--o|--)\s*(?:"([^"]+)")?\s*(\w+)(?:\s*:\s*(.+))?$`)
var importMetadataRE = regexp.MustCompile(`^%%\s*(?:grepple|pi):import\s+(\w+)\s+from\s+(?:"([^"]+)"|'([^']+)'|(\S+))\s*$`)
var packageMetadataRE = regexp.MustCompile(`^%%\s*(?:grepple|pi):package\s+(\w+)\s+(\S+)\s*$`)
var moduleMetadataRE = regexp.MustCompile(`^%%\s*(?:grepple|pi):module\s+(\w+)\s+(\S+)\s*$`)
var fileMetadataRE = regexp.MustCompile(`^%%\s*(?:grepple|pi):file\s+(\w+)\s+(\S+)\s*$`)
var fileLocalMetadataRE = regexp.MustCompile(`^%%\s*(?:grepple|pi):filelocal\s+(\w+)\s*$`)
var structTagMetadataRE = regexp.MustCompile(`^%%\s*(?:grepple|pi):struct-tag\s+(\w+)\s+(\w+)\s+(.+?)\s*$`)
var underlyingMetadataRE = regexp.MustCompile(`^%%\s*(?:grepple|pi):underlying\s+(\w+)\s+(.+?)\s*$`)
var completePackageMetadataRE = regexp.MustCompile(`^%%\s*(?:grepple|pi):complete-package\s+(\S+)\s*$`)
var packageDefaultMetadataRE = regexp.MustCompile(`^%%\s*(?:grepple|pi):package-default\s+(\S+)\s*$`)
var languageDefaultMetadataRE = regexp.MustCompile(`^%%\s*(?:grepple|pi):language-default\s+([A-Za-z_][\w-]*)\s*$`)
var exactDefaultMetadataRE = regexp.MustCompile(`^%%\s*(?:grepple|pi):exact-default\s*$`)
var defaultExportRE = regexp.MustCompile(`^%%\s*(?:grepple|pi):default-export\s+(\w+)\s*$`)
var generatedMetadataRE = regexp.MustCompile(`^%%\s*(?:grepple|pi):generated\s+entry\s+\S+\s+depth\s+\d+\s+max-nodes\s+\d+\s*$`)
var truncatedMetadataRE = regexp.MustCompile(`^%%\s*(?:grepple|pi):truncated\s+max-nodes\s+\d+\s*$`)
var metadataNamespaceRE = regexp.MustCompile(`^%%\s*(?:grepple|pi):`)
var typeAliases = []struct{ diagram, typescript string }{
	{"String", "string"}, {"Boolean", "boolean"}, {"Number", "number"},
	{"Integer", "number"}, {"int", "number"}, {"float", "number"},
	{"double", "number"}, {"Void", "void"},
}

func normalizeType(value string) string { return normalizeTypeForLanguage(value, "typescript") }

func normalizeTypeForLanguage(value, language string) string {
	if definition, ok := languageDefinitionForID(language); ok {
		return definition.normalizeType(value)
	}
	return normalizeTypeScript(value)
}

func normalizeTypeScript(value string) string {
	value = normalizeMermaidGeneric(value)
	for _, alias := range typeAliases {
		value = regexp.MustCompile(`\b`+alias.diagram+`\b`).ReplaceAllString(value, alias.typescript)
	}
	value = regexp.MustCompile(`\b(?:Array|List)<([^<>]+)>`).ReplaceAllString(value, "$1[]")
	return value
}

func normalizeMermaidGeneric(value string) string {
	return strings.Join(strings.Fields(decodeMermaidGeneric(value)), "")
}

func decodeMermaidGeneric(value string) string {
	value = decodeMermaidType(value)
	value = strings.TrimSpace(value)
	return regexp.MustCompile(`([A-Za-z_$][\w$]*)~([^~]+)~`).ReplaceAllString(value, "$1<$2>")
}

func decodeMermaidType(value string) string {
	return strings.NewReplacer(
		"&#123;", "{", "&#125;", "}",
		"&#40;", "(", "&#41;", ")",
		"&#44;", ",", "&#59;", ";",
		"&quot;", "\"", "&amp;", "&",
	).Replace(value)
}

func splitParameters(value string) []string {
	var result []string
	start, depth := 0, 0
	mermaidGeneric := false
	for index, character := range value {
		depth, mermaidGeneric = parameterDepth(character, depth, mermaidGeneric)
		if character == ',' && depth == 0 {
			result = append(result, strings.TrimSpace(value[start:index]))
			start = index + 1
		}
	}
	if final := strings.TrimSpace(value[start:]); final != "" {
		result = append(result, final)
	}
	return result
}

func parameterDepth(character rune, depth int, mermaidGeneric bool) (int, bool) {
	switch character {
	case '~':
		if mermaidGeneric {
			return depth - 1, false
		}
		return depth + 1, true
	case '<', '(', '[', '{':
		return depth + 1, mermaidGeneric
	case '>', ')', ']', '}':
		if depth > 0 {
			return depth - 1, mermaidGeneric
		}
		return depth, mermaidGeneric
	default:
		return depth, mermaidGeneric
	}
}

func parameterType(value string) string {
	value = decodeMermaidType(value)
	depth, mermaidGeneric := 0, false
	for index, character := range value {
		if character == ':' && depth == 0 {
			return decodeMermaidGeneric(value[index+1:])
		}
		depth, mermaidGeneric = parameterDepth(character, depth, mermaidGeneric)
	}
	trimmed := strings.TrimSpace(value)
	parts := strings.Fields(value)
	if strings.Contains(trimmed, "=>") || strings.ContainsAny(trimmed, "{}[]()|&") {
		return decodeMermaidGeneric(value)
	}
	if len(parts) > 1 && renderableName.MatchString(parts[len(parts)-1]) {
		return decodeMermaidGeneric(strings.Join(parts[:len(parts)-1], " "))
	}
	return decodeMermaidGeneric(value)
}

func parseMember(value string, line int) (DiagramMember, error) {
	if value == "" || !strings.ContainsAny(value[:1], "+-#~") {
		return DiagramMember{}, fmt.Errorf("Line %d: class member must begin with +, -, #, or ~", line)
	}
	visibility := map[byte]string{'+': "public", '-': "private", '#': "protected", '~': "package"}[value[0]]
	declaration, async, static := memberMarkers(strings.TrimSpace(value[1:]))
	if open := strings.Index(declaration, "("); open >= 0 {
		return parseMethod(declaration, visibility, async, static, open, line)
	}
	return parseProperty(declaration, visibility, async, static, line)
}

func memberMarkers(declaration string) (string, bool, bool) {
	async := strings.HasPrefix(declaration, "async ")
	if async {
		declaration = strings.TrimSpace(strings.TrimPrefix(declaration, "async "))
	}
	static := strings.HasSuffix(declaration, "$")
	if static {
		declaration = strings.TrimSpace(strings.TrimSuffix(declaration, "$"))
	}
	return declaration, async, static
}

func parseMethod(declaration, visibility string, async, static bool, open, line int) (DiagramMember, error) {
	closing := strings.LastIndex(declaration, ")")
	if closing < open {
		return DiagramMember{}, fmt.Errorf("Line %d: method is missing ')'", line)
	}
	prefix := strings.Fields(strings.TrimSpace(declaration[:open]))
	if len(prefix) == 0 {
		return DiagramMember{}, fmt.Errorf("Line %d: method must include a name", line)
	}
	name := prefix[len(prefix)-1]
	methodType := methodReturnType(declaration[closing+1:], prefix)
	var parameters []string
	for _, parameter := range splitParameters(declaration[open+1 : closing]) {
		parameters = append(parameters, parameterType(parameter))
	}
	member := Member{Kind: "method", Name: name, Visibility: visibility, Static: static, Async: async, Type: methodType, Parameters: parameters}
	return DiagramMember{Member: member, Line: line}, nil
}

func methodReturnType(suffix string, prefix []string) string {
	suffix = strings.TrimSpace(suffix)
	if strings.HasPrefix(suffix, ":") {
		return decodeMermaidGeneric(suffix[1:])
	}
	if len(prefix) > 1 {
		return decodeMermaidGeneric(strings.Join(prefix[:len(prefix)-1], " "))
	}
	return ""
}

func parseProperty(declaration, visibility string, async, static bool, line int) (DiagramMember, error) {
	if async {
		return DiagramMember{}, fmt.Errorf("Line %d: only methods and functions can be async", line)
	}
	name, propertyType, err := propertyParts(declaration, line)
	if err != nil {
		return DiagramMember{}, err
	}
	member := Member{Kind: "property", Name: name, Visibility: visibility, Static: static, Type: propertyType}
	return DiagramMember{Member: member, Line: line}, nil
}

func propertyParts(declaration string, line int) (string, string, error) {
	if colon := strings.Index(declaration, ":"); colon >= 0 {
		name := strings.TrimSuffix(strings.TrimSpace(declaration[:colon]), "?")
		return name, decodeMermaidGeneric(declaration[colon+1:]), nil
	}
	parts := strings.Fields(declaration)
	if len(parts) < 2 {
		return "", "", fmt.Errorf("Line %d: property must include a name and type", line)
	}
	name := strings.TrimSuffix(parts[len(parts)-1], "?")
	return name, decodeMermaidGeneric(strings.Join(parts[:len(parts)-1], " ")), nil
}

func applyStereotype(class *DiagramClass, stereotype string, line int) error {
	if handled, err := applyLanguageStereotype(class, stereotype, line); handled {
		return err
	}
	switch stereotype {
	case "interface":
		if class.Kind == "struct" || class.Kind == "alias" || class.Kind == "type" {
			return stereotypeConflict(class, stereotype, line)
		}
		class.Kind = "interface"
	case "struct", "alias", "type":
		if class.Language == "javascript" || class.Language == "typescript" || class.Kind == "interface" || isExplicitGoDeclarationKind(class.Kind) && class.Kind != stereotype || class.Function {
			return stereotypeConflict(class, stereotype, line)
		}
		class.Kind, class.Language = stereotype, "go"
	case "function":
		if isExplicitGoDeclarationKind(class.Kind) {
			return stereotypeConflict(class, stereotype, line)
		}
		class.Function = true
	case "export":
		class.Exported = true
	case "async":
		class.Async = true
	case "import":
		class.Import = true
	case "exact":
		class.Exact = true
	case "class":
		return fmt.Errorf("Line %d: Mermaid reserves 'class'; omit the stereotype because nodes default to classes", line)
	default:
		return unsupportedStereotype(stereotype, line)
	}
	return nil
}

func applyLanguageStereotype(class *DiagramClass, stereotype string, line int) (bool, error) {
	if !isSupportedLanguage(stereotype) {
		return false, nil
	}
	if class.Language != "" && class.Language != stereotype || stereotype != "go" && isExplicitGoDeclarationKind(class.Kind) {
		return true, stereotypeConflict(class, stereotype, line)
	}
	class.Language = stereotype
	return true, nil
}

func isExplicitGoDeclarationKind(kind string) bool {
	return kind == "struct" || kind == "alias" || kind == "type"
}

func stereotypeConflict(class *DiagramClass, stereotype string, line int) error {
	return fmt.Errorf("Line %d: stereotype '%s' conflicts with the kind or language of '%s'", line, stereotype, class.Name)
}

func unsupportedStereotype(stereotype string, line int) error {
	if strings.HasPrefix(stereotype, "import ") {
		return fmt.Errorf("Line %d: import sources are not valid Mermaid stereotypes; use metadata", line)
	}
	return fmt.Errorf("Line %d: unsupported stereotype '%s'", line, stereotype)
}

type classParser struct {
	diagram       *ClassDiagram
	current       *DiagramClass
	namespace     string
	namespaceLine int
	sawHeader     bool
}

func newClassParser() *classParser {
	return &classParser{diagram: &ClassDiagram{Classes: map[string]*DiagramClass{}}}
}

func (parser *classParser) parseLine(value string, line int) error {
	if value == "" {
		return nil
	}
	if strings.HasPrefix(value, "%%") {
		return parser.parseMetadata(value, line)
	}
	if !parser.sawHeader {
		return parser.parseHeader(value, line)
	}
	if parser.current != nil {
		return parser.parseMemberLine(value, line)
	}
	return parser.parseTopLevel(value, line)
}

func (parser *classParser) parseHeader(value string, line int) error {
	if value != "classDiagram" {
		return fmt.Errorf("Line %d: expected 'classDiagram'", line)
	}
	parser.sawHeader = true
	return nil
}

func (parser *classParser) parseMetadata(value string, line int) error {
	if matched, err := parser.parseDiagramMetadata(value, line); matched {
		return err
	}
	if matched, err := parser.parseGoMetadata(value, line); matched {
		return err
	}
	if matched, err := parser.parseClassMetadata(value, line); matched {
		return err
	}
	return validateNamespacedClassMetadata(value, line)
}

func (parser *classParser) parseDiagramMetadata(value string, line int) (bool, error) {
	if generatedMetadataRE.MatchString(value) || truncatedMetadataRE.MatchString(value) {
		return true, nil
	}
	if match := completePackageMetadataRE.FindStringSubmatch(value); match != nil {
		if !parser.sawHeader {
			return true, nil
		}
		return true, parser.applyCompletePackageMetadata(match[1], line)
	}
	if match := packageDefaultMetadataRE.FindStringSubmatch(value); match != nil {
		return true, parser.applyPackageDefault(match[1], line)
	}
	if match := languageDefaultMetadataRE.FindStringSubmatch(value); match != nil {
		return true, parser.applyLanguageDefault(match[1], line)
	}
	if exactDefaultMetadataRE.MatchString(value) {
		return true, parser.applyExactDefault(line)
	}
	return false, nil
}

func (parser *classParser) parseGoMetadata(value string, line int) (bool, error) {
	if match := structTagMetadataRE.FindStringSubmatch(value); match != nil {
		if !parser.sawHeader {
			return true, nil
		}
		return true, parser.applyStructTagMetadata(match[1], match[2], match[3], line)
	}
	if match := underlyingMetadataRE.FindStringSubmatch(value); match != nil {
		if !parser.sawHeader {
			return true, nil
		}
		return true, parser.applyUnderlyingMetadata(match[1], match[2], line)
	}
	return false, nil
}

func (parser *classParser) parseClassMetadata(value string, line int) (bool, error) {
	if match := packageMetadataRE.FindStringSubmatch(value); match != nil {
		if !parser.sawHeader {
			return true, nil
		}
		return true, parser.applyClassScope(match[1], match[2], "go", line)
	}
	if match := moduleMetadataRE.FindStringSubmatch(value); match != nil {
		if !parser.sawHeader {
			return true, nil
		}
		language := "typescript"
		if class := parser.diagram.Classes[match[1]]; class != nil && class.Language != "" && class.Language != "go" {
			language = class.Language
		}
		return true, parser.applyClassScope(match[1], match[2], language, line)
	}
	if match := fileMetadataRE.FindStringSubmatch(value); match != nil {
		return true, parser.applyFileMetadata(match[1], match[2], line)
	}
	if match := fileLocalMetadataRE.FindStringSubmatch(value); match != nil {
		return true, parser.applyFileLocalMetadata(match[1], line)
	}
	if match := importMetadataRE.FindStringSubmatch(value); match != nil {
		return true, parser.applyImportMetadata(match[1], match[2]+match[3]+match[4], line)
	}
	if match := defaultExportRE.FindStringSubmatch(value); match != nil {
		return true, parser.applyDefaultExportMetadata(match[1], line)
	}
	return false, nil
}

func (parser *classParser) applyFileMetadata(name, file string, line int) error {
	if !parser.sawHeader {
		return nil
	}
	class, err := parser.metadataTarget(name, line)
	if err != nil {
		return err
	}
	if class.FileLine != 0 {
		return fmt.Errorf("Line %d: duplicate file directive for '%s'", line, class.Name)
	}
	class.File, class.FileLine = normalizeFileMetadata(file), line
	return nil
}

func (parser *classParser) applyFileLocalMetadata(name string, line int) error {
	if !parser.sawHeader {
		return nil
	}
	class, err := parser.metadataTarget(name, line)
	if err != nil {
		return err
	}
	if class.FileLocalLine != 0 {
		return fmt.Errorf("Line %d: duplicate filelocal directive for '%s'", line, class.Name)
	}
	class.FileLocal, class.FileLocalLine = true, line
	return nil
}

func (parser *classParser) applyImportMetadata(name, source string, line int) error {
	if !parser.sawHeader {
		return nil
	}
	class, err := parser.metadataTarget(name, line)
	if err != nil {
		return err
	}
	if class.ImportLine != 0 {
		return fmt.Errorf("Line %d: duplicate import directive for '%s'", line, class.Name)
	}
	class.Import, class.ImportSource, class.ImportLine = true, source, line
	return nil
}

func (parser *classParser) applyDefaultExportMetadata(name string, line int) error {
	if !parser.sawHeader {
		return nil
	}
	class, err := parser.metadataTarget(name, line)
	if err != nil {
		return err
	}
	if class.DefaultExportLine != 0 {
		return fmt.Errorf("Line %d: duplicate default-export directive for '%s'", line, class.Name)
	}
	class.Exported, class.DefaultExport, class.DefaultExportLine = true, true, line
	return nil
}

func validateNamespacedClassMetadata(value string, line int) error {
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
		"import":           "%% grepple:import <target> from <source>",
		"package":          "%% grepple:package <target> <exact-go-import-path>",
		"module":           "%% grepple:module <target> <module-path>",
		"file":             "%% grepple:file <target> <file-path>",
		"filelocal":        "%% grepple:filelocal <target>",
		"struct-tag":       "%% grepple:struct-tag <target> <field> <Go-quoted-string>",
		"underlying":       "%% grepple:underlying <target> <normalized-Go-type-expression>",
		"complete-package": "%% grepple:complete-package <exact-go-import-path>",
		"package-default":  "%% grepple:package-default <exact-go-import-path>",
		"language-default": "%% grepple:language-default <language>",
		"exact-default":    "%% grepple:exact-default",
		"default-export":   "%% grepple:default-export <target>",
		"generated":        "%% grepple:generated entry <symbol> depth <depth> max-nodes <limit>",
	}
	if syntax, known := expected[directive]; known {
		return fmt.Errorf("Line %d: malformed %s directive; expected '%s'", line, directive, syntax)
	}
	return fmt.Errorf("Line %d: unknown metadata directive '%s'", line, directive)
}

func (parser *classParser) applyCompletePackageMetadata(path string, line int) error {
	if parser.diagram.CompletePackage != "" {
		return fmt.Errorf("Line %d: duplicate complete-package directive", line)
	}
	parser.diagram.CompletePackage, parser.diagram.CompletePackageLine = path, line
	return nil
}

func (parser *classParser) applyPackageDefault(path string, line int) error {
	if !parser.sawHeader {
		return nil
	}
	if parser.diagram.PackageDefaultLine != 0 {
		return fmt.Errorf("Line %d: duplicate package-default directive", line)
	}
	parser.diagram.PackageDefault, parser.diagram.PackageDefaultLine = path, line
	return nil
}

func (parser *classParser) applyLanguageDefault(language string, line int) error {
	if !isSupportedLanguage(language) {
		return fmt.Errorf("Line %d: malformed language-default directive: unsupported language %q", line, language)
	}
	if !parser.sawHeader {
		return nil
	}
	if parser.diagram.LanguageDefaultLine != 0 {
		return fmt.Errorf("Line %d: duplicate language-default directive", line)
	}
	parser.diagram.LanguageDefault, parser.diagram.LanguageDefaultLine = language, line
	return nil
}

func (parser *classParser) applyExactDefault(line int) error {
	if !parser.sawHeader {
		return nil
	}
	if parser.diagram.ExactDefaultLine != 0 {
		return fmt.Errorf("Line %d: duplicate exact-default directive", line)
	}
	parser.diagram.ExactDefault, parser.diagram.ExactDefaultLine = true, line
	return nil
}

func (parser *classParser) applyStructTagMetadata(name, field, quoted string, line int) error {
	class, err := parser.metadataTarget(name, line)
	if err != nil {
		return err
	}
	if len(quoted) == 0 || quoted[0] == '\'' {
		return fmt.Errorf("Line %d: malformed struct-tag directive; expected '%%%% grepple:struct-tag <target> <field> <Go-quoted-string>'", line)
	}
	value, err := strconv.Unquote(quoted)
	if err != nil {
		return fmt.Errorf("Line %d: malformed struct-tag directive; expected '%%%% grepple:struct-tag <target> <field> <Go-quoted-string>'", line)
	}
	if class.StructTags == nil {
		class.StructTags = map[string]StructTagRequirement{}
	}
	if _, exists := class.StructTags[field]; exists {
		return fmt.Errorf("Line %d: duplicate struct-tag directive for '%s.%s'", line, name, field)
	}
	class.StructTags[field] = StructTagRequirement{Value: value, Line: line}
	return nil
}

func (parser *classParser) applyUnderlyingMetadata(name, expression string, line int) error {
	class, err := parser.metadataTarget(name, line)
	if err != nil {
		return err
	}
	if class.UnderlyingLine != 0 {
		return fmt.Errorf("Line %d: duplicate underlying directive for '%s'", line, name)
	}
	if !validGoTypeExpression(expression) {
		return fmt.Errorf("Line %d: malformed underlying directive; expected '%%%% grepple:underlying <target> <normalized-Go-type-expression>'", line)
	}
	normalized, ok := normalizeGoUnderlyingType(expression)
	if !ok || !validGoTypeExpression(normalized) {
		return fmt.Errorf("Line %d: malformed underlying directive; expected '%%%% grepple:underlying <target> <normalized-Go-type-expression>'", line)
	}
	class.Underlying, class.UnderlyingLine = normalized, line
	return nil
}

func (parser *classParser) applyClassScope(name, scope, language string, line int) error {
	class, err := parser.metadataTarget(name, line)
	if err != nil {
		return err
	}
	if language == "go" {
		if class.PackageLine != 0 {
			return fmt.Errorf("Line %d: duplicate package directive for '%s'", line, name)
		}
		if class.Module != "" {
			return stereotypeConflict(class, "go package", line)
		}
		class.Package, class.PackageLine, class.Language = scope, line, language
		return nil
	}
	if class.ModuleLine != 0 {
		return fmt.Errorf("Line %d: duplicate module directive for '%s'", line, name)
	}
	if class.Package != "" {
		return stereotypeConflict(class, "TypeScript module", line)
	}
	class.Module, class.ModuleLine, class.Language = scope, line, language
	return nil
}

func (parser *classParser) metadataTarget(name string, line int) (*DiagramClass, error) {
	class := parser.diagram.Classes[name]
	if class == nil {
		return nil, fmt.Errorf("Line %d: metadata target '%s' is not declared", line, name)
	}
	return class, nil
}

func (parser *classParser) parseMemberLine(value string, line int) error {
	if value == "}" {
		parser.current = nil
		return nil
	}
	if relationRE.MatchString(value) {
		return fmt.Errorf("Line %d: relations are not allowed inside class bodies", line)
	}
	if classDirectionRE.MatchString(value) || strings.HasPrefix(value, "direction ") {
		return fmt.Errorf("Line %d: direction is only allowed at diagram top level", line)
	}
	if namespaceRE.MatchString(value) || strings.HasPrefix(value, "namespace ") {
		return fmt.Errorf("Line %d: namespaces are not allowed inside class bodies", line)
	}
	if match := stereotypeRE.FindStringSubmatch(value); match != nil && match[2] == "" {
		return applyStereotype(parser.current, strings.TrimSpace(match[1]), line)
	}
	member, err := parseMember(value, line)
	if err == nil {
		parser.current.Members = append(parser.current.Members, member)
	}
	return err
}

func (parser *classParser) parseTopLevel(value string, line int) error {
	if strings.HasPrefix(value, "note") {
		return parser.parseNote(value, line)
	}
	if matched, err := parser.parseTopLevelLayout(value, line); matched {
		return err
	}
	if match := classRE.FindStringSubmatch(value); match != nil {
		return parser.addClass(match[1], match[2], strings.HasSuffix(value, "{"), line)
	}
	if match := stereotypeRE.FindStringSubmatch(value); match != nil && match[2] != "" {
		return parser.applyExternalStereotype(match[2], match[1], line)
	}
	if match := relationRE.FindStringSubmatch(value); match != nil {
		return parser.addRelation(match, line)
	}
	return fmt.Errorf("Line %d: unsupported class diagram syntax: %s", line, value)
}

func (parser *classParser) parseTopLevelLayout(value string, line int) (bool, error) {
	if value == "}" {
		if parser.namespace == "" {
			return true, fmt.Errorf("Line %d: unexpected '}'", line)
		}
		parser.namespace, parser.namespaceLine = "", 0
		return true, nil
	}
	if match := classDirectionRE.FindStringSubmatch(value); match != nil {
		if parser.namespace != "" {
			return true, fmt.Errorf("Line %d: direction is only allowed at diagram top level", line)
		}
		if parser.diagram.Direction != "" {
			return true, fmt.Errorf("Line %d: duplicate direction statement", line)
		}
		parser.diagram.Direction = match[1]
		return true, nil
	}
	if strings.HasPrefix(value, "direction ") || value == "direction" {
		return true, fmt.Errorf("Line %d: malformed direction; expected 'direction LR|RL|TB|BT'", line)
	}
	if match := namespaceRE.FindStringSubmatch(value); match != nil {
		if parser.namespace != "" {
			return true, fmt.Errorf("Line %d: nested namespaces are not supported", line)
		}
		parser.namespace, parser.namespaceLine = match[1], line
		return true, nil
	}
	if strings.HasPrefix(value, "namespace ") || value == "namespace" {
		return true, fmt.Errorf("Line %d: malformed namespace; expected 'namespace <identifier> {'", line)
	}
	return false, nil
}

func (parser *classParser) addRelation(match []string, line int) error {
	if parser.namespace != "" {
		return fmt.Errorf("Line %d: relations are not allowed inside namespaces", line)
	}
	parser.diagram.Relations = append(parser.diagram.Relations, Relation{
		Left: match[1], LeftMultiplicity: match[2], Operator: match[3],
		RightMultiplicity: match[4], Right: match[5], Label: strings.TrimSpace(match[6]), Line: line,
	})
	return nil
}

func (parser *classParser) parseNote(value string, line int) error {
	if parser.namespace != "" {
		return fmt.Errorf("Line %d: notes are only allowed at diagram top level", line)
	}
	remainder := strings.TrimSpace(strings.TrimPrefix(value, "note"))
	target := ""
	if strings.HasPrefix(remainder, "for") {
		afterFor := strings.TrimSpace(strings.TrimPrefix(remainder, "for"))
		split := strings.IndexAny(afterFor, " \t")
		if split <= 0 {
			return malformedNote(line)
		}
		target = afterFor[:split]
		if !renderableName.MatchString(target) {
			return malformedNote(line)
		}
		remainder = strings.TrimSpace(afterFor[split:])
		if parser.diagram.Classes[target] == nil {
			return fmt.Errorf("Line %d: note target '%s' is not declared", line, target)
		}
	}
	if len(remainder) < 2 || remainder[0] != '"' {
		return malformedNote(line)
	}
	text, err := strconv.Unquote(remainder)
	if err != nil || strings.ContainsAny(text, "\r\n") {
		return malformedNote(line)
	}
	parser.diagram.Notes = append(parser.diagram.Notes, DiagramNote{Text: text, Target: target, Line: line})
	return nil
}

func (parser *classParser) addClass(name, displayLabel string, hasBody bool, line int) error {
	if parser.diagram.Classes[name] != nil {
		return fmt.Errorf("Line %d: duplicate diagram node '%s'", line, name)
	}
	class := &DiagramClass{Name: name, DisplayLabel: html.UnescapeString(displayLabel), Kind: "class", Line: line, StructTags: map[string]StructTagRequirement{}}
	parser.diagram.Classes[name] = class
	parser.diagram.Order = append(parser.diagram.Order, name)
	if hasBody {
		parser.current = class
	}
	return nil
}

func (parser *classParser) applyExternalStereotype(name, stereotype string, line int) error {
	class := parser.diagram.Classes[name]
	if class == nil {
		return fmt.Errorf("Line %d: stereotype target '%s' is not declared", line, name)
	}
	return applyStereotype(class, strings.TrimSpace(stereotype), line)
}

func validateRelationMultiplicities(diagram *ClassDiagram) error {
	for _, relation := range diagram.Relations {
		if isHeritageOperator(relation.Operator) {
			continue
		}
		for _, multiplicity := range []string{relation.LeftMultiplicity, relation.RightMultiplicity} {
			if multiplicity != "" && multiplicity != "1" && multiplicity != "*" {
				return fmt.Errorf("Line %d: unsupported association multiplicity %q; use \"1\", \"*\", or omit it", relation.Line, multiplicity)
			}
		}
	}
	return nil
}

func validateRelationEndpoints(diagram *ClassDiagram) error {
	for _, relation := range diagram.Relations {
		for _, endpoint := range []string{relation.Left, relation.Right} {
			if diagram.Classes[endpoint] == nil {
				return fmt.Errorf("Line %d: relation endpoint '%s' is not declared", relation.Line, endpoint)
			}
		}
	}
	return nil
}
func applyClassDefaults(diagram *ClassDiagram) error {
	if diagram.PackageDefault != "" && (diagram.LanguageDefault == "typescript" || diagram.LanguageDefault == "javascript") {
		return fmt.Errorf("Line %d: package-default conflicts with language-default %q", diagram.LanguageDefaultLine, diagram.LanguageDefault)
	}
	for _, name := range diagram.Order {
		if err := applyDefaultsToClass(diagram, name); err != nil {
			return err
		}
	}
	return nil
}

func applyDefaultsToClass(diagram *ClassDiagram, name string) error {
	class := diagram.Classes[name]
	if err := applyPackageDefaultToClass(diagram, class, name); err != nil {
		return err
	}
	if err := applyLanguageDefaultToClass(diagram, class, name); err != nil {
		return err
	}
	if diagram.ExactDefault && !class.Function && (class.Kind == "struct" || class.Kind == "interface") {
		class.Exact = true
	}
	return nil
}

func applyPackageDefaultToClass(diagram *ClassDiagram, class *DiagramClass, name string) error {
	if diagram.PackageDefault == "" {
		return nil
	}
	if class.Module != "" {
		return fmt.Errorf("Line %d: module directive for '%s' conflicts with package-default", class.ModuleLine, name)
	}
	if class.Package != "" && class.Package != diagram.PackageDefault {
		return fmt.Errorf("Line %d: package directive for '%s' conflicts with package-default", class.PackageLine, name)
	}
	if class.Package == "" {
		class.Package = diagram.PackageDefault
	}
	return nil
}

func applyLanguageDefaultToClass(diagram *ClassDiagram, class *DiagramClass, name string) error {
	if diagram.LanguageDefault == "" {
		if diagram.PackageDefault != "" && class.Language == "" {
			class.Language = "go"
		}
		return nil
	}
	if class.Language != "" && class.Language != diagram.LanguageDefault {
		return fmt.Errorf("Line %d: explicit metadata for '%s' conflicts with language-default", class.Line, name)
	}
	if class.Language == "" {
		class.Language = diagram.LanguageDefault
	}
	return nil
}

func finalizeDiagramTypes(diagram *ClassDiagram) {
	for _, class := range diagram.Classes {
		language := class.Language
		if language == "" {
			language = "typescript"
		}
		for index := range class.Members {
			member := &class.Members[index]
			member.Type = normalizeTypeForLanguage(member.Type, language)
			for parameterIndex := range member.Parameters {
				member.Parameters[parameterIndex] = normalizeTypeForLanguage(member.Parameters[parameterIndex], language)
			}
		}
	}
}

func formatMember(member Member) string {
	static, async := "", ""
	if member.Static {
		static = "static "
	}
	if member.Async {
		async = "async "
	}
	if member.Kind == "property" {
		return fmt.Sprintf("%s %s%s: %s", member.Visibility, static, member.Name, member.Type)
	}
	memberType := member.Type
	if memberType == "" {
		memberType = "unspecified"
	}
	return fmt.Sprintf("%s %s%s%s(%s): %s", member.Visibility, static, async, member.Name, strings.Join(member.Parameters, ", "), memberType)
}

func signaturesMatch(expected, actual Member) bool {
	if expected.Type != "" && expected.Type != actual.Type {
		return false
	}
	if expected.Async != actual.Async || len(expected.Parameters) != len(actual.Parameters) {
		return false
	}
	for index := range expected.Parameters {
		if expected.Parameters[index] != actual.Parameters[index] {
			return false
		}
	}
	return true
}

func membersMatch(expected, actual Member) bool {
	return expected.Kind == actual.Kind && expected.Name == actual.Name && expected.Visibility == actual.Visibility && expected.Static == actual.Static && signaturesMatch(expected, actual)
}
func typeReferences(memberType, target string) bool {
	for offset := 0; offset <= len(memberType)-len(target); {
		index := strings.Index(memberType[offset:], target)
		if index < 0 {
			return false
		}
		index += offset
		if unqualifiedTypeBoundary(memberType, index, len(target)) {
			return true
		}
		offset = index + len(target)
	}
	return false
}

func unqualifiedTypeBoundary(value string, index, length int) bool {
	if index > 0 && (identifierTypeByte(value[index-1]) || value[index-1] == '.') {
		return false
	}
	end := index + length
	return end == len(value) || !identifierTypeByte(value[end])
}

func identifierTypeByte(value byte) bool {
	return value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z' || value >= '0' && value <= '9' || value == '_' || value == '$'
}

func collectionReferences(memberType, target string) bool {
	if strings.HasPrefix(memberType, "tuple<") && strings.HasSuffix(memberType, ">") {
		return tupleCollectionReferences(memberType, target)
	}
	if strings.HasSuffix(memberType, "[]") {
		return typeReferences(strings.TrimSuffix(memberType, "[]"), target)
	}
	if strings.HasPrefix(memberType, "[]") {
		return typeReferences(memberType[2:], target)
	}
	if referenced, matched := bracketCollectionReferences(memberType, target); matched {
		return referenced
	}
	if referenced, matched := genericCollectionReferences(memberType, target); matched {
		return referenced
	}
	return strings.Contains(memberType, "[") && strings.HasSuffix(memberType, "]") && typeReferences(memberType, target)
}

func tupleCollectionReferences(memberType, target string) bool {
	for _, component := range splitParameters(memberType[len("tuple<") : len(memberType)-1]) {
		if collectionReferences(component, target) {
			return true
		}
	}
	return false
}

func bracketCollectionReferences(memberType, target string) (bool, bool) {
	if !strings.HasPrefix(memberType, "[") && !strings.HasPrefix(memberType, "map[") {
		return false, false
	}
	closing := strings.Index(memberType, "]")
	if closing < 0 {
		return false, true
	}
	return typeReferences(memberType[closing+1:], target), true
}

func genericCollectionReferences(memberType, target string) (bool, bool) {
	for _, collection := range []string{"ReadonlyArray", "Set"} {
		prefix := collection + "<"
		if strings.HasPrefix(memberType, prefix) && strings.HasSuffix(memberType, ">") {
			return typeReferences(memberType[len(prefix):len(memberType)-1], target), true
		}
	}
	return false, false
}

type classValidator struct {
	diagram     *ClassDiagram
	analysis    *Analysis
	diagnostics []Diagnostic
}

func (validator *classValidator) add(line int, message string) {
	validator.diagnostics = append(validator.diagnostics, Diagnostic{Line: line, Message: message})
}

type packageCompletenessItem struct {
	name, kind string
	location   Location
}

func (validator *classValidator) checkCompletePackage() {
	importPath := validator.diagram.CompletePackage
	if importPath == "" {
		return
	}
	uncovered := validator.uncoveredPackageDeclarations(importPath)
	uncovered = append(uncovered, validator.uncoveredPackageFunctions(importPath)...)
	sort.Slice(uncovered, func(left, right int) bool {
		return packageCompletenessLess(uncovered[left], uncovered[right])
	})
	for _, item := range uncovered {
		validator.addPackageCompletenessDiagnostic(item)
	}
}

func (validator *classValidator) uncoveredPackageDeclarations(importPath string) []packageCompletenessItem {
	var uncovered []packageCompletenessItem
	for _, declaration := range validator.analysis.GoDeclarations {
		if validator.analysis.GoPackagePaths[declaration.PackageID] != importPath || validator.completePackageContains(declaration) {
			continue
		}
		uncovered = append(uncovered, packageCompletenessItem{name: declaration.Name, kind: declaration.Kind, location: declaration.Location})
	}
	return uncovered
}

func (validator *classValidator) uncoveredPackageFunctions(importPath string) []packageCompletenessItem {
	var uncovered []packageCompletenessItem
	for _, functions := range validator.analysis.GoFunctions {
		for _, function := range functions {
			if !exportedGoName(function.Name) || validator.analysis.GoPackagePaths[function.PackageID] != importPath || validator.completePackageContainsFunction(function) {
				continue
			}
			uncovered = append(uncovered, packageCompletenessItem{name: function.Name, kind: "function", location: function.Location})
		}
	}
	return uncovered
}

func packageCompletenessLess(left, right packageCompletenessItem) bool {
	leftPath := normalizeFileMetadata(left.location.Path)
	rightPath := normalizeFileMetadata(right.location.Path)
	if leftPath != rightPath {
		return leftPath < rightPath
	}
	if left.location.Line != right.location.Line {
		return left.location.Line < right.location.Line
	}
	if left.name != right.name {
		return left.name < right.name
	}
	return left.kind < right.kind
}

func (validator *classValidator) addPackageCompletenessDiagnostic(item packageCompletenessItem) {
	path := normalizeFileMetadata(item.location.Path)
	validator.diagnostics = append(validator.diagnostics, Diagnostic{
		Line:     validator.diagram.CompletePackageLine,
		Message:  fmt.Sprintf("Package completeness missing %s '%s' declared at %s:%d.", item.kind, item.name, path, item.location.Line),
		sortPath: path, sortLine: item.location.Line, sortName: item.name,
	})
}

func (validator *classValidator) completePackageContains(declaration *Declaration) bool {
	for _, name := range validator.diagram.Order {
		class := validator.diagram.Classes[name]
		if class.Function || class.Import || class.Package == "" || class.Name != declaration.Name {
			continue
		}
		actual := validator.declaration(class.Name)
		if actual != nil && actual.PackageID == declaration.PackageID {
			return true
		}
	}
	return false
}

func (validator *classValidator) completePackageContainsFunction(function Member) bool {
	for _, name := range validator.diagram.Order {
		class := validator.diagram.Classes[name]
		if !class.Function || class.Import || class.Package == "" || class.Name != function.Name {
			continue
		}
		for _, candidate := range validator.analysis.Functions[class.Name] {
			if candidate.PackageID == function.PackageID && validator.memberScopeMatches(class, candidate) {
				return true
			}
		}
	}
	return false
}

func (validator *classValidator) checkClass(class *DiagramClass) {
	if class.Import {
		validator.checkImport(class)
	} else if class.Function {
		validator.checkFunction(class)
	} else {
		validator.checkDeclaration(class)
	}
	validator.checkExport(class)
}

func (validator *classValidator) checkImport(class *DiagramClass) {
	matches := 0
	for _, candidate := range validator.analysis.Imports[class.Name] {
		if validator.importScopeMatches(class, candidate) && (class.File == "" || normalizeFileMetadata(candidate.File) == class.File) && (class.ImportSource == "" || class.ImportSource == candidate.Source) {
			matches++
		}
	}
	if matches == 1 {
		return
	}
	if matches > 1 {
		validator.add(class.Line, ambiguityMessage("import", class.Name))
		return
	}
	source := ""
	if class.ImportSource != "" {
		source = " from '" + class.ImportSource + "'"
	}
	validator.add(class.Line, fmt.Sprintf("Missing import '%s'%s.", class.Name, source))
}

func functionSignature(class *DiagramClass) (DiagramMember, bool) {
	for _, member := range class.Members {
		if member.Kind == "method" && member.Name == class.Name {
			return member, true
		}
	}
	return DiagramMember{}, false
}

func (validator *classValidator) checkFunction(class *DiagramClass) {
	if validator.functionAmbiguous(class) {
		validator.add(class.Line, ambiguityMessage("function", class.Name))
		return
	}
	signature, ok := functionSignature(class)
	if !ok {
		if validator.hasFunctionInScope(class) {
			return
		}
		validator.add(class.Line, fmt.Sprintf("Missing function '%s'.", class.Name))
		return
	}
	signature.Async = signature.Async || class.Async
	candidates := validator.analysis.Functions[class.Name]
	if len(candidates) == 0 {
		validator.add(class.Line, fmt.Sprintf("Missing %sfunction '%s'.", asyncMarker(signature.Async), class.Name))
		return
	}
	if validator.hasMatchingFunction(class, signature) {
		return
	}
	memberType := signature.Type
	if memberType == "" {
		memberType = "unspecified"
	}
	validator.add(signature.Line, fmt.Sprintf("Expected %sfunction %s(%s): %s.", asyncMarker(signature.Async), class.Name, strings.Join(signature.Parameters, ", "), memberType))
}

func (validator *classValidator) hasFunctionInScope(class *DiagramClass) bool {
	for _, candidate := range validator.analysis.Functions[class.Name] {
		if validator.memberScopeMatches(class, candidate) && (class.File == "" || normalizeFileMetadata(candidate.File) == class.File) {
			return true
		}
	}
	return false
}

func (validator *classValidator) hasMatchingFunction(class *DiagramClass, signature DiagramMember) bool {
	for _, candidate := range validator.analysis.Functions[class.Name] {
		if validator.memberScopeMatches(class, candidate) && (class.File == "" || normalizeFileMetadata(candidate.File) == class.File) && signaturesMatch(signature.Member, candidate) {
			return true
		}
	}
	return false
}

func asyncMarker(async bool) string {
	if async {
		return "async "
	}
	return ""
}

func (validator *classValidator) declaration(name string) *Declaration {
	class := validator.diagram.Classes[name]
	if class != nil && class.Package != "" {
		return validator.goDeclaration(name, class.Package)
	}
	if class != nil && class.Module != "" {
		moduleID := resolveTypeScriptScope(class.Module, validator.analysis)
		if moduleID == "" {
			return nil
		}
		return validator.analysis.TSDeclarations[moduleID+":"+name]
	}
	if class != nil && class.Language != "" {
		return validator.analysis.DeclarationVariants[class.Language+":"+name]
	}
	return validator.analysis.Declarations[name]
}

func (validator *classValidator) goDeclaration(name, scope string) *Declaration {
	var result *Declaration
	for _, declaration := range validator.analysis.GoDeclarations {
		if declaration.Name != name || !goScopeMatches(scope, declaration.Package, declaration.PackageID, validator.analysis) {
			continue
		}
		if result != nil {
			return nil
		}
		result = declaration
	}
	return result
}

func normalizeFileMetadata(value string) string {
	return filepath.ToSlash(filepath.Clean(filepath.FromSlash(value)))
}

func (validator *classValidator) checkDeclaration(class *DiagramClass) {
	if validator.declarationAmbiguous(class) {
		validator.add(class.Line, ambiguityMessage("declaration", class.Name))
		return
	}
	actual := validator.declaration(class.Name)
	if actual == nil {
		validator.add(class.Line, fmt.Sprintf("Missing %s '%s'.", class.Kind, class.Name))
		return
	}
	if class.File != "" && normalizeFileMetadata(actual.File) != class.File {
		validator.add(class.FileLine, fmt.Sprintf("Expected '%s' in file '%s'; found '%s'.", class.Name, class.File, actual.File))
		return
	}
	if actual.Kind != class.Kind {
		validator.add(class.Line, kindMismatch(class, actual))
		return
	}
	if !languageMatches(class.Language, actual.Language) {
		validator.add(class.Line, fmt.Sprintf("Expected '%s' to be %s; found %s.", class.Name, class.Language, actual.Language))
		return
	}
	if class.Underlying != "" {
		validator.checkUnderlying(class, actual)
	}
	if len(class.StructTags) > 0 {
		validator.checkStructTags(class, actual)
	}
	if class.FileLocal {
		validator.checkGoFileLocal(class, actual)
	}
	for _, expected := range class.Members {
		members := actual.Members
		if class.Exact && actual.Language == "go" && class.File != "" && validator.diagram.CompletePackage == "" {
			members = membersInFile(members, class.File)
		}
		validator.checkMember(class.Name, expected, members)
	}
	if class.Exact {
		// File metadata constrains where declared members must be implemented, but
		// exactness still covers the type's package-wide method set. Otherwise an
		// additional method in another file would silently escape the schema.
		validator.checkUnexpectedMembers(class, actual.Members)
	}
}

func (validator *classValidator) checkUnderlying(class *DiagramClass, declaration *Declaration) {
	if declaration.Language != "go" || class.Kind != "alias" && class.Kind != "type" {
		validator.add(class.UnderlyingLine, fmt.Sprintf("Underlying metadata on '%s' requires a Go <<type>> or <<alias>> node.", class.Name))
		return
	}
	if class.Underlying != declaration.Underlying {
		validator.add(class.UnderlyingLine, fmt.Sprintf("Expected underlying type of '%s' to be '%s'; found '%s'.", class.Name, class.Underlying, declaration.Underlying))
	}
}

func (validator *classValidator) checkStructTags(class *DiagramClass, declaration *Declaration) {
	for _, field := range sortedKeys(class.StructTags) {
		requirement := class.StructTags[field]
		if declaration.Language != "go" || declaration.Kind != "struct" {
			validator.add(requirement.Line, fmt.Sprintf("Struct-tag metadata on '%s.%s' requires a Go struct.", class.Name, field))
			continue
		}
		actual, exists := declaration.StructTags[field]
		if !exists {
			validator.add(requirement.Line, fmt.Sprintf("Struct-tag field '%s.%s' is not a named field.", class.Name, field))
			continue
		}
		if !actual.Present {
			validator.add(requirement.Line, fmt.Sprintf("Expected Go struct tag %s on '%s.%s'; found no struct tag.", strconv.Quote(requirement.Value), class.Name, field))
			continue
		}
		if actual.Value != requirement.Value {
			validator.add(requirement.Line, fmt.Sprintf("Expected Go struct tag %s on '%s.%s'; found %s.", strconv.Quote(requirement.Value), class.Name, field, strconv.Quote(actual.Value)))
		}
	}
}

func (validator *classValidator) checkGoFileLocal(class *DiagramClass, declaration *Declaration) {
	if declaration.Language != "go" {
		validator.add(class.FileLocalLine, fmt.Sprintf("File-local metadata on '%s' requires a Go type.", class.Name))
		return
	}
	if !declaration.FileLocal {
		validator.add(class.FileLocalLine, fmt.Sprintf("Go type '%s' requires contiguous leading marker comment '//grepple:filelocal'.", class.Name))
	}
	location, found := validator.firstExternalGoTypeReference(declaration)
	if found {
		validator.add(class.FileLocalLine, fmt.Sprintf("File-local Go type '%s' is referenced outside declaration file at '%s:%d:%d'.", class.Name, location.Path, location.Line, location.Column))
	}
}

func (validator *classValidator) firstExternalGoTypeReference(declaration *Declaration) (Location, bool) {
	references := validator.analysis.GoTypeReferences[declaration.PackageID+":"+declaration.Name]
	var first Location
	found := false
	for _, location := range references {
		if normalizeFileMetadata(location.Path) == normalizeFileMetadata(declaration.File) {
			continue
		}
		if !found || location.Path < first.Path || location.Path == first.Path && (location.Line < first.Line || location.Line == first.Line && location.Column < first.Column) {
			first, found = location, true
		}
	}
	return first, found
}

func (validator *classValidator) checkUnexpectedMembers(class *DiagramClass, actual []Member) {
	for _, member := range actual {
		if !diagramContainsMember(class.Members, member) {
			validator.add(class.Line, fmt.Sprintf("Unexpected %s on exact '%s'.", formatMember(member), class.Name))
		}
	}
}

func membersInFile(members []Member, file string) []Member {
	result := make([]Member, 0, len(members))
	for _, member := range members {
		if normalizeFileMetadata(member.File) == file {
			result = append(result, member)
		}
	}
	return result
}

func diagramContainsMember(expected []DiagramMember, actual Member) bool {
	for _, member := range expected {
		if membersMatch(member.Member, actual) {
			return true
		}
	}
	return false
}

func languageMatches(expected, actual string) bool { return expected == "" || expected == actual }

func (validator *classValidator) memberScopeMatches(class *DiagramClass, member Member) bool {
	if !languageMatches(class.Language, member.Language) {
		return false
	}
	if member.Language == "go" {
		return goScopeMatches(class.Package, member.Package, member.PackageID, validator.analysis)
	}
	return class.Module == "" || resolveTypeScriptScope(class.Module, validator.analysis) == member.ModuleID
}

func (validator *classValidator) importScopeMatches(class *DiagramClass, item Import) bool {
	if !languageMatches(class.Language, item.Language) {
		return false
	}
	if item.Language == "go" {
		return goScopeMatches(class.Package, item.Package, item.PackageID, validator.analysis)
	}
	return class.Module == "" || resolveTypeScriptScope(class.Module, validator.analysis) == item.ImporterModuleID
}

func ambiguityMessage(kind, name string) string {
	return fmt.Sprintf("Ambiguous code %s '%s' exists in multiple language or Go package scopes; narrow source roots or add language metadata.", kind, name)
}

func (validator *classValidator) declarationAmbiguous(class *DiagramClass) bool {
	if class.Module != "" && typeScriptScopeAmbiguous(class.Module, validator.analysis) {
		return true
	}
	goCount := validator.goDeclarationCount(class)
	ecmaCount := validator.ecmaScriptDeclarationCount(class)
	if class.Language == "go" {
		return goCount > 1
	}
	if class.Language == "typescript" || class.Language == "javascript" {
		return ecmaCount > 1
	}
	return goCount > 1 || ecmaCount > 1 || goCount == 1 && ecmaCount == 1
}
func (validator *classValidator) goDeclarationCount(class *DiagramClass) int {
	count := 0
	for _, declaration := range validator.analysis.GoDeclarations {
		if declaration.Name == class.Name && goScopeMatches(class.Package, declaration.Package, declaration.PackageID, validator.analysis) {
			count++
		}
	}
	return count
}

func (validator *classValidator) ecmaScriptDeclarationCount(class *DiagramClass) int {
	count := 0
	moduleID := resolveTypeScriptScope(class.Module, validator.analysis)
	for _, declaration := range validator.analysis.TSDeclarations {
		if declaration.Name == class.Name && (class.Language == "" || declaration.Language == class.Language) && (class.Module == "" || moduleID == declaration.ModuleID) {
			count++
		}
	}
	return count
}

func (validator *classValidator) functionAmbiguous(class *DiagramClass) bool {
	identities := map[string]bool{}
	for _, member := range validator.analysis.Functions[class.Name] {
		if validator.memberScopeMatches(class, member) {
			identities[memberIdentity(member)] = true
		}
	}
	return len(identities) > 1
}

func memberIdentity(member Member) string {
	if member.Language == "go" {
		return "go:" + member.PackageID
	}
	return member.Language + ":" + member.ModuleID
}

func memberLanguageCount(members []Member) int {
	languages := map[string]bool{}
	for _, member := range members {
		languages[member.Language] = true
	}
	return len(languages)
}

func kindMismatch(expected *DiagramClass, actual *Declaration) string {
	article := "a"
	if expected.Kind == "interface" || expected.Kind == "alias" {
		article = "an"
	}
	return fmt.Sprintf("Expected '%s' to be %s %s; found %s.", expected.Name, article, expected.Kind, actual.Kind)
}

func (validator *classValidator) checkMember(owner string, expected DiagramMember, actual []Member) {
	var candidates []Member
	for _, member := range actual {
		if member.Name != expected.Name {
			continue
		}
		candidates = append(candidates, member)
		if membersMatch(expected.Member, member) {
			return
		}
	}
	found := "not found"
	if len(candidates) > 0 {
		values := make([]string, 0, len(candidates))
		for _, candidate := range candidates {
			values = append(values, formatMember(candidate))
		}
		found = strings.Join(values, "; ")
	}
	validator.add(expected.Line, fmt.Sprintf("Expected %s on '%s'; found %s.", formatMember(expected.Member), owner, found))
}

func (validator *classValidator) checkExport(class *DiagramClass) {
	if !class.Exported {
		return
	}
	if class.Module != "" {
		moduleID := resolveTypeScriptScope(class.Module, validator.analysis)
		found, kind := validator.analysis.TSExports[moduleID][class.Name], "export"
		if class.DefaultExport {
			found, kind = validator.analysis.TSDefaultExports[moduleID] == class.Name, "default export"
		}
		if !found {
			validator.add(class.Line, fmt.Sprintf("Missing %s for '%s'.", kind, class.Name))
		}
		return
	}
	exports, kind := validator.analysis.Exports, "export"
	if class.Language != "" {
		exports = validator.analysis.ExportVariants
	}
	key := class.Name
	if class.Language != "" {
		key = class.Language + ":" + class.Name
	}
	if class.DefaultExport {
		exports, kind = validator.analysis.DefaultExports, "default export"
		if class.Language != "" {
			exports = validator.analysis.DefaultExportVariants
		}
	}
	if !exports[key] {
		validator.add(class.Line, fmt.Sprintf("Missing %s for '%s'.", kind, class.Name))
	}
}

func (validator *classValidator) checkRelation(relation Relation) {
	if isHeritageOperator(relation.Operator) {
		validator.checkHeritage(relation)
		return
	}
	validator.checkAssociation(relation)
}

func isHeritageOperator(operator string) bool {
	return operator == "<|--" || operator == "--|>" || operator == "<|.." || operator == "..|>"
}

func (validator *classValidator) checkHeritage(relation Relation) {
	reverse := relation.Operator == "<|--" || relation.Operator == "<|.."
	child, parent := relation.Left, relation.Right
	if reverse {
		child, parent = relation.Right, relation.Left
	}
	declaration := validator.declaration(child)
	if declaration == nil {
		return
	}
	implements := relation.Operator == "<|.." || relation.Operator == "..|>"
	parents, verb := declaration.Extends, "extend"
	if implements {
		parents, verb = declaration.Implements, "implement"
	}
	parentDeclaration := validator.declaration(parent)
	resolvedParent := typeScriptReferenceName(declaration, parentDeclaration, validator.analysis)
	if !parents[parent] && (resolvedParent == "" || !parents[resolvedParent]) && !validator.structurallyImplements(declaration, parent, implements) {
		validator.add(relation.Line, fmt.Sprintf("'%s' must %s '%s'.", child, verb, parent))
	}
}

func (validator *classValidator) structurallyImplements(declaration *Declaration, parent string, implements bool) bool {
	contract := validator.declaration(parent)
	if !implements || declaration.Language != "go" || contract == nil || contract.Language != "go" || contract.Kind != "interface" {
		return false
	}
	for _, required := range contract.Members {
		matched := false
		for _, actual := range declaration.Members {
			if structuralMembersMatch(required, actual) {
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

func associationSides(relation Relation) (string, string, string) {
	if relation.Operator == "<--" || relation.Operator == "<.." || relation.Operator == "--*" || relation.Operator == "--o" {
		return relation.Right, relation.Left, relation.LeftMultiplicity
	}
	return relation.Left, relation.Right, relation.RightMultiplicity
}

func referenceMatchesMultiplicity(memberType, target, multiplicity string) bool {
	if !typeReferences(memberType, target) {
		return false
	}
	collection := collectionReferences(memberType, target)
	switch multiplicity {
	case "*":
		return collection
	case "1":
		return !collection
	default:
		return true
	}
}

func declarationReferences(declaration *Declaration, target, multiplicity string) bool {
	if (declaration.Kind == "alias" || declaration.Kind == "type") && referenceMatchesMultiplicity(declaration.Underlying, target, multiplicity) {
		return true
	}
	for _, member := range declaration.Members {
		if referenceMatchesMultiplicity(member.Type, target, multiplicity) {
			return true
		}
		for _, parameter := range member.Parameters {
			if referenceMatchesMultiplicity(parameter, target, multiplicity) {
				return true
			}
		}
	}
	return false
}

func packageMultiplicityMatches(value, target, multiplicity string) bool {
	cardinality, found := packageGoTypeCardinality(value, target)
	return found && (multiplicity == "" || multiplicity == "*" && cardinality == "many" || multiplicity == "1" && cardinality == "one")
}

func packageDeclarationReferences(declaration *Declaration, target, multiplicity string) bool {
	if (declaration.Kind == "alias" || declaration.Kind == "type") && packageMultiplicityMatches(declaration.Underlying, target, multiplicity) {
		return true
	}
	for _, member := range declaration.Members {
		if packageMemberReferences(member, target, multiplicity) {
			return true
		}
	}
	return false
}

func packageMemberReferences(member Member, target, multiplicity string) bool {
	types := append([]string{member.Type}, member.Parameters...)
	for _, value := range types {
		if packageMultiplicityMatches(value, target, multiplicity) {
			return true
		}
	}
	return false
}

func memberReferences(member Member, target, multiplicity string) bool {
	types := append([]string{member.Type}, member.Parameters...)
	for _, memberType := range types {
		if referenceMatchesMultiplicity(memberType, target, multiplicity) {
			return true
		}
	}
	return false
}
func (validator *classValidator) memberReferences(member Member, target, multiplicity string) bool {
	if member.Language == "go" {
		return packageMemberReferences(member, target, multiplicity)
	}
	return memberReferences(member, target, multiplicity)
}

func (validator *classValidator) functionReferences(class *DiagramClass, target string, targetDeclaration *Declaration, multiplicity string) bool {
	signature, ok := functionSignature(class)
	if !ok {
		return validator.anyFunctionCandidateReferences(class, target, multiplicity)
	}
	signature.Async = signature.Async || class.Async
	for _, candidate := range validator.analysis.Functions[class.Name] {
		if validator.matchingFunctionReferences(class, signature, candidate, target, targetDeclaration, multiplicity) {
			return true
		}
	}
	return false
}

func (validator *classValidator) anyFunctionCandidateReferences(class *DiagramClass, target, multiplicity string) bool {
	for _, candidate := range validator.analysis.Functions[class.Name] {
		if validator.memberScopeMatches(class, candidate) && validator.memberReferences(candidate, target, multiplicity) {
			return true
		}
	}
	return false
}

func (validator *classValidator) matchingFunctionReferences(class *DiagramClass, signature DiagramMember, candidate Member, target string, targetDeclaration *Declaration, multiplicity string) bool {
	if !validator.memberScopeMatches(class, candidate) || class.File != "" && normalizeFileMetadata(candidate.File) != class.File || !signaturesMatch(signature.Member, candidate) {
		return false
	}
	if validator.memberReferences(candidate, target, multiplicity) {
		return true
	}
	ownerDeclaration := &Declaration{Language: candidate.Language, ModuleID: candidate.ModuleID}
	reference := typeScriptReferenceName(ownerDeclaration, targetDeclaration, validator.analysis)
	return reference != "" && memberReferences(candidate, reference, multiplicity)
}

func (validator *classValidator) checkAssociation(relation Relation) {
	owner, target, multiplicity := associationSides(relation)
	if validator.associationExists(owner, target, multiplicity) {
		return
	}
	cardinality := associationCardinalityDescription(multiplicity)
	validator.add(relation.Line, fmt.Sprintf("'%s' must contain %s '%s' for this relationship.", owner, cardinality, target))
}

func (validator *classValidator) associationExists(owner, target, multiplicity string) bool {
	ownerClass := validator.diagram.Classes[owner]
	targetDeclaration := validator.declaration(target)
	if ownerClass.Function {
		return validator.functionReferences(ownerClass, target, targetDeclaration, multiplicity)
	}
	declaration := validator.declaration(owner)
	if declaration == nil {
		return true
	}
	if declaration.Language == "go" {
		return packageDeclarationReferences(declaration, target, multiplicity)
	}
	reference := typeScriptReferenceName(declaration, targetDeclaration, validator.analysis)
	return declarationReferences(declaration, target, multiplicity) || reference != "" && declarationReferences(declaration, reference, multiplicity)
}

func associationCardinalityDescription(multiplicity string) string {
	if multiplicity == "*" {
		return "a collection of"
	}
	if multiplicity == "1" {
		return "a non-collection reference to"
	}
	return "a reference to"
}

// CheckClassDiagram validates a Mermaid class schema against supported source languages.
func CheckClassDiagram(diagram string, sources []Source) ([]Diagnostic, error) {
	analysis, err := Analyze(sources)
	if err != nil {
		return nil, err
	}
	return CheckClassDiagramWithAnalysis(diagram, analysis)
}

// CheckClassDiagramWithAnalysis validates a class schema using a shared source analysis.
func CheckClassDiagramWithAnalysis(diagram string, analysis *Analysis) ([]Diagnostic, error) {
	return checkClassDiagramWithAnalysis(diagram, analysis)
}

func checkClassDiagramWithAnalysis(diagram string, analysis *Analysis) ([]Diagnostic, error) {
	parsed, err := ParseClassDiagram(diagram)
	if err != nil {
		return nil, err
	}
	validator := classValidator{diagram: parsed, analysis: analysis}
	for _, name := range parsed.Order {
		validator.checkClass(parsed.Classes[name])
	}
	for _, relation := range parsed.Relations {
		validator.checkRelation(relation)
	}
	validator.checkCompletePackage()
	sortDiagnostics(validator.diagnostics)
	return validator.diagnostics, nil
}
