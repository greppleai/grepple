package parser

import (
	"path/filepath"
	"sort"
	"strings"
)

// RustModuleTarget identifies one syntax-evidenced module in a selected crate.
// ModulePath is crate-relative and LocalScope is the inline-module path within Path.
type RustModuleTarget struct {
	Path       string
	CrateRoot  string
	ModulePath string
	LocalScope string
}

type rustModuleQueueItem = RustModuleTarget

// RustModuleIndex resolves Rust module paths only through selected crate roots,
// module declarations, and explicit path attributes recorded in graph.
type RustModuleIndex struct {
	byFile   map[string][]RustModuleTarget
	byModule map[string][]RustModuleTarget
	exports  []NavigationExport
}

// BuildRustModuleIndex constructs a reusable module index without reparsing source.
func BuildRustModuleIndex(graph NavigationGraph, paths []string) *RustModuleIndex {
	index := &RustModuleIndex{byFile: make(map[string][]RustModuleTarget), byModule: make(map[string][]RustModuleTarget), exports: graph.Exports}
	files, queue := rustModuleSourceFiles(paths)
	moduleFacts := rustModuleFactsByFile(graph.Imports)
	seen := make(map[string]bool)
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		key := RustModuleTargetKey(current)
		if seen[key] {
			continue
		}
		seen[key] = true
		index.add(current)
		queue = append(queue, rustModuleChildren(current, moduleFacts[filepath.Clean(current.Path)], files)...)
	}
	return index
}

func rustModuleSourceFiles(paths []string) (map[string]string, []rustModuleQueueItem) {
	files := make(map[string]string, len(paths))
	roots := []string{}
	for _, path := range paths {
		clean := filepath.Clean(path)
		files[clean] = path
		if rustCrateRootPath(clean) {
			roots = append(roots, path)
		}
	}
	sort.Strings(roots)
	queue := make([]rustModuleQueueItem, 0, len(roots))
	for _, root := range roots {
		queue = append(queue, RustModuleTarget{Path: root, CrateRoot: filepath.Clean(root)})
	}
	return files, queue
}

func rustModuleChildren(current RustModuleTarget, facts []NavigationImport, files map[string]string) []rustModuleQueueItem {
	children := []rustModuleQueueItem{}
	for _, fact := range facts {
		if fact.Scope != current.LocalScope {
			continue
		}
		modulePath := rustJoinModulePath(current.ModulePath, fact.Alias)
		if fact.Inline {
			children = append(children, RustModuleTarget{Path: current.Path, CrateRoot: current.CrateRoot, ModulePath: modulePath, LocalScope: rustJoinModulePath(current.LocalScope, fact.Alias)})
			continue
		}
		for _, candidate := range rustModuleDeclarationTargets(current, fact.Alias, fact.TargetPathHint, files) {
			children = append(children, RustModuleTarget{Path: candidate, CrateRoot: current.CrateRoot, ModulePath: modulePath})
		}
	}
	return children
}

func (index *RustModuleIndex) add(target RustModuleTarget) {
	clean := filepath.Clean(target.Path)
	for _, existing := range index.byFile[clean] {
		if RustModuleTargetKey(existing) == RustModuleTargetKey(target) {
			return
		}
	}
	index.byFile[clean] = append(index.byFile[clean], target)
	key := RustModuleTargetModuleKey(target)
	index.byModule[key] = append(index.byModule[key], target)
	sort.Slice(index.byModule[key], func(i, j int) bool {
		return RustModuleTargetKey(index.byModule[key][i]) < RustModuleTargetKey(index.byModule[key][j])
	})
}

// ResolveImport returns modules directly named by importPath or containing its
// final exported item. Bare external-crate paths remain unresolved.
func (index *RustModuleIndex) ResolveImport(sourceFile, importPath string) []RustModuleTarget {
	matches := []RustModuleTarget{}
	for _, source := range index.byFile[filepath.Clean(sourceFile)] {
		if source.LocalScope != "" {
			continue
		}
		targetPath, ok := rustResolveModulePath(source, importPath)
		if !ok {
			continue
		}
		matches = append(matches, index.byModule[rustModuleKey(source.CrateRoot, targetPath)]...)
		if len(targetPath) == 0 {
			continue
		}
		module, member := targetPath[:len(targetPath)-1], targetPath[len(targetPath)-1]
		for _, candidate := range index.byModule[rustModuleKey(source.CrateRoot, module)] {
			if rustTargetExports(index.exports, candidate, member) {
				matches = append(matches, candidate)
			}
		}
	}
	return compactRustModuleTargets(matches)
}

// ResolveItemModules resolves the module portion of an item path without
// requiring the item to be publicly exported. It is intended for same-crate
// ownership checks such as explicitly qualified impl targets.
func (index *RustModuleIndex) ResolveItemModules(sourceFile, itemPath string) []RustModuleTarget {
	matches := []RustModuleTarget{}
	for _, source := range index.byFile[filepath.Clean(sourceFile)] {
		if source.LocalScope != "" {
			continue
		}
		targetPath, ok := rustResolveModulePath(source, itemPath)
		if !ok || len(targetPath) == 0 {
			continue
		}
		matches = append(matches, index.byModule[rustModuleKey(source.CrateRoot, targetPath[:len(targetPath)-1])]...)
	}
	return compactRustModuleTargets(matches)
}

