package gritql

import (
	"sort"
	"sync"

	"github.com/greppleai/grepple/internal/parser"
)

const (
	defaultTargetLanguage = "go"
	// JavaScriptGrammar identifies the JavaScript syntax contract.
	JavaScriptGrammar = "javascript"
	// TreeSitterJavaScriptGrammar identifies the pinned JavaScript grammar implementation.
	TreeSitterJavaScriptGrammar = "tree-sitter-javascript@0.25.0"
	// PythonGrammar identifies the Python syntax contract.
	PythonGrammar = "python"
	// TreeSitterPythonGrammar identifies the pinned Python grammar implementation.
	TreeSitterPythonGrammar = "tree-sitter-python@0.25.0"
	// TypeScriptGrammar identifies the TypeScript syntax contract.
	TypeScriptGrammar = "typescript"
	// TSXGrammar identifies the TSX syntax contract.
	TSXGrammar = "tsx"
	// TreeSitterTypeScriptGrammar identifies the pinned TypeScript grammar implementation.
	TreeSitterTypeScriptGrammar = "tree-sitter-typescript@0.23.2"
)

// LanguageCapabilities describes one production GritQL target adapter.
type LanguageCapabilities struct {
	ID         string
	Grammar    string
	TreeSitter string
}

// SupportedLanguages returns deterministic metadata for registered GritQL targets.
func SupportedLanguages() []LanguageCapabilities {
	adapters := targetLanguageAdapterMap()
	languages := make([]LanguageCapabilities, 0, len(adapters))
	for _, adapter := range adapters {
		languages = append(languages, LanguageCapabilities{ID: adapter.id, Grammar: adapter.grammar, TreeSitter: adapter.treeSitter})
	}
	sort.Slice(languages, func(i, j int) bool { return languages[i].ID < languages[j].ID })
	return languages
}

// targetLanguageAdapter owns target-language syntax behavior while the compiler,
// query algebra, matcher, evaluator, and diagnostics remain language-neutral.
type targetLanguageAdapter struct {
	id                     string
	grammar                string
	treeSitter             string
	metadataLanguage       string
	metadataGrammar        string
	goGrammar              string
	compileTemplates       func(decodedSnippet, int) ([]Template, string, error)
	rootCategory           func(SnippetContext, string) bool
	unfieldedCardinality   func(string, bool) bool
	placeholderCardinality func(parser.Node, parser.Node) (SlotCardinality, bool)
}

func compileGoTemplates(decoded decodedSnippet, maxDepth int) ([]Template, string, error) {
	template, code, err := compileGoTemplate(decoded, maxDepth)
	if err != nil {
		return nil, code, err
	}
	return []Template{template}, "", nil
}
func wrappedTargetLanguageAdapter(id, grammar, treeSitter string, compile func(decodedSnippet, int) ([]Template, string, error)) targetLanguageAdapter {
	config := wrappedLanguageByID(id)
	return targetLanguageAdapter{
		id: id, grammar: grammar, treeSitter: treeSitter, metadataLanguage: id, metadataGrammar: grammar, compileTemplates: compile,
		rootCategory: func(context SnippetContext, kind string) bool {
			return wrappedRootCategoryAccepts(id, config.declarations, context, kind)
		},
	}
}

var targetLanguageAdaptersOnce sync.Once
var targetLanguageAdapters map[string]targetLanguageAdapter

func targetLanguageAdapterMap() map[string]targetLanguageAdapter {
	targetLanguageAdaptersOnce.Do(func() {
		targetLanguageAdapters = newTargetLanguageAdapters()
	})
	return targetLanguageAdapters
}

