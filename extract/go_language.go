package extract

import (
	"path/filepath"
	"strings"

	codeparser "github.com/greppleai/grepple/parser"
)

type goAnalysis struct {
	result  *Analysis
	sources []Source
	methods map[string][]Member
}

func goLanguageDefinition() *languageDefinition {
	return &languageDefinition{
		info:          Language{ID: "go", Extensions: parserLanguageExtensions("go")},
		acceptsSource: isGoSourceFile,
		acceptsInput: func(path string, explicit bool) (bool, error) {
			if !isGeneratedGoFile(path) {
				return true, nil
			}
			return false, invalidGeneratedInput(path, explicit)
		},
		newAnalysis: func(result *Analysis, sources []Source) languageAnalysis {
			return &goAnalysis{result: result, sources: sources, methods: map[string][]Member{}}
		},
		nearestProjectRoot: nearestGoRoot,
		sourceScope:        goSourceScope,
		normalizeType:      normalizeGoType,
		generateStructure:  generateGoClass,
		generateFlow:       generateGoFlowchart,
		validFlowEdge: func(analysis *Analysis, source, target *Symbol) bool {
			return hasGoCallPath(analysis, source, target) || hasGoOrderedPhasePath(analysis, source, target)
		},
	}
}

func isGoSourceFile(path string) bool {
	if !strings.HasSuffix(strings.ToLower(path), ".go") {
		return false
	}
	name := filepath.Base(path)
	return !strings.HasSuffix(name, "_test.go") &&
		!strings.HasSuffix(name, "_generated.go") &&
		!strings.HasSuffix(name, ".generated.go") &&
		!strings.HasPrefix(name, "zz_generated")
}

func goSourceScope(source Source) (string, error) {
	document, err := codeparser.ParseDocument("go", source.Text)
	if err != nil {
		return "", err
	}
	defer document.Close()
	root := document.Root()
	if root.HasError() {
		return "", malformedSourceError(source.Path, root)
	}
	name := goPackageName(root, []byte(source.Text))
	return filepath.Clean(absolutePath(filepath.Dir(source.Path))) + ":" + name, nil
}

func (analysis *goAnalysis) Analyze(source Source) error {
	return analyzeGoSource(source, analysis.result, analysis.methods)
}

func (analysis *goAnalysis) Finalize() error {
	prepareGoFallbackScopes(analysis.result, analysis.sources)
	mergeGoMethods(analysis.result, analysis.methods)
	expandGoEmbeddedMembers(analysis.result)
	finalizeGoIndexes(analysis.result)
	return nil
}

func nearestGoRoot(directory string) string {
	for current := directory; ; current = filepath.Dir(current) {
		if fileExists(filepath.Join(current, "go.mod")) {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			return ""
		}
	}
}
