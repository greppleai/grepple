package parser

//go:generate go run ./internal/generate

// NavigationFactCapabilities identifies normalized source fact kinds emitted by
// one language adapter. False means the adapter has no contract for that fact;
// ambiguous or unresolved emitted facts remain supported facts.
type NavigationFactCapabilities struct {
	Declarations   bool
	Calls          bool
	Imports        bool
	TypeReferences bool
	Fields         bool
	MemberAccess   bool
	Entrypoints    bool
}

// LanguageCapabilities describes one Tree-sitter-backed application language.
// Returned extension slices are copies and safe for callers to modify.
type LanguageCapabilities struct {
	ID                   string
	Extensions           []string
	Navigation           bool
	ImportNavigation     bool
	EntrypointNavigation bool
	NavigationFacts      NavigationFactCapabilities
	GrammarABI           uint32
	GrammarFingerprint   string
}

// ContentLanguageCapabilities describes parser-owned search and outline support.
// Specialized is true for lightweight non-Tree-sitter implementations.
type ContentLanguageCapabilities struct {
	ID             string
	Extensions     []string
	StructuralGrep bool
	Outline        bool
	Navigation     bool
	Specialized    bool
	// CodeFeaturesNotApplicable marks code-only features as not applicable.
	CodeFeaturesNotApplicable bool
}

// GrammarCardinality describes whether one grammar position accepts one or
// multiple syntax nodes. Unknown means the parent or position is not declared
// in the pinned grammar's node-types metadata.
type GrammarCardinality uint8

// GrammarCardinalityUnknown, GrammarCardinalityOne, and GrammarCardinalityMany
// identify absent, scalar, and repeated grammar positions.
const (
	GrammarCardinalityUnknown GrammarCardinality = iota
	GrammarCardinalityOne
	GrammarCardinalityMany
)

type languageGeneratedMetadata struct {
	fingerprint string
	fields      map[string]map[string]GrammarCardinality
	children    map[string]GrammarCardinality
	subtypes    map[string]map[string]bool
}

type stringSet map[string]struct{}

