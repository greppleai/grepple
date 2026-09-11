package gritql

const (
	defaultTargetLanguage = "go"
	// TypeScriptGrammar identifies the TypeScript syntax contract.
	TypeScriptGrammar = "typescript"
	// TSXGrammar identifies the TSX syntax contract.
	TSXGrammar = "tsx"
	// TreeSitterTypeScriptGrammar identifies the pinned TypeScript grammar implementation.
	TreeSitterTypeScriptGrammar = "tree-sitter-typescript@0.23.2"
)

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
