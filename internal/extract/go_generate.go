package extract

import (
	"fmt"
	"strings"
)

func generateGoClass(entry string, entrySource Source, sources []Source, options GenerateOptions) (string, error) {
	depth, nodeLimit := classOptions(options)
	entryAnalysis, err := Analyze([]Source{entrySource})
	if err != nil {
		return "", err
	}
	if entryAnalysis.Declarations[entry] == nil {
		return "", fmt.Errorf("Go declaration '%s' was not found in entry file %s", entry, entrySource.Path)
	}
	identity := entryAnalysis.Declarations[entry].PackageID
	analysis, err := Analyze(sources)
	if err != nil {
		return "", err
	}
	declarations := map[string]*Declaration{}
	for _, declaration := range analysis.PackageDeclarations {
		if declaration.PackageID == identity {
			declarations[declaration.Name] = declaration
		}
	}
	inferStructuralImplementations(declarations)
	selected, truncated := selectClasses(entry, declarations, depth, nodeLimit)
	lines := []string{"classDiagram", fmt.Sprintf("    %%%% grepple:generated entry %s depth %d max-nodes %d", entry, depth, nodeLimit)}
	if truncated {
		lines = append(lines, fmt.Sprintf("    %%%% grepple:truncated max-nodes %d", nodeLimit))
	}
	for _, name := range selected {
		rendered, renderErr := renderClass(name, declarations[name], analysis)
		if renderErr != nil {
			return "", renderErr
		}
		lines = append(lines, rendered...)
	}
	lines = append(lines, classRelations(selected, declarations)...)
	diagram := strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n"
	return validateGeneratedClass(diagram, sources)
}

func inferStructuralImplementations(declarations map[string]*Declaration) {
	for _, name := range sortedKeys(declarations) {
		declaration := declarations[name]
		if !focusedSemanticsFor(declaration.Language).structuralInterfaces || declaration.Kind != "struct" {
			continue
		}
		for _, candidateName := range sortedKeys(declarations) {
			candidate := declarations[candidateName]
			if candidate.Language == declaration.Language && candidate.Kind == "interface" && len(candidate.Members) > 0 && memberSetSatisfies(declaration.Members, candidate.Members) {
				declaration.Implements[candidateName] = true
			}
		}
	}
}
