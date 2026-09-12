package gritql

import "sort"

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
	languages := make([]LanguageCapabilities, 0, len(targetLanguageAdapters))
	for _, adapter := range targetLanguageAdapters {
		languages = append(languages, LanguageCapabilities{ID: adapter.id, Grammar: adapter.grammar, TreeSitter: adapter.treeSitter})
	}
	sort.Slice(languages, func(i, j int) bool { return languages[i].ID < languages[j].ID })
	return languages
}

// targetLanguageAdapter owns target-language syntax behavior while the compiler,
// query algebra, matcher, evaluator, and diagnostics remain language-neutral.
type targetLanguageAdapter struct {
	id               string
	grammar          string
	treeSitter       string
	metadataLanguage string
	metadataGrammar  string
	goGrammar        string
	compileTemplates func(decodedSnippet, int) ([]Template, string, error)
	rootCategory     func(SnippetContext, string) bool
}

func compileGoTemplates(decoded decodedSnippet, maxDepth int) ([]Template, string, error) {
	template, code, err := compileGoTemplate(decoded, maxDepth)
	if err != nil {
		return nil, code, err
	}
	return []Template{template}, "", nil
}

var targetLanguageAdapters = map[string]targetLanguageAdapter{
	defaultTargetLanguage: {
		id:               defaultTargetLanguage,
		grammar:          GoGrammar,
		treeSitter:       TreeSitterGoGrammar,
		goGrammar:        GoGrammar,
		compileTemplates: compileGoTemplates,
		rootCategory:     goRootCategoryAccepts,
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
	"python": {
		id:               "python",
		grammar:          PythonGrammar,
		treeSitter:       TreeSitterPythonGrammar,
		metadataLanguage: "python",
		metadataGrammar:  PythonGrammar,
		compileTemplates: compilePythonTemplates,
		rootCategory:     pythonRootCategoryAccepts,
	},
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
var unfieldedCardinalityRules = map[string]func(string, bool) bool{
	defaultTargetLanguage: goUnfieldedCardinality,
}

func goUnfieldedCardinality(parentKind string, hasOpenParen bool) bool {
	if parentKind == "const_declaration" || parentKind == "type_declaration" {
		return hasOpenParen
	}
	return true
}

func targetLanguageByID(id string) (targetLanguageAdapter, bool) {
	adapter, ok := targetLanguageAdapters[id]
	return adapter, ok
}

func unfieldedCardinalityAllowed(language, parentKind string, hasOpenParen bool) bool {
	rule, ok := unfieldedCardinalityRules[language]
	if !ok {
		return true
	}
	return rule(parentKind, hasOpenParen)
}
