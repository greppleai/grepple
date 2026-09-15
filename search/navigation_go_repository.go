package search

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/greppleai/grepple/parser"
)

func goRepositoryContext(paths []string) ([]string, map[string]string, map[string][]string) {
	inputs := collectGoRepositoryInputs(paths)
	replacements := resolveGoRepositoryReplacements(inputs)
	for importPath := range replacements {
		inputs.importRoots[importPath] = true
	}
	return sortedGoRepositorySet(inputs.importRoots), replacements, inputs.packageFiles
}

type goRepositoryInputs struct {
	goPaths        []string
	configPaths    map[string]bool
	ownershipRoots map[string]bool
	importRoots    map[string]bool
	packageFiles   map[string][]string
}

func collectGoRepositoryInputs(paths []string) goRepositoryInputs {
	inputs := goRepositoryInputs{configPaths: make(map[string]bool), ownershipRoots: make(map[string]bool), importRoots: make(map[string]bool), packageFiles: make(map[string][]string)}
	for _, sourcePath := range paths {
		if parser.LanguageFor(sourcePath) != "go" {
			continue
		}
		inputs.goPaths = append(inputs.goPaths, sourcePath)
		if moduleRoot, moduleID, ok := goModuleForFile(sourcePath); ok {
			inputs.importRoots[moduleID] = true
			inputs.ownershipRoots[moduleRoot] = true
			inputs.configPaths[filepath.Join(moduleRoot, "go.mod")] = true
			packageID := moduleID
			if relative, err := filepath.Rel(moduleRoot, filepath.Dir(sourcePath)); err == nil && relative != "." {
				packageID += "/" + filepath.ToSlash(relative)
			}
			inputs.packageFiles[packageID] = append(inputs.packageFiles[packageID], sourcePath)
		}
		if workPath, ok := nearestGoRepositoryFile(filepath.Dir(sourcePath), "go.work"); ok {
			inputs.configPaths[workPath] = true
			inputs.ownershipRoots[filepath.Dir(workPath)] = true
		}
	}
	return inputs
}

func resolveGoRepositoryReplacements(inputs goRepositoryInputs) map[string]string {
	replacements := make(map[string]string)
	conflicts := make(map[string]bool)
	for _, configPath := range sortedGoRepositorySet(inputs.configPaths) {
		for importPath, target := range localGoReplacements(configPath) {
			if !ownedGoReplacementTarget(target, inputs.goPaths, inputs.ownershipRoots) {
				continue
			}
			if previous, exists := replacements[importPath]; exists && previous != target {
				conflicts[importPath] = true
				delete(replacements, importPath)
				continue
			}
			if !conflicts[importPath] {
				replacements[importPath] = target
			}
		}
	}
	return replacements
}

func sortedGoRepositorySet(values map[string]bool) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

// NavigationRepositoryContextFiles returns sorted Go module/workspace files that affect repository identity.
func NavigationRepositoryContextFiles(paths []string) []string {
	configs := make(map[string]bool)
	for _, sourcePath := range paths {
		if parser.LanguageFor(sourcePath) != "go" {
			continue
		}
		if moduleRoot, _, ok := goModuleForFile(sourcePath); ok {
			configs[filepath.Join(moduleRoot, "go.mod")] = true
		}
		if workPath, ok := nearestGoRepositoryFile(filepath.Dir(sourcePath), "go.work"); ok {
			configs[workPath] = true
		}
	}
	result := make([]string, 0, len(configs))
	for path := range configs {
		result = append(result, path)
	}
	sort.Strings(result)
	return result
}

func nearestGoRepositoryFile(directory, name string) (string, bool) {
	for {
		path := filepath.Join(directory, name)
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
			return path, true
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return "", false
		}
		directory = parent
	}
}

func localGoReplacements(configPath string) map[string]string {
	content, err := os.ReadFile(configPath)
	if err != nil {
		return nil
	}
	result := make(map[string]string)
	for _, rawLine := range strings.Split(string(content), "\n") {
		line := strings.TrimSpace(strings.SplitN(rawLine, "//", 2)[0])
		if strings.HasPrefix(line, "replace ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "replace "))
		}
		parts := strings.SplitN(line, "=>", 2)
		if len(parts) != 2 {
			continue
		}
		left, right := strings.Fields(parts[0]), strings.Fields(parts[1])
		if len(left) == 0 || len(right) == 0 || !localGoReplacementPath(right[0]) {
			continue
		}
		target := right[0]
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(configPath), filepath.FromSlash(target))
		}
		result[left[0]] = filepath.Clean(target)
	}
	return result
}

func localGoReplacementPath(path string) bool {
	return filepath.IsAbs(path) || strings.HasPrefix(path, "./") || strings.HasPrefix(path, "../")
}

func ownedGoReplacementTarget(directory string, paths []string, ownershipRoots map[string]bool) bool {
	if info, err := os.Stat(directory); err != nil || !info.IsDir() {
		return false
	}
	if selectedGoTarget(directory, paths) {
		return true
	}
	for root := range ownershipRoots {
		relative, err := filepath.Rel(root, directory)
		if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func selectedGoTarget(directory string, paths []string) bool {
	for _, sourcePath := range paths {
		relative, err := filepath.Rel(directory, sourcePath)
		if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func longestGoReplacementPrefix(importPath string, replacements map[string]string) (string, string) {
	best := ""
	for candidate := range replacements {
		if (importPath == candidate || strings.HasPrefix(importPath, candidate+"/")) && len(candidate) > len(best) {
			best = candidate
		}
	}
	return best, replacements[best]
}
