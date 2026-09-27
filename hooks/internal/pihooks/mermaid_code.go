package pihooks

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	mermaidcode "github.com/greppleai/grepple/internal/extract"
)

const mermaidCodeRule = "mermaid-code"

var mermaidExcludedDirectories = map[string]bool{
	".git": true, ".worktrees": true, "dist": true, "generated": true,
	"node_modules": true, "vendor": true,
}

// AnalyzeMermaidSchemas validates conventionally named source-backed Mermaid schemas.
func AnalyzeMermaidSchemas(root string) ([]Diagnostic, error) {
	diagrams, err := discoverMermaidSchemas(root)
	if err != nil {
		return nil, err
	}
	var result []Diagnostic
	if len(diagrams) > 0 {
		sources, loadErr := mermaidcode.LoadSources([]string{root})
		if loadErr != nil {
			return nil, fmt.Errorf("load sources for Mermaid schemas: %w", loadErr)
		}
		sources = filterMermaidAnalysisSources(sources)
		analysis, analyzeErr := mermaidcode.Analyze(sources)
		if analyzeErr != nil {
			return nil, fmt.Errorf("analyze sources for Mermaid schemas: %w", analyzeErr)
		}
		legacy, validateErr := validateMermaidSchemas(diagrams, analysis)
		if validateErr != nil {
			return nil, validateErr
		}
		result = append(result, legacy...)
	}
	sortMermaidDiagnostics(result)
	return result, nil
}

func filterMermaidAnalysisSources(sources []mermaidcode.Source) []mermaidcode.Source {
	generatedRoots := make([]string, 0)
	for _, source := range sources {
		if !isGeneratedTreeSitterParser(source) {
			continue
		}
		root := filepath.Dir(source.Path)
		if filepath.Base(root) == "src" {
			root = filepath.Dir(root)
		}
		generatedRoots = append(generatedRoots, filepath.Clean(root))
	}
	if len(generatedRoots) == 0 {
		return sources
	}
	filtered := sources[:0]
	for _, source := range sources {
		if sourceWithinAnyRoot(source.Path, generatedRoots) {
			continue
		}
		filtered = append(filtered, source)
	}
	return filtered
}

func sourceWithinAnyRoot(path string, roots []string) bool {
	path = filepath.Clean(path)
	for _, root := range roots {
		relative, err := filepath.Rel(root, path)
		if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func isGeneratedTreeSitterParser(source mermaidcode.Source) bool {
	return filepath.Base(source.Path) == "parser.c" &&
		strings.Contains(source.Text, "#define LANGUAGE_VERSION ") &&
		strings.Contains(source.Text, "#define STATE_COUNT ") &&
		strings.Contains(source.Text, "ts_lex_modes[") &&
		strings.Contains(source.Text, "const TSLanguage *")
}

type mermaidSchemaDiscovery struct {
	diagrams []string
}

func discoverMermaidSchemas(root string) ([]string, error) {
	discovery := mermaidSchemaDiscovery{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		return discovery.visit(root, path, entry, walkErr)
	})
	sort.Strings(discovery.diagrams)
	return discovery.diagrams, err
}

func (d *mermaidSchemaDiscovery) visit(root, path string, entry os.DirEntry, walkErr error) error {
	if walkErr != nil {
		return walkErr
	}
	if entry.IsDir() && path != root && mermaidExcludedDirectories[entry.Name()] {
		return filepath.SkipDir
	}
	if entry.Type()&os.ModeSymlink != 0 {
		if entry.IsDir() {
			return filepath.SkipDir
		}
		return nil
	}
	if entry.IsDir() {
		return nil
	}
	if isMermaidSchema(entry.Name()) {
		d.diagrams = append(d.diagrams, path)
	}
	return nil
}

func isMermaidSchema(name string) bool {
	return strings.HasSuffix(name, ".class.mmd") || strings.HasSuffix(name, ".structure.mmd") || strings.HasSuffix(name, ".flow.mmd")
}

func validateMermaidSchemas(paths []string, analysis *mermaidcode.Analysis) ([]Diagnostic, error) {
	var result []Diagnostic
	for _, path := range paths {
		diagnostics, err := validateMermaidSchema(path, analysis)
		if err != nil {
			return nil, err
		}
		result = append(result, diagnostics...)
	}
	sortMermaidDiagnostics(result)
	return result, nil
}

func sortMermaidDiagnostics(result []Diagnostic) {
	sort.Slice(result, func(left, right int) bool {
		leftPosition, rightPosition := result[left].Position.Start, result[right].Position.Start
		return leftPosition.Filename < rightPosition.Filename || leftPosition.Filename == rightPosition.Filename && leftPosition.Line < rightPosition.Line
	})
}

func validateMermaidSchema(path string, analysis *mermaidcode.Analysis) ([]Diagnostic, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read Mermaid schema %s: %w", path, err)
	}
	var mismatches []mermaidcode.Diagnostic
	if strings.HasSuffix(path, ".flow.mmd") {
		mismatches, err = mermaidcode.CheckFlowchartWithAnalysis(string(content), analysis)
	} else {
		mismatches, err = mermaidcode.CheckClassDiagramWithAnalysis(string(content), analysis)
	}
	if err != nil {
		return nil, fmt.Errorf("validate Mermaid schema %s: %w", path, err)
	}
	return mermaidDiagnostics(path, mismatches), nil
}

func mermaidDiagnostics(path string, mismatches []mermaidcode.Diagnostic) []Diagnostic {
	result := make([]Diagnostic, 0, len(mismatches))
	for _, mismatch := range mismatches {
		position := SourcePosition{Filename: path, Line: mismatch.Line, Column: 1}
		result = append(result, Diagnostic{
			Severity: "error", Failure: mismatch.Message, RuleName: mermaidCodeRule,
			Category: "architecture", Position: DiagnosticPosition{Start: position, End: position}, Confidence: 1,
		})
	}
	return result
}
