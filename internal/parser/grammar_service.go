package parser

// Grammar provides read-only, pinned grammar facts for one language.
// Composite languages include their explicitly supported embedded grammars.
type Grammar interface {
	NodeKind(string) bool
	FieldName(string) bool
	TokenKind(string) bool
	FieldCardinality(string, string) GrammarCardinality
	ChildrenCardinality(string) GrammarCardinality
	Subtype(string, string) bool
}

type grammarComposition interface{ GrammarLanguages() []string }
type grammarView struct{ language string }

func (view grammarView) languages() []string {
	if composite, ok := adapterForLanguage(view.language).(grammarComposition); ok {
		return composite.GrammarLanguages()
	}
	return []string{view.language}
}
func (view grammarView) NodeKind(kind string) bool {
	for _, language := range view.languages() {
		adapter := adapterForLanguage(language)
		if adapter != nil && kind != "" && adapter.Grammar().valid() && adapter.Grammar().raw.IdForNodeKind(kind, true) != 0 {
			return true
		}
	}
	return false
}
func (view grammarView) FieldName(field string) bool {
	for _, language := range view.languages() {
		adapter := adapterForLanguage(language)
		if adapter != nil && field != "" && adapter.Grammar().valid() && adapter.Grammar().raw.FieldIdForName(field) != 0 {
			return true
		}
	}
	return false
}
func (view grammarView) TokenKind(kind string) bool {
	for _, language := range view.languages() {
		adapter := adapterForLanguage(language)
		if adapter != nil && kind != "" && adapter.Grammar().valid() && adapter.Grammar().raw.IdForNodeKind(kind, false) != 0 {
			return true
		}
	}
	return false
}
func (view grammarView) FieldCardinality(parent, field string) GrammarCardinality {
	value := GrammarCardinalityUnknown
	for _, language := range view.languages() {
		candidate := generatedLanguageMetadata[language].fields[parent][field]
		if candidate == GrammarCardinalityMany {
			return candidate
		}
		if candidate != GrammarCardinalityUnknown {
			value = candidate
		}
	}
	return value
}
func (view grammarView) ChildrenCardinality(parent string) GrammarCardinality {
	value := GrammarCardinalityUnknown
	for _, language := range view.languages() {
		candidate := generatedLanguageMetadata[language].children[parent]
		if candidate == GrammarCardinalityMany {
			return candidate
		}
		if candidate != GrammarCardinalityUnknown {
			value = candidate
		}
	}
	return value
}
func (view grammarView) Subtype(supertype, kind string) bool {
	for _, language := range view.languages() {
		if generatedLanguageMetadata[language].subtypes[supertype][kind] {
			return true
		}
	}
	return false
}
