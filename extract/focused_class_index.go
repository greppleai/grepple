package extract

type focusedClassIndex interface {
	declaration(string, *DiagramClass, *Analysis) *Declaration
	declarationCount(*DiagramClass, *Analysis) int
	memberScopeMatches(*DiagramClass, Member, *Analysis) bool
	importScopeMatches(*DiagramClass, Import, *Analysis) bool
}

type packageFocusedClassIndex struct{}

func (packageFocusedClassIndex) declaration(name string, class *DiagramClass, analysis *Analysis) *Declaration {
	var result *Declaration
	for _, declaration := range analysis.PackageDeclarations {
		if declaration.Name != name || !packageScopeMatches(class.Package, declaration.Package, declaration.PackageID, analysis) {
			continue
		}
		if result != nil {
			return nil
		}
		result = declaration
	}
	return result
}

func (packageFocusedClassIndex) declarationCount(class *DiagramClass, analysis *Analysis) int {
	count := 0
	for _, declaration := range analysis.PackageDeclarations {
		if declaration.Name == class.Name && packageScopeMatches(class.Package, declaration.Package, declaration.PackageID, analysis) {
			count++
		}
	}
	return count
}

func (packageFocusedClassIndex) memberScopeMatches(class *DiagramClass, member Member, analysis *Analysis) bool {
	return packageScopeMatches(class.Package, member.Package, member.PackageID, analysis)
}

func (packageFocusedClassIndex) importScopeMatches(class *DiagramClass, item Import, analysis *Analysis) bool {
	return packageScopeMatches(class.Package, item.Package, item.PackageID, analysis)
}

type moduleFocusedClassIndex struct{}

func (moduleFocusedClassIndex) declaration(name string, class *DiagramClass, analysis *Analysis) *Declaration {
	if class.Module == "" {
		return analysis.DeclarationVariants[class.Language+":"+name]
	}
	moduleID := resolveModuleScope(class.Module, analysis)
	if moduleID == "" {
		return nil
	}
	return analysis.ModuleDeclarations[moduleDeclarationKey(analysis, moduleID, name)]
}

func (moduleFocusedClassIndex) declarationCount(class *DiagramClass, analysis *Analysis) int {
	count, moduleID := 0, resolveModuleScope(class.Module, analysis)
	for _, declaration := range analysis.ModuleDeclarations {
		if declaration.Name == class.Name && (class.Language == "" || declaration.Language == class.Language) && (class.Module == "" || moduleID == declaration.ModuleID) {
			count++
		}
	}
	return count
}

func (moduleFocusedClassIndex) memberScopeMatches(class *DiagramClass, member Member, analysis *Analysis) bool {
	return class.Module == "" || resolveModuleScope(class.Module, analysis) == member.ModuleID
}

func (moduleFocusedClassIndex) importScopeMatches(class *DiagramClass, item Import, analysis *Analysis) bool {
	return class.Module == "" || resolveModuleScope(class.Module, analysis) == item.ImporterModuleID
}

func focusedClassIndexFor(class *DiagramClass) focusedClassIndex {
	if class == nil {
		return nil
	}
	if class.Package != "" {
		return packageFocusedClassIndex{}
	}
	if class.Module != "" {
		return moduleFocusedClassIndex{}
	}
	if class.Language == "" {
		return nil
	}
	definition, ok := languageDefinitionForID(class.Language)
	if !ok {
		return nil
	}
	return definition.classIndex
}
