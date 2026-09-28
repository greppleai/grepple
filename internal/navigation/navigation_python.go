package navigation

import (
	"path/filepath"
	"strings"
)

func pythonImportTargetFiles(files []string, sourceFile, importPath string) []string {
	target, relative := pythonImportTarget(sourceFile, importPath)
	if target == "" {
		return nil
	}
	matches := []string{}
	for _, candidate := range files {
		module, ok := pythonModuleFilePath(candidate)
		if !ok {
			continue
		}
		if pythonImportModuleMatches(module, target, sourceFile, relative) {
			matches = append(matches, candidate)
		}
	}
	return compactSortedStrings(matches)
}

func pythonImportTarget(sourceFile, importPath string) (string, bool) {
	importPath = strings.TrimSpace(importPath)
	if importPath == "" {
		return "", false
	}
	if strings.HasPrefix(importPath, ".") {
		return pythonRelativeImportTarget(sourceFile, importPath), true
	}
	return filepath.FromSlash(strings.ReplaceAll(importPath, ".", "/")), false
}

func pythonRelativeImportTarget(sourceFile, importPath string) string {
	level := 0
	for level < len(importPath) && importPath[level] == '.' {
		level++
	}
	base := filepath.Dir(sourceFile)
	for parent := 1; parent < level; parent++ {
		base = filepath.Dir(base)
	}
	remainder := strings.TrimPrefix(importPath[level:], ".")
	if remainder == "" {
		return filepath.Clean(base)
	}
	return filepath.Clean(filepath.Join(base, filepath.FromSlash(strings.ReplaceAll(remainder, ".", "/"))))
}

func pythonModuleFilePath(path string) (string, bool) {
	extension := strings.ToLower(filepath.Ext(path))
	if extension != ".py" && extension != ".pyi" && extension != ".pyw" {
		return "", false
	}
	module := strings.TrimSuffix(filepath.Clean(path), filepath.Ext(path))
	if filepath.Base(module) == "__init__" {
		module = filepath.Dir(module)
	}
	return module, true
}

func pythonImportModuleMatches(module, target, sourceFile string, relative bool) bool {
	module, target = filepath.Clean(module), filepath.Clean(target)
	if relative {
		return module == target
	}
	suffix := string(filepath.Separator) + target
	root := "."
	if module != target {
		if !strings.HasSuffix(module, suffix) {
			return false
		}
		root = strings.TrimSuffix(module, suffix)
	}
	relativeSource, err := filepath.Rel(root, filepath.Clean(sourceFile))
	return err == nil && relativeSource != ".." && !strings.HasPrefix(relativeSource, ".."+string(filepath.Separator))
}

type pythonNavigationIndex struct{ baseLanguageNavigationIndex }

func (index *pythonNavigationIndex) importTargets(request navigationImportRequest) navigationImportTargets {
	sourceFile, importPath := request.sourceFile, request.importPath
	target, relative := pythonImportTarget(sourceFile, importPath)
	if target == "" {
		return navigationImportTargets{}
	}
	paths := index.corpus.pathIndex()
	if relative {
		return navigationImportTargets{files: compactSortedStrings(append([]string(nil), paths.pythonExact[filepath.Clean(target)]...))}
	}
	matches := []string{}
	for _, candidate := range paths.pythonSuffixes[filepath.Clean(target)] {
		module, _ := pythonModuleFilePath(candidate)
		if pythonImportModuleMatches(module, target, sourceFile, false) {
			matches = append(matches, candidate)
		}
	}
	return navigationImportTargets{files: compactSortedStrings(matches)}
}
