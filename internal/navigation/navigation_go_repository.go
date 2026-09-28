package navigation

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/greppleai/grepple/internal/parser"
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
		if parser.NewParser().LanguageFor(sourcePath) != "go" {
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

// repositoryContextFiles returns sorted Go module/workspace files that affect repository identity.
func repositoryContextFiles(paths []string) []string {
	configs := make(map[string]bool)
	for _, sourcePath := range paths {
		if parser.NewParser().LanguageFor(sourcePath) != "go" {
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

func localGoImportDirectory(sourceFile, importPath string) (string, bool) {
	root, module, ok := goModuleForFile(sourceFile)
	return goImportDirectory(root, module, ok, importPath)
}

func goImportDirectory(root, module string, known bool, importPath string) (string, bool) {
	if !known {
		return "", false
	}
	if importPath == module {
		return root, true
	}
	if strings.HasPrefix(importPath, module+"/") {
		relative := strings.TrimPrefix(importPath, module+"/")
		return filepath.Clean(filepath.Join(root, filepath.FromSlash(relative))), true
	}
	return "", true
}

// enrichNavigationRepositoryIdentity applies path-dependent Go module and package
// identity after path-neutral per-file facts have been loaded from cache.
func enrichNavigationRepositoryIdentity(graph *parser.NavigationGraph, sourcePath string) {
	if graph == nil || parser.NewParser().LanguageFor(sourcePath) != "go" {
		return
	}
	moduleRoot, moduleID, ok := goModuleForFile(sourcePath)
	if !ok {
		return
	}
	packageID := moduleID
	if relative, err := filepath.Rel(moduleRoot, filepath.Dir(sourcePath)); err == nil && relative != "." {
		packageID += "/" + filepath.ToSlash(relative)
	}
	for index := range graph.Declarations {
		graph.Declarations[index].ModuleID = moduleID
		graph.Declarations[index].PackageID = packageID
	}
	for index := range graph.TypeDeclarations {
		graph.TypeDeclarations[index].ModuleID = moduleID
		graph.TypeDeclarations[index].PackageID = packageID
	}
}

func goModuleForFile(sourcePath string) (root, moduleID string, ok bool) {
	directory := filepath.Dir(sourcePath)
	for {
		if modulePath, found := goModulePath(filepath.Join(directory, "go.mod")); found {
			return filepath.Clean(directory), modulePath, true
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return "", "", false
		}
		directory = parent
	}
}

func goModulePath(path string) (string, bool) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	for _, line := range strings.Split(string(content), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "module" {
			return strings.TrimSpace(fields[1]), true
		}
	}
	return "", false
}

type goNavigationIndex struct {
	baseLanguageNavigationIndex
	replacements map[string]string
	packages     map[string][]string
	modules      map[string]goSourceModule
}

type goSourceModule struct {
	root, name string
	known      bool
}

// Local import checks revisit the same source for many candidate declarations.
// Keep filesystem-derived module identity scoped to this one graph build.
func (index *goNavigationIndex) localImportDirectory(sourceFile, importPath string) (string, bool) {
	if index.modules == nil {
		index.modules = make(map[string]goSourceModule)
	}
	module, found := index.modules[sourceFile]
	if !found {
		module.root, module.name, module.known = goModuleForFile(sourceFile)
		index.modules[sourceFile] = module
	}
	return goImportDirectory(module.root, module.name, module.known, importPath)
}

func (index *goNavigationIndex) importTargets(request navigationImportRequest) navigationImportTargets {
	sourceFile, importPath := request.sourceFile, request.importPath
	targets := append([]string(nil), index.packages[importPath]...)
	root, directory := longestGoReplacementPrefix(importPath, index.replacements)
	if root != "" {
		relative := strings.TrimPrefix(strings.TrimPrefix(importPath, root), "/")
		targets = append(targets, index.filesInDirectory(filepath.Join(directory, filepath.FromSlash(relative)))...)
	}
	if len(targets) == 0 {
		if directory, known := index.localImportDirectory(sourceFile, importPath); known {
			targets = append(targets, index.filesInDirectory(directory)...)
		}
	}
	sort.Strings(targets)
	return navigationImportTargets{files: compactSortedStrings(targets)}
}

func (index *goNavigationIndex) filesInDirectory(directory string) []string {
	if directory == "" {
		return nil
	}
	return index.corpus.pathIndex().byDirectory[filepath.Clean(directory)]
}

func (index *goNavigationIndex) filterCandidates(call navigationCall, candidates []navigationDeclaration) []navigationDeclaration {
	if call.importPath == "" {
		return candidates
	}
	return filterNavigationCandidates(candidates, func(candidate navigationDeclaration) bool {
		return index.importMatches(call, candidate)
	})
}

func (index *goNavigationIndex) importMatches(call navigationCall, candidate navigationDeclaration) bool {
	if directory, known := index.localImportDirectory(call.importSourceFile, call.importPath); known && directory != "" {
		return filepath.Clean(filepath.Dir(candidate.file)) == directory
	}
	if index.baseLanguageNavigationIndex.importMatches(call, candidate) {
		return true
	}
	return candidate.packageName == goImportPackageName(call.importPath)
}

func goImportPackageName(importPath string) string {
	name := filepath.Base(filepath.FromSlash(importPath))
	if len(name) > 1 && name[0] == 'v' {
		if major, err := strconv.Atoi(name[1:]); err == nil && major >= 2 {
			return filepath.Base(filepath.Dir(filepath.FromSlash(importPath)))
		}
	}
	return name
}

func (*goNavigationIndex) fieldOriginMatches(candidate, owner navigationField) bool {
	return candidate.packageName == owner.packageName && filepath.Clean(filepath.Dir(candidate.file)) == filepath.Clean(filepath.Dir(owner.file))
}