func newTargetLanguageAdapters() map[string]targetLanguageAdapter {
	return map[string]targetLanguageAdapter{
		"c":      wrappedTargetLanguageAdapter("c", CGrammar, TreeSitterCGrammar, compileCTemplates),
		"cpp":    wrappedTargetLanguageAdapter("cpp", CPPGrammar, TreeSitterCPPGrammar, compileCPPTemplates),
		"csharp": wrappedTargetLanguageAdapter("csharp", CSharpGrammar, TreeSitterCSharpGrammar, compileCSharpTemplates),
		"dart":   wrappedTargetLanguageAdapter("dart", DartGrammar, TreeSitterDartGrammar, compileDartTemplates),
		"hcl":    wrappedTargetLanguageAdapter("hcl", HCLGrammar, TreeSitterHCLGrammar, compileHCLTemplates),
		defaultTargetLanguage: {
			id:                   defaultTargetLanguage,
			grammar:              GoGrammar,
			treeSitter:           TreeSitterGoGrammar,
			goGrammar:            GoGrammar,
			compileTemplates:     compileGoTemplates,
			rootCategory:         goRootCategoryAccepts,
			unfieldedCardinality: goUnfieldedCardinality,
		},
		"javascript": {
			id:               "javascript",
			grammar:          JavaScriptGrammar,
			treeSitter:       TreeSitterJavaScriptGrammar,
			metadataLanguage: "javascript",
			metadataGrammar:  JavaScriptGrammar,
			compileTemplates: compileJavaScriptTemplates,
			rootCategory:     javaScriptRootCategoryAccepts,
		},
		"java":   wrappedTargetLanguageAdapter("java", JavaGrammar, TreeSitterJavaGrammar, compileJavaTemplates),
		"kotlin": wrappedTargetLanguageAdapter("kotlin", KotlinGrammar, TreeSitterKotlinGrammar, compileKotlinTemplates),
		"python": {
			id:               "python",
			grammar:          PythonGrammar,
			treeSitter:       TreeSitterPythonGrammar,
			metadataLanguage: "python",
			metadataGrammar:  PythonGrammar,
			compileTemplates: compilePythonTemplates,
			rootCategory:     pythonRootCategoryAccepts,
		},
		"rust":   wrappedTargetLanguageAdapter("rust", RustGrammar, TreeSitterRustGrammar, compileRustTemplates),
		"php":    wrappedTargetLanguageAdapter("php", PHPGrammar, TreeSitterPHPGrammar, compilePHPTemplates),
		"shell":  wrappedTargetLanguageAdapter("shell", ShellGrammar, TreeSitterShellGrammar, compileShellTemplates),
		"swift":  wrappedTargetLanguageAdapter("swift", SwiftGrammar, TreeSitterSwiftGrammar, compileSwiftTemplates),
		"svelte": svelteTargetLanguageAdapter(),
		"css":    cssTargetLanguageAdapter(),
		"typescript": {
			id:               "typescript",
			grammar:          TypeScriptGrammar,
			treeSitter:       TreeSitterTypeScriptGrammar,
			metadataLanguage: "typescript",
			metadataGrammar:  TypeScriptGrammar,
			compileTemplates: compileTypeScriptTemplates,
			rootCategory:     typeScriptRootCategoryAccepts,
		},
		"tsx": {
			id:               "tsx",
			grammar:          TSXGrammar,
			treeSitter:       TreeSitterTypeScriptGrammar,
			metadataLanguage: "tsx",
			metadataGrammar:  TSXGrammar,
			compileTemplates: compileTSXTemplates,
			rootCategory:     tsxRootCategoryAccepts,
		},
	}
}

func goUnfieldedCardinality(parentKind string, hasOpenParen bool) bool {
	if parentKind == "const_declaration" || parentKind == "type_declaration" {
		return hasOpenParen
	}
	return true
}

func targetLanguageByID(id string) (targetLanguageAdapter, bool) {
	adapter, ok := targetLanguageAdapterMap()[id]
	return adapter, ok
}

func unfieldedCardinalityAllowed(language, parentKind string, hasOpenParen bool) bool {
	adapter, ok := targetLanguageByID(language)
	if !ok || adapter.unfieldedCardinality == nil {
		return true
	}
	return adapter.unfieldedCardinality(parentKind, hasOpenParen)
}
