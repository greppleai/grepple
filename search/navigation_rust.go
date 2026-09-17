package search

import (
	"sort"

	"github.com/greppleai/grepple/parser"
)

func (index *navigationIndex) rustModuleIndex() *parser.RustModuleIndex {
	if index.rustModules == nil {
		paths := make([]string, 0, len(index.contents))
		for path := range index.contents {
			paths = append(paths, path)
		}
		sort.Strings(paths)
		index.rustModules = parser.BuildRustModuleIndex(index.graph, paths)
	}
	return index.rustModules
}

func (index *navigationIndex) rustImportTargetFiles(sourceFile, sourceScope, importPath string) []string {
	return rustModuleTargetFiles(index.rustModuleIndex().ResolveImportFrom(sourceFile, sourceScope, importPath))
}

func (index *navigationIndex) rustVisibleCandidates(call navigationCall, candidates []navigationDeclaration) []navigationDeclaration {
	if navigationLanguageFamily(call.language) != "rust" {
		return candidates
	}
	if call.importPath != "" && len(call.rustTargetModules) == 0 {
		return nil
	}
	sourceModules := index.rustModuleIndex().ModuleKeys(call.file, call.moduleScope)
	if len(sourceModules) == 0 {
		return candidates
	}
	visible := make([]navigationDeclaration, 0, len(candidates))
	for _, candidate := range candidates {
		if index.rustCandidateVisibleFrom(candidate, sourceModules) {
			visible = append(visible, candidate)
		}
	}
	return visible
}

func (index *navigationIndex) rustCandidateVisibleFrom(candidate navigationDeclaration, sourceModules []string) bool {
	declarationModules := index.rustModuleIndex().ModuleKeys(candidate.file, candidate.moduleScope)
	for _, declarationModule := range declarationModules {
		for _, sourceModule := range sourceModules {
			if parser.RustModulesShareCrate(declarationModule, sourceModule) && parser.RustItemVisibleFrom(declarationModule, sourceModule, candidate.visibilityDetail) {
				return true
			}
		}
	}
	return false
}

func (index *navigationIndex) rustReExportTargets(sourceFile, sourceScope, importPath, name string, seen map[string]bool) []parser.RustModuleTarget {
	if importPath == "" {
		return nil
	}
	result := []parser.RustModuleTarget{}
	for _, candidate := range index.rustModuleIndex().ResolveImportFrom(sourceFile, sourceScope, importPath) {
		key := parser.RustModuleTargetModuleKey(candidate) + "\x00" + name
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, candidate)
		for _, export := range index.exports[navigationSymbolKey("rust", candidate.Path)] {
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

func rustModuleTargetFiles(targets []parser.RustModuleTarget) []string {
	files := make([]string, 0, len(targets))
	for _, target := range targets {
		files = append(files, target.Path)
	}
	return compactSortedStrings(files)
}

func rustModuleTargetKeys(targets []parser.RustModuleTarget) []string {
	keys := make([]string, 0, len(targets))
	for _, target := range targets {
		keys = append(keys, parser.RustModuleTargetModuleKey(target))
	}
	return compactSortedStrings(keys)
}

func compactRustModuleTargets(targets []parser.RustModuleTarget) []parser.RustModuleTarget {
	sort.Slice(targets, func(i, j int) bool {
		return parser.RustModuleTargetKey(targets[i]) < parser.RustModuleTargetKey(targets[j])
	})
	result := targets[:0]
	last := ""
	for _, target := range targets {
		key := parser.RustModuleTargetKey(target)
		if len(result) == 0 || key != last {
			result = append(result, target)
			last = key
		}
	}
	return result
}
