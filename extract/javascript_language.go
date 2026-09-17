package extract

import (
	"path/filepath"
	"strings"
)

type javaScriptAnalysis struct {
	result *Analysis
}

func javaScriptLanguageDefinition() *languageDefinition {
	return &languageDefinition{
		info: Language{
			ID: "javascript", Extensions: parserLanguageExtensions("javascript"),
			FocusedStructure: true, FocusedFlow: true,
		},
		flowIndex:  moduleFocusedFlowIndex{},
		classIndex: moduleFocusedClassIndex{},
		semantics: focusedLanguageSemantics{
			moduleReferences: true, moduleExports: true,
		},
		acceptsSource: isJavaScriptSourceFile,
		newAnalysis: func(result *Analysis, sources []Source) languageAnalysis {
			prepareTypeScriptModules(result, sources)
			return &javaScriptAnalysis{result: result}
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

func isJavaScriptSourceFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".js", ".jsx":
		return true
	default:
		return false
	}
}

func (analysis *javaScriptAnalysis) Analyze(source Source) error {
	return analyzeECMAScriptSource(source, analysis.result)
}

func (*javaScriptAnalysis) Finalize() error { return nil }
