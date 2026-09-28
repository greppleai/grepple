package parser

// Grammar provides read-only, pinned grammar facts for one language.
// Unsupported languages return false or GrammarCardinalityUnknown.
type Grammar interface {
	NodeKind(kind string) bool
	FieldName(field string) bool
	TokenKind(kind string) bool
	FieldCardinality(parentKind, field string) GrammarCardinality
	ChildrenCardinality(parentKind string) GrammarCardinality
	Subtype(supertype, kind string) bool
}

type grammarView struct{ language string }

func (view grammarView) NodeKind(kind string) bool {
	adapter := adapterForLanguage(view.language)
	if adapter == nil || kind == "" {
		return false
	}
	grammar := adapter.Grammar()
	return grammar.valid() && grammar.raw.IdForNodeKind(kind, true) != 0
}

func (view grammarView) FieldName(field string) bool {
	adapter := adapterForLanguage(view.language)
	if adapter == nil || field == "" {
		return false
	}
	grammar := adapter.Grammar()
	return grammar.valid() && grammar.raw.FieldIdForName(field) != 0
}

func (view grammarView) TokenKind(kind string) bool {
	adapter := adapterForLanguage(view.language)
	if adapter == nil || kind == "" {
		return false
	}
	grammar := adapter.Grammar()
	return grammar.valid() && grammar.raw.IdForNodeKind(kind, false) != 0
}

func (view grammarView) FieldCardinality(parentKind, field string) GrammarCardinality {
	metadata, ok := generatedLanguageMetadata[view.language]
	if !ok {
		return GrammarCardinalityUnknown
	}
	return metadata.fields[parentKind][field]
}

func (view grammarView) ChildrenCardinality(parentKind string) GrammarCardinality {
	metadata, ok := generatedLanguageMetadata[view.language]
	if !ok {
		return GrammarCardinalityUnknown
	}
	return metadata.children[parentKind]
}

func (view grammarView) Subtype(supertype, kind string) bool {
	metadata, ok := generatedLanguageMetadata[view.language]
	return ok && metadata.subtypes[supertype][kind]
}
