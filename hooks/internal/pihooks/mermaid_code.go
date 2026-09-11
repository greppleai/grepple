package pihooks

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	mermaidcode "github.com/greppleai/grepple/extract"
)

const mermaidCodeRule = "mermaid-code"

var mermaidExcludedDirectories = map[string]bool{
	".git": true, ".worktrees": true, "dist": true, "generated": true,
	"node_modules": true, "vendor": true,
}

// AnalyzeMermaidSchemas validates conventionally named Mermaid schemas and canonical
// package and workspace bundles under root. Source discovery is skipped when only bundles are present.
func AnalyzeMermaidSchemas(root string) ([]Diagnostic, error) {
	diagrams, packageBundles, workspaceBundles, err := discoverMermaidSchemas(root)
	if err != nil {
		return nil, err
	}
	result := validatePackageBundles(packageBundles)
	result = append(result, validateWorkspaceBundles(workspaceBundles)...)
	if len(diagrams) > 0 {
		sources, loadErr := mermaidcode.LoadSources([]string{root})
		if loadErr != nil {
			return nil, fmt.Errorf("load sources for Mermaid schemas: %w", loadErr)
		}
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

type mermaidSchemaDiscovery struct {
	diagrams         []string
	packageBundles   []string
	workspaceBundles []string
}

func discoverMermaidSchemas(root string) ([]string, []string, []string, error) {
	discovery := mermaidSchemaDiscovery{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		return discovery.visit(root, path, entry, walkErr)
	})
	sort.Strings(discovery.diagrams)
	sort.Strings(discovery.packageBundles)
	sort.Strings(discovery.workspaceBundles)
	return discovery.diagrams, discovery.packageBundles, discovery.workspaceBundles, err
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
		return d.addBundle(path, entry.Name())
	}
	if isMermaidSchema(entry.Name()) {
		d.diagrams = append(d.diagrams, path)
	}
	return nil
}

func (d *mermaidSchemaDiscovery) addBundle(path, name string) error {
	switch filepath.Ext(name) {
	case ".package":
		d.packageBundles = append(d.packageBundles, path)
		return filepath.SkipDir
	case ".workspace":
		d.workspaceBundles = append(d.workspaceBundles, path)
		return filepath.SkipDir
	default:
		return nil
	}
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

func validatePackageBundles(bundles []string) []Diagnostic {
	return validateCanonicalBundles(bundles, "package", mermaidcode.CheckPackageBundle)
}

func validateWorkspaceBundles(bundles []string) []Diagnostic {
	return validateCanonicalBundles(bundles, "workspace", mermaidcode.CheckWorkspaceBundle)
}

func validateCanonicalBundles(bundles []string, kind string, check func(string, string) error) []Diagnostic {
	result := make([]Diagnostic, 0, len(bundles))
	for _, bundle := range bundles {
		if err := check(bundle, ""); err != nil {
			message := strings.ReplaceAll(err.Error(), "\n", " ")
			const maximumMessageLength = 500
			if len(message) > maximumMessageLength {
				message = message[:maximumMessageLength] + "..."
			}
			path := filepath.Join(bundle, "manifest.json")
			position := SourcePosition{Filename: path, Line: 1, Column: 1}
			result = append(result, Diagnostic{
				Severity: "error", Failure: "canonical " + kind + " bundle mismatch: " + message, RuleName: mermaidCodeRule,
				Category: "architecture", Position: DiagnosticPosition{Start: position, End: position}, Confidence: 1,
			})
		}
	}
	return result
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
