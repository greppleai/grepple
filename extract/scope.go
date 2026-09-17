package extract

import (
	"os"
	"path/filepath"
	"strings"
)

var typeScriptExtensions = []string{".ts", ".tsx", ".mts", ".cts"}
var ecmaScriptExtensions = []string{".ts", ".tsx", ".mts", ".cts", ".js", ".jsx"}

func prepareTypeScriptModules(analysis *Analysis, sources []Source) {
	paths := moduleSourcePaths(sources)
	fallbackRoot := commonDirectory(paths)
	roots := map[string]bool{}
	for _, sourcePath := range paths {
		if root := nearestTypeScriptRoot(filepath.Dir(sourcePath)); root != "" {
			roots[root] = true
		}
	}
	for _, sourcePath := range paths {
		moduleID := absolutePath(sourcePath)
		root := nearestTypeScriptRoot(filepath.Dir(moduleID))
		if root == "" || len(roots) > 1 {
			root = fallbackRoot
		}
		stable := stableRelativePath(root, moduleID)
		analysis.ModulePaths[moduleID] = stable
		if old, ok := analysis.ModuleIndex[stable]; ok && old != moduleID {
			analysis.ModuleIndex[stable] = ""
		} else {
			analysis.ModuleIndex[stable] = moduleID
		}
	}
}

// prepareSourcePaths records the portable path used by file metadata. Like module
// metadata, it is relative to the nearest language project root, with a common-root
// fallback when sources span projects.
func prepareSourcePaths(analysis *Analysis, sources []Source) {
	pathsByLanguage := map[string][]string{}
	for _, source := range sources {
		language := languageForPath(source.Path)
		pathsByLanguage[language] = append(pathsByLanguage[language], absolutePath(source.Path))
	}
	for language, paths := range pathsByLanguage {
		fallbackRoot := commonDirectory(paths)
		roots := map[string]bool{}
		for _, sourcePath := range paths {
			if root := nearestSourceRoot(language, filepath.Dir(sourcePath)); root != "" {
				roots[root] = true
			}
		}
		for _, sourcePath := range paths {
			root := nearestSourceRoot(language, filepath.Dir(sourcePath))
			if root == "" || len(roots) > 1 {
				root = fallbackRoot
			}
			analysis.SourcePaths[sourcePath] = stableRelativePath(root, sourcePath)
		}
	}
}

func nearestSourceRoot(language, directory string) string {
	return adapterProjectRoot(language, directory)
}

func moduleSourcePaths(sources []Source) []string {
	result := []string{}
	for _, source := range sources {
		if language := languageForPath(source.Path); focusedSemanticsFor(language).moduleReferences {
			result = append(result, absolutePath(source.Path))
		}
	}
	return result
}

func fileExists(path string) bool { info, err := os.Stat(path); return err == nil && !info.IsDir() }

func commonDirectory(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	common := filepath.Dir(paths[0])
	for _, item := range paths[1:] {
		for !pathWithin(common, item) {
			parent := filepath.Dir(common)
			if parent == common {
				return common
			}
			common = parent
		}
	}
	return common
}

