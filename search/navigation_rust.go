package search

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/greppleai/grepple/parser"
)

type rustModuleIdentity struct {
	root     string
	segments []string
}

type rustModuleQueueItem struct {
	file     string
	identity rustModuleIdentity
}

type rustModuleIndex struct {
	byFile   map[string][]rustModuleIdentity
	byModule map[string][]string
}

func (index *navigationIndex) rustImportTargetFiles(sourceFile, importPath string) []string {
	if index.rustModules == nil {
		index.rustModules = buildRustModuleIndex(index.graph, index.contents)
	}
	return index.rustModules.importTargetFiles(index.graph.Exports, sourceFile, importPath)
}

func buildRustModuleIndex(graph parser.NavigationGraph, contents map[string]string) *rustModuleIndex {
	result := &rustModuleIndex{byFile: make(map[string][]rustModuleIdentity), byModule: make(map[string][]string)}
	files := make(map[string]string, len(contents))
	roots := []string{}
	for path := range contents {
		clean := filepath.Clean(path)
		files[clean] = path
		if rustCrateRootPath(clean) {
			roots = append(roots, path)
		}
	}
	sort.Strings(roots)
	queue := make([]rustModuleQueueItem, 0, len(roots))
	for _, root := range roots {
		queue = append(queue, rustModuleQueueItem{file: root, identity: rustModuleIdentity{root: filepath.Clean(root)}})
	}
	moduleFacts := rustModuleFactsByFile(graph.Imports)
	seen := make(map[string]bool)
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		key := current.identity.root + "\x00" + filepath.Clean(current.file) + "\x00" + strings.Join(current.identity.segments, "::")
		if seen[key] {
			continue
		}
		seen[key] = true
		result.add(current.file, current.identity)
		for _, fact := range moduleFacts[filepath.Clean(current.file)] {
			for _, candidate := range rustModuleDeclarationTargets(current.file, fact.Alias, files) {
				segments := append(append([]string(nil), current.identity.segments...), fact.Alias)
				queue = append(queue, rustModuleQueueItem{file: candidate, identity: rustModuleIdentity{root: current.identity.root, segments: segments}})
			}
		}
	}
	return result
}

func (index *rustModuleIndex) add(file string, identity rustModuleIdentity) {
	clean := filepath.Clean(file)
	for _, existing := range index.byFile[clean] {
		if existing.root == identity.root && strings.Join(existing.segments, "::") == strings.Join(identity.segments, "::") {
			return
		}
	}
	identity.segments = append([]string(nil), identity.segments...)
	index.byFile[clean] = append(index.byFile[clean], identity)
	key := rustModuleKey(identity.root, identity.segments)
	index.byModule[key] = append(index.byModule[key], file)
	index.byModule[key] = compactSortedStrings(index.byModule[key])
}

func (index *rustModuleIndex) importTargetFiles(exports []parser.NavigationExport, sourceFile, importPath string) []string {
	matches := []string{}
	for _, source := range index.byFile[filepath.Clean(sourceFile)] {
		target, ok := rustResolveModulePath(source, importPath)
		if !ok {
			continue
		}
		matches = append(matches, index.byModule[rustModuleKey(source.root, target)]...)
		if len(target) == 0 {
			continue
		}
		module, member := target[:len(target)-1], target[len(target)-1]
		for _, file := range index.byModule[rustModuleKey(source.root, module)] {
			if rustFileExports(exports, file, member) {
				matches = append(matches, file)
			}
		}
	}
	return compactSortedStrings(matches)
}

func rustResolveModulePath(source rustModuleIdentity, importPath string) ([]string, bool) {
	if strings.HasPrefix(importPath, "::") {
		return nil, false
	}
	parts := strings.Split(strings.TrimSpace(importPath), "::")
	if len(parts) == 0 {
		return nil, false
	}
	var target []string
	switch parts[0] {
	case "crate":
		target = []string{}
		parts = parts[1:]
	case "self":
		target = append([]string(nil), source.segments...)
		parts = parts[1:]
	case "super":
		target = append([]string(nil), source.segments...)
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

func rustModuleFactsByFile(imports []parser.NavigationImport) map[string][]parser.NavigationImport {
	facts := make(map[string][]parser.NavigationImport)
	for _, item := range imports {
		if navigationLanguageFamily(item.Language) != "rust" || item.Alias == "" || item.Alias == "*" || item.Imported != "*" || item.ImportPath != "self::"+item.Alias {
			continue
		}
		path := filepath.Clean(item.Path)
		facts[path] = append(facts[path], item)
	}
	return facts
}

func rustModuleDeclarationTargets(sourceFile, name string, files map[string]string) []string {
	directory := filepath.Dir(sourceFile)
	base := strings.TrimSuffix(filepath.Base(sourceFile), filepath.Ext(sourceFile))
	if base != "lib" && base != "main" && base != "mod" {
		directory = filepath.Join(directory, base)
	}
	matches := []string{}
	for _, candidate := range []string{filepath.Join(directory, name+".rs"), filepath.Join(directory, name, "mod.rs")} {
		if original, ok := files[filepath.Clean(candidate)]; ok {
			matches = append(matches, original)
		}
	}
	return compactSortedStrings(matches)
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

func rustFileExports(exports []parser.NavigationExport, file, name string) bool {
	for _, item := range exports {
		if navigationLanguageFamily(item.Language) == "rust" && filepath.Clean(item.Path) == filepath.Clean(file) && (item.Name == name || item.Name == "*") {
			return true
		}
	}
	return false
}

func rustModuleKey(root string, segments []string) string {
	return filepath.Clean(root) + "\x00" + strings.Join(segments, "::")
}
