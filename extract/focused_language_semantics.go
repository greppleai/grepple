package extract

type focusedLanguageSemantics struct {
	packageMetadata             bool
	defaultModuleMetadata       bool
	explicitDeclarationKinds    bool
	exactFileMembers            bool
	underlyingTypes             bool
	structTags                  bool
	fileLocalTypes              bool
	structuralInterfaces        bool
	packageTypeReferences       bool
	restrictedStructuralMembers bool
	moduleReferences            bool
	moduleExports               bool
}

func focusedSemanticsFor(language string) focusedLanguageSemantics {
	if definition, ok := languageDefinitionForID(language); ok {
		return definition.semantics
	}
	return focusedLanguageSemantics{}
}

func focusedLanguageWith(predicate func(focusedLanguageSemantics) bool) string {
	for _, definition := range registeredLanguages() {
		if predicate(definition.semantics) {
			return definition.info.ID
		}
	}
	return ""
}

func packageMetadataLanguage() string {
	return focusedLanguageWith(func(semantics focusedLanguageSemantics) bool { return semantics.packageMetadata })
}

func defaultModuleMetadataLanguage() string {
	return focusedLanguageWith(func(semantics focusedLanguageSemantics) bool { return semantics.defaultModuleMetadata })
}

func explicitDeclarationKindLanguage() string {
	return focusedLanguageWith(func(semantics focusedLanguageSemantics) bool { return semantics.explicitDeclarationKinds })
}