func pathWithin(root, item string) bool {
	relative, err := filepath.Rel(root, item)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func stableRelativePath(root, item string) string {
	if root != "" {
		if relative, err := filepath.Rel(root, item); err == nil && pathWithin(root, item) {
			return filepath.ToSlash(filepath.Clean(relative))
		}
	}
	return filepath.ToSlash(filepath.Clean(item))
}

func resolveTypeScriptImports(analysis *Analysis) {
	for moduleID, bindings := range analysis.ModuleImportBindings {
		for local, binding := range bindings {
			binding.ModuleID = resolveRelativeTypeScriptModule(moduleID, binding.Source, analysis)
			if binding.ModuleID != "" {
				exported := binding.Imported
				if binding.Default {
					exported = "default"
				}
				binding.Resolved = analysis.ModuleExportNames[binding.ModuleID][exported]
			}
			bindings[local] = binding
		}
	}
}

func resolveRelativeTypeScriptModule(owner, specifier string, analysis *Analysis) string {
	if !strings.HasPrefix(specifier, ".") {
		return ""
	}
	base := filepath.Clean(filepath.Join(filepath.Dir(owner), filepath.FromSlash(specifier)))
	candidates := []string{base}
	if filepath.Ext(base) == "" {
		candidates = candidates[:0]
		for _, extension := range ecmaScriptExtensions {
			candidates = append(candidates, base+extension)
		}
		for _, extension := range ecmaScriptExtensions {
			candidates = append(candidates, filepath.Join(base, "index"+extension))
		}
	}
	found := ""
	for _, candidate := range candidates {
		candidate = absolutePath(candidate)
		if _, exists := analysis.ModulePaths[candidate]; !exists {
			continue
		}
		if found != "" && found != candidate {
			analysis.duplicateErrors = append(analysis.duplicateErrors, "ambiguous ECMAScript import "+specifier+" from "+analysis.ModulePaths[owner])
			return ""
		}
		found = candidate
	}
	return found
}

func resolveModuleScope(scope string, analysis *Analysis) string {
	normalized := filepath.ToSlash(filepath.Clean(filepath.FromSlash(scope)))
	if moduleID, ok := analysis.ModuleIndex[normalized]; ok {
		return moduleID
	}
	found := ""
	for moduleID, stable := range analysis.ModulePaths {
		if stable == normalized || strings.HasSuffix(stable, "/"+normalized) {
			if found != "" && found != moduleID {
				return ""
			}
			found = moduleID
		}
	}
	return found
}

func mergeTypeScriptInterfaces(target, addition *Declaration) {
	target.Members = append(target.Members, addition.Members...)
	for name := range addition.Extends {
		target.Extends[name] = true
	}
	for name := range addition.Implements {
		target.Implements[name] = true
	}
}

func packageScopeMatches(scope string, declarationPackage, packageID string, analysis *Analysis) bool {
	if scope == "" {
		return true
	}
	if importPath := analysis.PackagePaths[packageID]; importPath != "" && scope == importPath {
		return true
	}
	if analysis.PackageFallbackScopes[packageID] == scope {
		return true
	}
	if scope != declarationPackage {
		return false
	}
	count := 0
	for _, name := range analysis.PackageNames {
		if name == scope {
			count++
		}
	}
	return count == 1
}

func packageDisplayScope(packageID, packageName string, analysis *Analysis) string {
	if importPath := analysis.PackagePaths[packageID]; importPath != "" {
		return importPath
	}
	count := 0
	for _, name := range analysis.PackageNames {
		if name == packageName {
			count++
		}
	}
	if count == 1 {
		return packageName
	}
	return analysis.PackageFallbackScopes[packageID]
}

func moduleScopeAmbiguous(scope string, analysis *Analysis) bool {
	normalized := filepath.ToSlash(filepath.Clean(filepath.FromSlash(scope)))
	if moduleID, ok := analysis.ModuleIndex[normalized]; ok && moduleID != "" {
		return false
	}
	count := 0
	for _, stable := range analysis.ModulePaths {
		if stable == normalized || strings.HasSuffix(stable, "/"+normalized) {
			count++
		}
	}
	return count > 1
}

func preparePackageFallbackScopes(analysis *Analysis, sources []Source) {
	paths := []string{}
	for _, source := range sources {
		if languageForPath(source.Path) == "go" {
			paths = append(paths, absolutePath(source.Path))
		}
	}
	root := commonDirectory(paths)
	for packageID, packageName := range analysis.PackageNames {
		directory := strings.TrimSuffix(packageID, ":"+packageName)
		analysis.PackageFallbackScopes[packageID] = stableRelativePath(root, directory)
	}
}

func structuralMembersMatch(required, actual Member) bool {
	if !membersMatch(required, actual) {
		return false
	}
	if focusedSemanticsFor(required.Language).restrictedStructuralMembers && required.Visibility != "public" {
		return required.PackageID == actual.PackageID
	}
	return true
}

func moduleReferenceName(owner, target *Declaration, analysis *Analysis) string {
	if owner == nil || target == nil || owner.Language != target.Language || !focusedSemanticsFor(owner.Language).moduleReferences {
		return ""
	}
	targetKey := target.ModuleID + ":" + target.Name
	for alias, key := range moduleDependencyCandidates(owner, analysis) {
		if key == targetKey {
			return alias
		}
	}
	return ""
}
