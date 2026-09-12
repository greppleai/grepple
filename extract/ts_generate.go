package extract

import (
	"fmt"
	"strings"
)

type typeScriptClassItem struct {
	key   string
	depth int
}

func generateTypeScriptClass(entry string, entrySource Source, sources []Source, options GenerateOptions) (string, error) {
	analysis, err := Analyze(sources)
	if err != nil {
		return "", err
	}
	entryKey := absolutePath(entrySource.Path) + ":" + entry
	if analysis.TSDeclarations[entryKey] == nil {
		return "", fmt.Errorf("class or interface '%s' was not found in entry file %s", entry, entrySource.Path)
	}
	depth, limit := classOptions(options)
	selected, truncated := selectTypeScriptClasses(entryKey, analysis, depth, limit)
	lines := []string{"classDiagram", fmt.Sprintf("    %%%% grepple:generated entry %s depth %d max-nodes %d", entry, depth, limit)}
	if truncated {
		lines = append(lines, fmt.Sprintf("    %%%% grepple:truncated max-nodes %d", limit))
	}
	declarations := map[string]*Declaration{}
	for _, key := range selected {
		declaration := analysis.TSDeclarations[key]
		if old := declarations[declaration.Name]; old != nil && old.ModuleID != declaration.ModuleID {
			return "", fmt.Errorf("selected declarations share Mermaid name %s", declaration.Name)
		}
		declarations[declaration.Name] = declaration
		rendered, renderErr := renderClass(declaration.Name, declaration, analysis)
		if renderErr != nil {
			return "", renderErr
		}
		lines = append(lines, rendered...)
	}
	lines = append(lines, typeScriptClassRelations(selected, analysis)...)
	diagram := strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n"
	diagnostics, err := CheckClassDiagramWithAnalysis(diagram, analysis)
	if err != nil {
		return "", err
	}
	if len(diagnostics) > 0 {
		return "", fmt.Errorf("Generated class diagram failed validation:\n%s", formatDiagnostics(diagnostics))
	}
	return diagram, nil
}

func selectTypeScriptClasses(entry string, analysis *Analysis, depth, limit int) ([]string, bool) {
	pending := []typeScriptClassItem{{entry, 0}}
	seen := map[string]bool{}
	result := []string{}
	for len(pending) > 0 {
		item := pending[0]
		pending = pending[1:]
		if seen[item.key] || analysis.TSDeclarations[item.key] == nil {
			continue
		}
		if len(result) >= limit {
			return result, true
		}
		seen[item.key], result = true, append(result, item.key)
		if item.depth >= depth {
			continue
		}
		for _, dependency := range typeScriptDeclarationDependencies(analysis.TSDeclarations[item.key], analysis) {
			if !seen[dependency] {
				pending = append(pending, typeScriptClassItem{dependency, item.depth + 1})
			}
		}
	}
	return result, false
}

func typeScriptDeclarationDependencies(declaration *Declaration, analysis *Analysis) []string {
	result := map[string]bool{}
	candidates := typeScriptDependencyCandidates(declaration, analysis)
	for local, key := range candidates {
		if typeScriptDeclarationReferences(declaration, local) {
			result[key] = true
		}
	}
	return sortedKeys(result)
}

func typeScriptDependencyCandidates(declaration *Declaration, analysis *Analysis) map[string]string {
	result := sameModuleDependencyCandidates(declaration, analysis)
	if declaration.Language == "python" || declaration.Language == "java" || declaration.Language == "kotlin" {
		addUniqueModuleDependencyCandidates(result, declaration.Language, analysis)
	}
	addImportedDependencyCandidates(result, declaration.ModuleID, analysis)
	return result
}

func sameModuleDependencyCandidates(declaration *Declaration, analysis *Analysis) map[string]string {
	result := map[string]string{}
	for key, candidate := range analysis.TSDeclarations {
		if candidate.ModuleID == declaration.ModuleID {
			result[candidate.Name] = key
		}
	}
	return result
}

func addUniqueModuleDependencyCandidates(result map[string]string, language string, analysis *Analysis) {
	unique := map[string]string{}
	ambiguous := map[string]bool{}
	for key, candidate := range analysis.TSDeclarations {
		if candidate.Language != language {
			continue
		}
		if old := unique[candidate.Name]; old != "" && old != key {
			ambiguous[candidate.Name] = true
		} else {
			unique[candidate.Name] = key
		}
	}
	for name, key := range unique {
		if !ambiguous[name] {
			result[name] = key
		}
	}
}

func addImportedDependencyCandidates(result map[string]string, moduleID string, analysis *Analysis) {
	for local, binding := range analysis.TSImportBindings[moduleID] {
		if binding.ModuleID == "" || binding.Resolved == "" {
			continue
		}
		key := binding.ModuleID + ":" + binding.Resolved
		if analysis.TSDeclarations[key] != nil {
			result[local] = key
		}
	}
}

func typeScriptDeclarationReferences(declaration *Declaration, name string) bool {
	if declaration.Extends[name] || declaration.Implements[name] {
		return true
	}
	for _, member := range declaration.Members {
		if typeReferences(member.Type, name) {
			return true
		}
		for _, parameter := range member.Parameters {
			if typeReferences(parameter, name) {
				return true
			}
		}
	}
	return false
}

func typeScriptClassRelations(selected []string, analysis *Analysis) []string {
	chosen, result := stringSet(selected), map[string]bool{}
	for _, ownerKey := range selected {
		owner := analysis.TSDeclarations[ownerKey]
		for alias, targetKey := range typeScriptDependencyCandidates(owner, analysis) {
			if !chosen[targetKey] || targetKey == ownerKey {
				continue
			}
			target := analysis.TSDeclarations[targetKey]
			addTypeScriptRelation(owner, target, alias, result)
		}
	}
	return sortedKeys(result)
}

func addTypeScriptRelation(owner, target *Declaration, alias string, result map[string]bool) {
	if owner.Extends[alias] {
		result[fmt.Sprintf("    %s <|-- %s", target.Name, owner.Name)] = true
		return
	}
	if owner.Implements[alias] {
		result[fmt.Sprintf("    %s <|.. %s", target.Name, owner.Name)] = true
		return
	}
	if found, many := propertyReference(owner.Members, alias); found {
		result[associationRelation(owner.Name, target.Name, many)] = true
		return
	}
	if methodReferences(owner.Members, alias) {
		result[fmt.Sprintf("    %s ..> %s", owner.Name, target.Name)] = true
	}
}

func methodReferences(members []Member, target string) bool {
	for _, member := range members {
		if member.Kind == "property" {
			continue
		}
		if typeReferences(member.Type, target) {
			return true
		}
		for _, parameter := range member.Parameters {
			if typeReferences(parameter, target) {
				return true
			}
		}
	}
	return false
}
