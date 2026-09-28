package parser

// Parser classifies languages, parses source, and derives projections from caller-owned documents.
// Callers must close each returned Document; projections neither close nor reparse it.
// NavigationGraph reuses the optional navigation fact cache when configured.
type Parser interface {
	// LanguageFor classifies a source path, falling back to text.
	LanguageFor(path string) string
	// SupportedLanguages returns defensive metadata for parser-backed languages.
	SupportedLanguages() []LanguageCapabilities
	// SupportedContentLanguages includes lightweight formats and text.
	SupportedContentLanguages() []ContentLanguageCapabilities
	// CapabilitiesForLanguage returns metadata for a canonical parser language.
	CapabilitiesForLanguage(id string) (LanguageCapabilities, bool)
	// GetGrammar returns immutable pinned grammar metadata for language.
	// Unsupported languages have a grammar whose queries return false or unknown.
	GetGrammar(language string) Grammar
	Parse(language, content string) (*Document, error)
	NavigationGraph(document *Document, path string) NavigationGraph
	Outline(document *Document, path string) FileOutline
	Segments(document *Document, hitLines map[int]bool) ([]Segment, SegmentBuildStatus)
}

type documentParser struct{ disableCache bool }

// ParserOptions configures optional Parser performance behavior.
type ParserOptions struct{ DisableNavigationCache bool }

// NewParser returns the language-neutral parser and document projector.
// The navigation fact cache is enabled only when GREPPLE_NAVIGATION_CACHE_DIR is set.
func NewParser(options ...ParserOptions) Parser {
	if len(options) > 0 {
		return documentParser{disableCache: options[0].DisableNavigationCache}
	}
	return documentParser{}
}

func (documentParser) LanguageFor(path string) string { return languageFor(path) }

func (documentParser) SupportedLanguages() []LanguageCapabilities { return supportedLanguages() }

func (documentParser) SupportedContentLanguages() []ContentLanguageCapabilities {
	return supportedContentLanguages()
}

func (documentParser) CapabilitiesForLanguage(id string) (LanguageCapabilities, bool) {
	return capabilitiesForLanguage(id)
}

func (documentParser) GetGrammar(language string) Grammar {
	return grammarView{language: language}
}

func (documentParser) Parse(language, content string) (*Document, error) {
	return parseDocument(language, content)
}

func (service documentParser) NavigationGraph(document *Document, path string) NavigationGraph {
	if service.disableCache {
		return navigationGraphFromDocument(document, path)
	}
	return cachedNavigationGraphFromDocument(document, path)
}

func (documentParser) Outline(document *Document, path string) FileOutline {
	return outlineFromDocument(path, document)
}

func (documentParser) Segments(document *Document, hitLines map[int]bool) ([]Segment, SegmentBuildStatus) {
	return buildSegmentsFromDocument(document, hitLines)
}
