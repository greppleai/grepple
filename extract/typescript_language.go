package extract

import (
	"path/filepath"
	"strings"
)

type typeScriptAnalysis struct {
	result *Analysis
}

func typeScriptLanguageDefinition() *languageDefinition {
	return &languageDefinition{
		info: Language{
			ID: "typescript", Extensions: parserLanguageExtensions("typescript", "tsx"),
			FocusedStructure: true, FocusedFlow: true,
		},
		flowIndex:  moduleFocusedFlowIndex{},
		classIndex: moduleFocusedClassIndex{},
		semantics: focusedLanguageSemantics{
			defaultModuleMetadata: true, moduleReferences: true, moduleExports: true,
		},
		acceptsSource: isTypeScriptSourceFile,
		newAnalysis: func(result *Analysis, sources []Source) languageAnalysis {
			prepareTypeScriptModules(result, sources)
			return &typeScriptAnalysis{result: result}
		},
		nearestProjectRoot: nearestTypeScriptRoot,
		sourceScope: func(source Source) (string, error) {
			return absolutePath(source.Path), nil
		},
		normalizeType:     normalizeTypeScript,
		generateStructure: generateModuleClass,
		generateFlow:      generateModuleFlowchart,
		validFlowEdge: func(analysis *Analysis, source, target *Symbol) bool {
			return hasModuleCallPath(analysis, source, target) || hasModuleOrderedPath(analysis, source, target)
		},
	}
}

func isTypeScriptSourceFile(path string) bool {
	lower := strings.ToLower(path)
	for _, extension := range typeScriptExtensions {
		if strings.HasSuffix(lower, extension) {
			return !isTypeScriptDeclaration(lower)
		}
	}
	return false
}

func (analysis *typeScriptAnalysis) Analyze(source Source) error {
	return analyzeECMAScriptSource(source, analysis.result)
}

func (analysis *typeScriptAnalysis) Finalize() error {
	resolveTypeScriptImports(analysis.result)
	finalizeTypeScriptIndexes(analysis.result)
	return nil
}

func nearestTypeScriptRoot(directory string) string {
	for current := directory; ; current = filepath.Dir(current) {
		if fileExists(filepath.Join(current, "package.json")) || fileExists(filepath.Join(current, "tsconfig.json")) {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			return ""
		}
	}
}
