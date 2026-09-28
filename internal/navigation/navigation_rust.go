package navigation

import (
	"sort"

	"github.com/greppleai/grepple/internal/navigation/rustmodule"
)

type rustNavigationIndex struct {
	baseLanguageNavigationIndex
	modules *rustmodule.RustModuleIndex
}

func newRustNavigationIndex(corpus *navigationCorpus) *rustNavigationIndex {
	paths := append([]string(nil), corpus.files...)
	sort.Strings(paths)
	return &rustNavigationIndex{
		baseLanguageNavigationIndex: baseLanguageNavigationIndex{family: "rust", corpus: corpus},
		modules:                     rustmodule.BuildRustModuleIndex(corpus.graph, paths),
	}
}

func (index *rustNavigationIndex) importTargets(request navigationImportRequest) navigationImportTargets {
	sourceFile, sourceScope, importPath := request.sourceFile, request.sourceScope, request.importPath
	targets := index.modules.ResolveImportFrom(sourceFile, sourceScope, importPath)
	return navigationImportTargets{files: rustModuleTargetFiles(targets), scopes: rustModuleTargetKeys(targets)}
}

func (index *rustNavigationIndex) filterCandidates(call navigationCall, candidates []navigationDeclaration) []navigationDeclaration {
	if call.importPath != "" && len(call.importTargetScopes) == 0 {
		return nil
	}
	sourceModules := index.modules.ModuleKeys(call.file, call.moduleScope)
	if len(sourceModules) == 0 {
		return candidates
	}
	visible := make([]navigationDeclaration, 0, len(candidates))
	for _, candidate := range candidates {
		if index.candidateVisibleFrom(candidate, sourceModules) {
			visible = append(visible, candidate)
		}
	}
	return visible
}

func (index *rustNavigationIndex) candidateVisibleFrom(candidate navigationDeclaration, sourceModules []string) bool {
	declarationModules := index.modules.ModuleKeys(candidate.file, candidate.moduleScope)
	for _, declarationModule := range declarationModules {
		for _, sourceModule := range sourceModules {
			if rustmodule.RustModulesShareCrate(declarationModule, sourceModule) && rustmodule.RustItemVisibleFrom(declarationModule, sourceModule, candidate.visibilityDetail) {
				return true
			}
		}
	}
	return false
}

func (index *rustNavigationIndex) reExportTargets(sourceFile, sourceScope, importPath, name string, seen map[string]bool) navigationImportTargets {
	targets := index.rustReExportTargets(sourceFile, sourceScope, importPath, name, seen)
	return navigationImportTargets{files: rustModuleTargetFiles(targets), scopes: rustModuleTargetKeys(targets)}
}

func (index *rustNavigationIndex) rustReExportTargets(sourceFile, sourceScope, importPath, name string, seen map[string]bool) []rustmodule.RustModuleTarget {
	if importPath == "" {
		return nil
	}
	result := []rustmodule.RustModuleTarget{}
	for _, candidate := range index.modules.ResolveImportFrom(sourceFile, sourceScope, importPath) {
		key := rustmodule.RustModuleTargetModuleKey(candidate) + "\x00" + name
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, candidate)
		for _, export := range index.corpus.exports[navigationSymbolKey("rust", candidate.Path)] {
			if export.scope != candidate.LocalScope || (export.name != name && export.name != "*") || export.importPath == "" {
				continue
			}
			nextName := export.importedName
			if nextName == "" || nextName == "*" {
				nextName = name
			}
			result = append(result, index.rustReExportTargets(export.file, export.scope, export.importPath, nextName, seen)...)
		}
	}
	return compactRustModuleTargets(result)
}

func (index *rustNavigationIndex) importMatches(call navigationCall, candidate navigationDeclaration) bool {
	if len(call.importTargetScopes) == 0 {
		return index.baseLanguageNavigationIndex.importMatches(call, candidate)
	}
	for _, candidateModule := range index.modules.ModuleKeys(candidate.file, candidate.moduleScope) {
		if stringSliceContains(call.importTargetScopes, candidateModule) {
			return true
		}
	}
	return false
}

func rustModuleTargetFiles(targets []rustmodule.RustModuleTarget) []string {
	files := make([]string, 0, len(targets))
	for _, target := range targets {
		files = append(files, target.Path)
	}
	return compactSortedStrings(files)
}

func rustModuleTargetKeys(targets []rustmodule.RustModuleTarget) []string {
	keys := make([]string, 0, len(targets))
	for _, target := range targets {
		keys = append(keys, rustmodule.RustModuleTargetModuleKey(target))
	}
	return compactSortedStrings(keys)
}

func compactRustModuleTargets(targets []rustmodule.RustModuleTarget) []rustmodule.RustModuleTarget {
	sort.Slice(targets, func(i, j int) bool {
		return rustmodule.RustModuleTargetKey(targets[i]) < rustmodule.RustModuleTargetKey(targets[j])
	})
	result := targets[:0]
	last := ""
	for _, target := range targets {
		key := rustmodule.RustModuleTargetKey(target)
		if len(result) == 0 || key != last {
			result = append(result, target)
			last = key
		}
	}
	return result
}
