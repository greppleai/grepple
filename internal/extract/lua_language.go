package extract

import (
	"path/filepath"
	"strings"

	codeparser "github.com/greppleai/grepple/internal/parser"
)

type luaAnalysis struct{ result *Analysis }

func luaLanguageDefinition() *languageDefinition {
	return &languageDefinition{
		info:      Language{ID: "lua", Extensions: parserLanguageExtensions("lua"), FocusedFlow: true},
		flowIndex: moduleFocusedFlowIndex{}, classIndex: moduleFocusedClassIndex{},
		acceptsSource:      func(path string) bool { return strings.EqualFold(filepath.Ext(path), ".lua") },
		newAnalysis:        func(result *Analysis, _ []Source) languageAnalysis { return &luaAnalysis{result: result} },
		nearestProjectRoot: func(directory string) string { return directory },
		sourceScope:        func(source Source) (string, error) { return absolutePath(source.Path), nil },
		normalizeType:      normalizeTypeScript,
		generateStructure:  generateModuleClass, generateFlow: generateModuleFlowchart,
		validFlowEdge: func(analysis *Analysis, source, target *Symbol) bool {
			return hasModuleCallPath(analysis, source, target) || hasModuleOrderedPath(analysis, source, target)
		},
	}
}

func (analysis *luaAnalysis) Analyze(source Source) error {
	document, err := parseSource(source)
	if err != nil {
		return err
	}
	defer document.Close()
	moduleID := absolutePath(source.Path)
	analysis.result.ModulePaths[moduleID] = analysis.result.SourcePaths[moduleID]
	analyzer := sourceAnalyzer{result: analysis.result, source: source, text: []byte(source.Text), moduleID: moduleID, language: "lua"}
	if err := document.Read(func(view codeparser.DocumentView) error {
		for _, statement := range view.Root().NamedChildren() {
			if statement.Kind() == "function_declaration" {
				analyzer.analyzeFunction(statement, false, false)
			}
			if statement.Kind() == "variable_declaration" {
				for _, child := range statement.NamedChildren() {
					if child.Kind() == "function_declaration" {
						analyzer.analyzeFunction(child, false, false)
					}
				}
			}
		}
		return nil
	}); err != nil {
		return err
	}
	analysis.result.Navigation.Merge(codeparser.NewParser().NavigationGraph(document, source.Path))
	return nil
}

func (*luaAnalysis) Finalize() error { return nil }
