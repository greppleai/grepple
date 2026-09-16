package search

import (
	"path/filepath"
	"strings"
)

func pythonImportTargetFiles(files []string, sourceFile, importPath string) []string {
	importPath = strings.TrimSpace(importPath)
	if importPath == "" {
		return nil
	}
	relative := strings.HasPrefix(importPath, ".")
	target := ""
	if relative {
		target = pythonRelativeImportTarget(sourceFile, importPath)
	} else {
		target = filepath.FromSlash(strings.ReplaceAll(importPath, ".", "/"))
	}
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
