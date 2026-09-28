package navigation

import (
	"sort"

	"github.com/greppleai/grepple/parser"
)

type navigationImportTargets struct {
	files  []string
	scopes []string
}

type navigationCorpus struct {
	contents     map[string]string
	exports      map[string][]navigationExport
	graph        parser.NavigationGraph
	files        []string
	paths        *navigationPathIndex
	exportLookup *navigationExportIndex
}

type languageNavigationIndex interface {
	bind(languageNavigationIndex)
	importTargets(sourceFile, sourceScope, importPath, imported, kind string) navigationImportTargets
	reExportTargets(sourceFile, sourceScope, importPath, name string, seen map[string]bool) navigationImportTargets
	filterCandidates(navigationCall, []navigationDeclaration) []navigationDeclaration
	importMatches(navigationCall, navigationDeclaration) bool
	fieldOriginMatches(navigationField, navigationField) bool
}

type baseLanguageNavigationIndex struct {
	family string
	corpus *navigationCorpus
	owner  languageNavigationIndex
}

func (index *baseLanguageNavigationIndex) bind(owner languageNavigationIndex) {
	index.owner = owner
}

func (index *baseLanguageNavigationIndex) importTargets(_, _, _, _, _ string) navigationImportTargets {
	return navigationImportTargets{}
}

func (index *baseLanguageNavigationIndex) reExportTargets(sourceFile, sourceScope, importPath, name string, seen map[string]bool) navigationImportTargets {
	if importPath == "" {
		return navigationImportTargets{}
	}
	result := navigationImportTargets{}
	candidates := index.owner.importTargets(sourceFile, sourceScope, importPath, "", "")
	for _, candidateFile := range candidates.files {
		key := navigationSymbolKey(index.family, candidateFile) + "\x00" + name
		if seen[key] {
			continue
		}
		seen[key] = true
		result.files = append(result.files, candidateFile)
		for _, export := range index.corpus.exports[navigationSymbolKey(index.family, candidateFile)] {
			if export.name != name && export.name != "*" {
				continue
			}
			nextName := export.importedName
			if nextName == "" || nextName == "*" {
				nextName = name
			}
			nested := index.owner.reExportTargets(export.file, export.scope, export.importPath, nextName, seen)
			result.files = append(result.files, nested.files...)
		}
	}
	sort.Strings(result.files)
	result.files = compactSortedStrings(result.files)
	return result
}

func (*baseLanguageNavigationIndex) filterCandidates(_ navigationCall, candidates []navigationDeclaration) []navigationDeclaration {
	return candidates
}

func (index *baseLanguageNavigationIndex) importMatches(call navigationCall, candidate navigationDeclaration) bool {
	if stringSliceContains(call.importTargetFiles, candidate.file) {
		return true
	}
	if call.importPath == "" {
		return false
	}
	targets := index.owner.importTargets(call.importSourceFile, call.moduleScope, call.importPath, "", "")
	return stringSliceContains(targets.files, candidate.file)
}

func (*baseLanguageNavigationIndex) fieldOriginMatches(candidate, owner navigationField) bool {
	return candidate.file == owner.file
}

type languageNavigationIndexBuildContext struct {
	corpus       *navigationCorpus
	replacements map[string]string
	packages     map[string][]string
}

type languageNavigationIndexFactory func(string, baseLanguageNavigationIndex, languageNavigationIndexBuildContext) languageNavigationIndex

var languageNavigationIndexFactories = map[string]languageNavigationIndexFactory{
	"go": func(_ string, base baseLanguageNavigationIndex, context languageNavigationIndexBuildContext) languageNavigationIndex {
		return &goNavigationIndex{baseLanguageNavigationIndex: base, replacements: context.replacements, packages: context.packages}
	},
	"rust": func(_ string, _ baseLanguageNavigationIndex, context languageNavigationIndexBuildContext) languageNavigationIndex {
		return newRustNavigationIndex(context.corpus)
	},
	"python": func(_ string, base baseLanguageNavigationIndex, _ languageNavigationIndexBuildContext) languageNavigationIndex {
		return &pythonNavigationIndex{baseLanguageNavigationIndex: base}
	},
	"javascript": func(_ string, base baseLanguageNavigationIndex, _ languageNavigationIndexBuildContext) languageNavigationIndex {
		return newECMANavigationIndex(base)
	},
	"typescript": func(_ string, base baseLanguageNavigationIndex, _ languageNavigationIndexBuildContext) languageNavigationIndex {
		return newECMANavigationIndex(base)
	},
	"java": func(_ string, base baseLanguageNavigationIndex, _ languageNavigationIndexBuildContext) languageNavigationIndex {
		return &qualifiedExportNavigationIndex{baseLanguageNavigationIndex: base}
	},
	"kotlin": func(_ string, base baseLanguageNavigationIndex, _ languageNavigationIndexBuildContext) languageNavigationIndex {
		return &qualifiedExportNavigationIndex{baseLanguageNavigationIndex: base}
	},
	"csharp": func(_ string, base baseLanguageNavigationIndex, _ languageNavigationIndexBuildContext) languageNavigationIndex {
		return &cSharpNavigationIndex{baseLanguageNavigationIndex: base}
	},
	"c": func(_ string, base baseLanguageNavigationIndex, _ languageNavigationIndexBuildContext) languageNavigationIndex {
		return &cFamilyNavigationIndex{baseLanguageNavigationIndex: base}
	},
	"cpp": func(_ string, base baseLanguageNavigationIndex, _ languageNavigationIndexBuildContext) languageNavigationIndex {
		return &cFamilyNavigationIndex{baseLanguageNavigationIndex: base}
	},
	"php": func(_ string, base baseLanguageNavigationIndex, _ languageNavigationIndexBuildContext) languageNavigationIndex {
		return &qualifiedExportNavigationIndex{baseLanguageNavigationIndex: base}
	},
	"shell": func(_ string, base baseLanguageNavigationIndex, _ languageNavigationIndexBuildContext) languageNavigationIndex {
		return &base
	},
}

func newLanguageNavigationIndexes(corpus *navigationCorpus, paths []string) (map[string]languageNavigationIndex, []string) {
	roots, replacements, packages := goRepositoryContext(paths)
	context := languageNavigationIndexBuildContext{corpus: corpus, replacements: replacements, packages: packages}
	families := make([]string, 0, len(languageNavigationIndexFactories))
	for family := range languageNavigationIndexFactories {
		families = append(families, family)
	}
	sort.Strings(families)
	indexes := make(map[string]languageNavigationIndex, len(families))
	for _, family := range families {
		base := baseLanguageNavigationIndex{family: family, corpus: corpus}
		index := languageNavigationIndexFactories[family](family, base, context)
		index.bind(index)
		indexes[family] = index
	}
	return indexes, roots
}