// ModuleKeys returns absolute crate/module identities for a declaration scope.
func (index *RustModuleIndex) ModuleKeys(path, localScope string) []string {
	keys := []string{}
	for _, target := range index.byFile[filepath.Clean(path)] {
		if target.LocalScope == localScope {
			keys = append(keys, RustModuleTargetModuleKey(target))
		}
	}
	sort.Strings(keys)
	return compactRustStrings(keys)
}

func rustResolveModulePath(source RustModuleTarget, importPath string) ([]string, bool) {
	if strings.HasPrefix(importPath, "::") {
		return nil, false
	}
	parts := strings.Split(strings.TrimSpace(importPath), "::")
	if len(parts) == 0 {
		return nil, false
	}
	target := rustModuleSegments(source.ModulePath)
	switch parts[0] {
	case "crate":
		target = nil
		parts = parts[1:]
	case "self":
		parts = parts[1:]
	case "super":
		for len(parts) > 0 && parts[0] == "super" {
			if len(target) == 0 {
				return nil, false
			}
			target = target[:len(target)-1]
			parts = parts[1:]
		}
	default:
		return nil, false
	}
	for _, part := range parts {
		if part == "" || part == "self" || part == "super" || part == "crate" {
			return nil, false
		}
		target = append(target, part)
	}
	return target, true
}

func rustModuleFactsByFile(imports []NavigationImport) map[string][]NavigationImport {
	facts := make(map[string][]NavigationImport)
	for _, item := range imports {
		if item.Language != "rust" || item.Kind != "module" || item.Alias == "" {
			continue
		}
		path := filepath.Clean(item.Path)
		facts[path] = append(facts[path], item)
	}
	return facts
}

func rustModuleDeclarationTargets(source RustModuleTarget, name, targetPathHint string, files map[string]string) []string {
	directory := rustModuleDirectory(source.Path)
	if source.LocalScope != "" {
		directory = filepath.Join(directory, filepath.FromSlash(strings.ReplaceAll(source.LocalScope, "::", "/")))
	}
	candidates := []string{}
	if targetPathHint != "" && !filepath.IsAbs(filepath.FromSlash(targetPathHint)) {
		candidates = append(candidates, filepath.Join(directory, filepath.FromSlash(targetPathHint)))
	} else if targetPathHint == "" {
		candidates = append(candidates, filepath.Join(directory, name+".rs"), filepath.Join(directory, name, "mod.rs"))
	}
	matches := []string{}
	for _, candidate := range candidates {
		if original, ok := files[filepath.Clean(candidate)]; ok {
			matches = append(matches, original)
		}
	}
	sort.Strings(matches)
	return compactRustStrings(matches)
}

func rustModuleDirectory(sourceFile string) string {
	directory := filepath.Dir(sourceFile)
	base := strings.TrimSuffix(filepath.Base(sourceFile), filepath.Ext(sourceFile))
	if base != "lib" && base != "main" && base != "mod" {
		directory = filepath.Join(directory, base)
	}
	return directory
}

func rustCrateRootPath(path string) bool {
	if strings.ToLower(filepath.Ext(path)) != ".rs" {
		return false
	}
	base := filepath.Base(path)
	parent := filepath.Base(filepath.Dir(path))
	if (base == "lib.rs" || base == "main.rs") && parent == "src" {
		return true
	}
	if filepath.Base(filepath.Dir(filepath.Dir(path))) == "src" && parent == "bin" {
		return true
	}
	return base == "main.rs" && filepath.Base(filepath.Dir(filepath.Dir(path))) == "bin" && filepath.Base(filepath.Dir(filepath.Dir(filepath.Dir(path)))) == "src"
}

func rustTargetExports(exports []NavigationExport, target RustModuleTarget, name string) bool {
	for _, item := range exports {
		if item.Language == "rust" && filepath.Clean(item.Path) == filepath.Clean(target.Path) && item.Scope == target.LocalScope && (item.Name == name || item.Name == "*") {
			return true
		}
	}
	return false
}

// RustModuleTargetModuleKey returns a stable selected-crate module identity.
func RustModuleTargetModuleKey(target RustModuleTarget) string {
	return rustModuleKey(target.CrateRoot, rustModuleSegments(target.ModulePath))
}

// RustModuleTargetKey returns a stable identity including physical and inline scope.
func RustModuleTargetKey(target RustModuleTarget) string {
	return filepath.Clean(target.Path) + "\x00" + RustModuleTargetModuleKey(target) + "\x00" + target.LocalScope
}

func rustModuleKey(root string, segments []string) string {
	return filepath.Clean(root) + "\x00" + strings.Join(segments, "::")
}

func rustModuleSegments(path string) []string {
	if path == "" {
		return nil
	}
	return strings.Split(path, "::")
}

func rustJoinModulePath(prefix, name string) string {
	if prefix == "" {
		return name
	}
	return prefix + "::" + name
}

func compactRustModuleTargets(targets []RustModuleTarget) []RustModuleTarget {
	sort.Slice(targets, func(i, j int) bool { return RustModuleTargetKey(targets[i]) < RustModuleTargetKey(targets[j]) })
	result := targets[:0]
	last := ""
	for _, target := range targets {
		key := RustModuleTargetKey(target)
		if len(result) == 0 || key != last {
			result = append(result, target)
			last = key
		}
	}
	return result
}

func compactRustStrings(values []string) []string {
	result := values[:0]
	for _, value := range values {
		if len(result) == 0 || result[len(result)-1] != value {
			result = append(result, value)
		}
	}
	return result
}