func newStringSet(values ...string) stringSet {
	set := make(stringSet, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}

func (set stringSet) contains(value string) bool {
	_, ok := set[value]
	return ok
}

type structureRules struct {
	structuralTypes         stringSet
	contextTypes            stringSet
	containerTypes          stringSet
	classDeclarationTypes   stringSet
	classBodyTypes          stringSet
	exportTypes             stringSet
	functionLikeTypes       stringSet
	functionExpressionTypes stringSet
	nameFieldCandidates     stringSet
}

func extractNodeName(node *syntaxNode, _ string, config *structureRules) string {
	if named := node.ChildByFieldName("name"); named != nil {
		return named.Text()
	}
	for _, child := range node.Children() {
		if config.nameFieldCandidates.contains(child.Kind()) {
			return child.Text()
		}
	}
	return ""
}

// segmentBuilder lets an adapter own its segment construction instead of using
// the shared AST segment engine.
type segmentBuilder interface {
	BuildSegments(root *syntaxNode, content string, hits map[int]bool) []Segment
}

type languageAdapter interface {
	ID() string
	Grammar() syntaxLanguage
	Parse(string) (*syntaxTree, error)
	Rules() *structureRules
	Navigation() navigationAdapter
	Outline(root *syntaxNode, content string) []Symbol
}

var languageAdapters = buildLanguageAdapters(
	newGoLanguage(),
	newJavaLanguage(),
	newKotlinLanguage(),
	newDartLanguage(),
	newSwiftLanguage(),
	newJavaScriptLanguage(),
	newTypeScriptLanguage("typescript", false),
	newTypeScriptLanguage("tsx", true),
	newPythonLanguage(),
	newCSharpLanguage(),
	newCLanguage(),
	newCPPLanguage(),
	newRustLanguage(),
	newPHPLanguage(),
	newShellLanguage(),
)

// contentAdapters hold parser-backed formats that are not code-navigation
// languages. They are reachable through adapterForLanguage but are excluded
// from the application-language registries.
var contentAdapters = buildLanguageAdapters(
	newMarkdownLanguage(),
)

var navigationAdapters = buildNavigationAdapters(languageAdapters)

var languageCapabilities = []LanguageCapabilities{
	{ID: "go", Extensions: []string{".go"}, Navigation: true},
	{ID: "java", Extensions: []string{".java"}, Navigation: true},
	{ID: "kotlin", Extensions: []string{".kt", ".kts"}, Navigation: true},
	{ID: "dart", Extensions: []string{".dart"}, Navigation: true},
	{ID: "swift", Extensions: []string{".swift"}, Navigation: true},
	{ID: "javascript", Extensions: []string{".js", ".jsx"}, Navigation: true},
	{ID: "typescript", Extensions: []string{".ts", ".mts", ".cts"}, Navigation: true},
	{ID: "tsx", Extensions: []string{".tsx"}, Navigation: true},
	{ID: "python", Extensions: []string{".py", ".pyi", ".pyw"}, Navigation: true},
	{ID: "csharp", Extensions: []string{".cs"}, Navigation: true},
	{ID: "c", Extensions: []string{".c", ".h"}, Navigation: true},
	{ID: "cpp", Extensions: []string{".cc", ".cpp", ".cxx", ".hpp", ".hh", ".hxx"}, Navigation: true},
	{ID: "rust", Extensions: []string{".rs"}, Navigation: true},
	{ID: "php", Extensions: []string{".php"}, Navigation: true},
	{ID: "shell", Extensions: []string{".sh", ".bash", ".zsh"}, Navigation: true},
}

// supportedLanguages returns deterministic metadata for parser-backed languages.
func supportedLanguages() []LanguageCapabilities {
	result := make([]LanguageCapabilities, len(languageCapabilities))
	for i, capability := range languageCapabilities {
		result[i] = enrichLanguageCapabilities(capability)
	}
	return result
}

// supportedContentLanguages returns deterministic parser and lightweight content capabilities.
func supportedContentLanguages() []ContentLanguageCapabilities {
	languages := make([]ContentLanguageCapabilities, 0, len(languageCapabilities)+4)
	for _, capability := range supportedLanguages() {
		languages = append(languages, ContentLanguageCapabilities{
			ID: capability.ID, Extensions: capability.Extensions, StructuralGrep: true, Outline: true, Navigation: capability.Navigation,
		})
	}
	languages = append(languages,
		ContentLanguageCapabilities{ID: "markdown", Extensions: []string{".md", ".markdown", ".mdown", ".mkd"}, StructuralGrep: true, Outline: true, Specialized: true, CodeFeaturesNotApplicable: true},
		ContentLanguageCapabilities{ID: "json", Extensions: []string{".json"}, Outline: true, Specialized: true},
		ContentLanguageCapabilities{ID: "yaml", Extensions: []string{".yaml", ".yml"}, Outline: true, Specialized: true},
		ContentLanguageCapabilities{ID: "text"},
	)
	return languages
}

// capabilitiesForLanguage returns metadata for a canonical language ID.
func capabilitiesForLanguage(id string) (LanguageCapabilities, bool) {
	for _, capability := range languageCapabilities {
		if capability.ID == id {
			return enrichLanguageCapabilities(capability), true
		}
	}
	return LanguageCapabilities{}, false
}

func enrichLanguageCapabilities(capability LanguageCapabilities) LanguageCapabilities {
	capability.Extensions = append([]string(nil), capability.Extensions...)
	if generated, ok := generatedLanguageMetadata[capability.ID]; ok {
		capability.GrammarFingerprint = generated.fingerprint
	}
	if adapter := adapterForLanguage(capability.ID); adapter != nil {
		if adapter.Grammar().valid() {
			capability.GrammarABI = adapter.Grammar().abiVersion()
		}
		if navigation := adapter.Navigation(); navigation != nil {
			capability.NavigationFacts = navigation.FactCapabilities()
			capability.ImportNavigation = capability.NavigationFacts.Imports
			capability.EntrypointNavigation = capability.NavigationFacts.Entrypoints
		}
	}
	return capability
}

func buildLanguageAdapters(adapters ...languageAdapter) map[string]languageAdapter {
	registry := make(map[string]languageAdapter, len(adapters))
	for _, adapter := range adapters {
		registry[adapter.ID()] = adapter
	}
	return registry
}

func buildNavigationAdapters(adapters map[string]languageAdapter) map[string]navigationAdapter {
	registry := make(map[string]navigationAdapter, len(adapters))
	for id, adapter := range adapters {
		registry[id] = adapter.Navigation()
	}
	return registry
}

func navigationAdapterForLanguage(id string) navigationAdapter {
	return navigationAdapters[id]
}

func adapterForLanguage(id string) languageAdapter {
	if adapter, ok := languageAdapters[id]; ok {
		return adapter
	}
	return contentAdapters[id]
}

func descendantName(node *syntaxNode, content string, candidates stringSet) string {
	if candidates.contains(node.Kind()) {
		return node.Text()
	}
	for _, child := range node.NamedChildren() {
		if name := descendantName(child, content, candidates); name != "" {
			return name
		}
	}
	return ""
}
