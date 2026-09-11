package parser

//go:generate go run ./internal/generate
import sitter "github.com/tree-sitter/go-tree-sitter"

// LanguageCapabilities describes one Tree-sitter-backed application language.
// Returned extension slices are copies and safe for callers to modify.
type LanguageCapabilities struct {
	ID                 string
	Extensions         []string
	Navigation         bool
	GrammarABI         uint32
	GrammarFingerprint string
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
	structuralTypes       stringSet
	contextTypes          stringSet
	containerTypes        stringSet
	classDeclarationTypes stringSet
	classBodyTypes        stringSet
	exportTypes           stringSet
	functionLikeTypes     stringSet
	blockTypes            stringSet
	jsxElementTypes       stringSet
	nameFieldCandidates   stringSet
}

type languageAdapter interface {
	ID() string
	Grammar() *sitter.Language
	Rules() *structureRules
	Outline(root *sitter.Node, content string) []Symbol
}

var languageAdapters = buildLanguageAdapters(
	newGoLanguage(),
	newJavaLanguage(),
	newKotlinLanguage(),
	newJavaScriptLanguage(),
	newTypeScriptLanguage("typescript", false),
	newTypeScriptLanguage("tsx", true),
	newPythonLanguage(),
	newCSharpLanguage(),
	newCLanguage(),
	newCPPLanguage(),
	newRustLanguage(),
	newShellLanguage(),
)

var languageCapabilities = []LanguageCapabilities{
	{ID: "go", Extensions: []string{".go"}, Navigation: true},
	{ID: "java", Extensions: []string{".java"}, Navigation: true},
	{ID: "kotlin", Extensions: []string{".kt", ".kts"}, Navigation: true},
	{ID: "javascript", Extensions: []string{".js", ".jsx"}, Navigation: true},
	{ID: "typescript", Extensions: []string{".ts", ".mts", ".cts"}, Navigation: true},
	{ID: "tsx", Extensions: []string{".tsx"}, Navigation: true},
	{ID: "python", Extensions: []string{".py", ".pyi", ".pyw"}, Navigation: true},
	{ID: "csharp", Extensions: []string{".cs"}, Navigation: true},
	{ID: "c", Extensions: []string{".c", ".h"}, Navigation: true},
	{ID: "cpp", Extensions: []string{".cc", ".cpp", ".cxx", ".hpp", ".hh", ".hxx"}, Navigation: true},
	{ID: "rust", Extensions: []string{".rs"}, Navigation: true},
	{ID: "shell", Extensions: []string{".sh", ".bash", ".zsh"}, Navigation: true},
}

// SupportedLanguages returns deterministic metadata for parser-backed languages.
func SupportedLanguages() []LanguageCapabilities {
	result := make([]LanguageCapabilities, len(languageCapabilities))
	for i, capability := range languageCapabilities {
		result[i] = enrichLanguageCapabilities(capability)
	}
	return result
}

// CapabilitiesForLanguage returns metadata for a canonical language ID.
func CapabilitiesForLanguage(id string) (LanguageCapabilities, bool) {
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
	if adapter := adapterForLanguage(capability.ID); adapter != nil && adapter.Grammar() != nil {
		capability.GrammarABI = adapter.Grammar().AbiVersion()
	}
	return capability
}

// GrammarFieldCardinality returns the cardinality of a named field on a node kind.
func GrammarFieldCardinality(language, parentKind, field string) GrammarCardinality {
	metadata, ok := generatedLanguageMetadata[language]
	if !ok {
		return GrammarCardinalityUnknown
	}
	return metadata.fields[parentKind][field]
}

// GrammarChildrenCardinality returns the cardinality of an unfielded children position.
func GrammarChildrenCardinality(language, parentKind string) GrammarCardinality {
	metadata, ok := generatedLanguageMetadata[language]
	if !ok {
		return GrammarCardinalityUnknown
	}
	return metadata.children[parentKind]
}

// GrammarSubtype reports whether kind belongs transitively to the named grammar
// supertype in the pinned node-types metadata.
func GrammarSubtype(language, supertype, kind string) bool {
	metadata, ok := generatedLanguageMetadata[language]
	return ok && metadata.subtypes[supertype][kind]
}

func buildLanguageAdapters(adapters ...languageAdapter) map[string]languageAdapter {
	registry := make(map[string]languageAdapter, len(adapters))
	for _, adapter := range adapters {
		registry[adapter.ID()] = adapter
	}
	return registry
}

func adapterForLanguage(id string) languageAdapter {
	return languageAdapters[id]
}

func descendantName(node *sitter.Node, content string, candidates stringSet) string {
	if candidates.contains(node.Kind()) {
		return nodeText(node, content)
	}
	for _, child := range namedChildren(node) {
		if name := descendantName(child, content, candidates); name != "" {
			return name
		}
	}
	return ""
}
