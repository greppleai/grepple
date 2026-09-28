package extract

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	codeparser "github.com/greppleai/grepple/parser"
)

// Language describes source-language features supported by extraction.
type Language struct {
	ID               string
	Extensions       []string
	FocusedStructure bool
	FocusedFlow      bool
}

type languageAnalysis interface {
	Analyze(Source) error
	Finalize() error
}

// languageDefinition is the single integration surface for a built-in language.
// Adding a language requires one definition plus its analyzer and renderers; discovery,
// analysis orchestration, source scoping, normalization, and validation dispatch stay unchanged.
type languageDefinition struct {
	info               Language
	acceptsSource      func(string) bool
	acceptsInput       func(string, bool) (bool, error)
	newAnalysis        func(*Analysis, []Source) languageAnalysis
	nearestProjectRoot func(string) string
	sourceScope        func(Source) (string, error)
	normalizeType      func(string) string
	generateStructure  func(string, Source, []Source, GenerateOptions) (string, error)
	generateFlow       func(string, Source, []Source, int, int) (string, error)
	validFlowEdge      func(*Analysis, *Symbol, *Symbol) bool
	flowIndex          focusedFlowIndex
	classIndex         focusedClassIndex
	semantics          focusedLanguageSemantics
}

func registeredLanguages() []*languageDefinition {
	return []*languageDefinition{goLanguageDefinition(), typeScriptLanguageDefinition(), javaScriptLanguageDefinition(), pythonLanguageDefinition(), javaLanguageDefinition(), kotlinLanguageDefinition(), cSharpLanguageDefinition(), rustLanguageDefinition(), cLanguageDefinition(), cppLanguageDefinition(), phpLanguageDefinition()}
}

// SupportedLanguages returns stable metadata for all built-in language adapters.
func SupportedLanguages() []Language {
	definitions := registeredLanguages()
	result := make([]Language, 0, len(definitions))
	for _, definition := range definitions {
		info := definition.info
		info.Extensions = append([]string(nil), info.Extensions...)
		result = append(result, info)
	}
	return result
}

// LanguageForPath identifies the built-in language adapter for path.
func LanguageForPath(path string) (Language, bool) {
	definition, ok := languageDefinitionForPath(path)
	if !ok {
		return Language{}, false
	}
	info := definition.info
	info.Extensions = append([]string(nil), info.Extensions...)
	return info, true
}

func languageDefinitionForPath(path string) (*languageDefinition, bool) {
	language := codeparser.LanguageFor(path)
	if language == "tsx" {
		language = "typescript"
	}
	return languageDefinitionForID(language)
}

func parserLanguageExtensions(ids ...string) []string {
	var extensions []string
	for _, id := range ids {
		capability, ok := codeparser.CapabilitiesForLanguage(id)
		if ok {
			extensions = append(extensions, capability.Extensions...)
		}
	}
	return extensions
}

func languageDefinitionForID(language string) (*languageDefinition, bool) {
	for _, definition := range registeredLanguages() {
		if definition.info.ID == language {
			return definition, true
		}
	}
	return nil, false
}

func languageForPath(path string) string {
	definition, ok := languageDefinitionForPath(path)
	if !ok {
		return ""
	}
	return definition.info.ID
}

func supportedSourceDescription() string {
	definitions := registeredLanguages()
	names := make([]string, 0, len(definitions))
	for _, definition := range definitions {
		names = append(names, definition.info.ID)
	}
	sort.Strings(names)
	return strings.Join(names, " or ")
}

func unsupportedLanguageError(path string) error {
	return fmt.Errorf("unsupported source language for %s (supported: %s)", path, supportedSourceDescription())
}

func isSupportedLanguage(language string) bool {
	for _, definition := range registeredLanguages() {
		if definition.info.ID == language {
			return true
		}
	}
	return false
}

func adapterProjectRoot(language, directory string) string {
	for _, definition := range registeredLanguages() {
		if definition.info.ID == language {
			return definition.nearestProjectRoot(filepath.Clean(directory))
		}
	}
	return ""
}
