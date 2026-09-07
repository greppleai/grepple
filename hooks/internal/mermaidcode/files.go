package mermaidcode

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var excludedDirectories = map[string]bool{
	".git": true, ".grepple": true, ".worktrees": true, "dist": true, "generated": true,
	"node_modules": true, "vendor": true,
}

func isSourceFile(path string) bool {
	if strings.HasSuffix(path, ".go") {
		name := filepath.Base(path)
		return !strings.HasSuffix(name, "_test.go") && !strings.HasSuffix(name, "_generated.go") && !strings.HasSuffix(name, ".generated.go") && !strings.HasPrefix(name, "zz_generated")
	}
	for _, extension := range []string{".ts", ".tsx", ".mts", ".cts"} {
		if strings.HasSuffix(path, extension) && !isTypeScriptDeclaration(path) {
			return true
		}
	}
	return false
}

func isTypeScriptDeclaration(path string) bool {
	return strings.HasSuffix(path, ".d.ts") || strings.HasSuffix(path, ".d.mts") || strings.HasSuffix(path, ".d.cts")
}

type sourceCollector struct{ paths map[string]bool }

func (collector *sourceCollector) collect(path string, explicit bool) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return invalidSourceInput(path, explicit)
	}
	if info.IsDir() {
		return collector.collectDirectory(path)
	}
	if !info.Mode().IsRegular() || !isSourceFile(path) {
		return invalidCodeInput(path, explicit)
	}
	if isGeneratedGoFile(path) {
		return invalidGeneratedInput(path, explicit)
	}
	collector.paths[absolutePath(path)] = true
	return nil
}

func isGeneratedGoFile(path string) bool {
	if !strings.HasSuffix(path, ".go") {
		return false
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(content), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(line, "// Code generated ") && strings.Contains(line, " DO NOT EDIT.") {
			return true
		}
		if trimmed != "" && !strings.HasPrefix(trimmed, "//") {
			return false
		}
	}
	return false
}

func invalidGeneratedInput(path string, explicit bool) error {
	if explicit {
		return fmt.Errorf("Source input is a generated Go file: %s", path)
	}
	return nil
}

func invalidSourceInput(path string, explicit bool) error {
	if explicit {
		return fmt.Errorf("Source input is not a file or directory: %s", path)
	}
	return nil
}

func invalidCodeInput(path string, explicit bool) error {
	if explicit {
		return fmt.Errorf("Source input is not a Go or TypeScript source file: %s", path)
	}
	return nil
}

func (collector *sourceCollector) collectDirectory(path string) error {
	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	sort.Slice(entries, func(left, right int) bool { return entries[left].Name() < entries[right].Name() })
	for _, entry := range entries {
		if collector.shouldSkip(entry) {
			continue
		}
		if entry.IsDir() || isSourceFile(entry.Name()) {
			if err := collector.collect(filepath.Join(path, entry.Name()), false); err != nil {
				return err
			}
		}
	}
	return nil
}

func (collector *sourceCollector) shouldSkip(entry os.DirEntry) bool {
	return entry.Type()&os.ModeSymlink != 0 || entry.IsDir() && excludedDirectories[entry.Name()]
}

// DiscoverSources recursively finds supported Go or TypeScript source files.
func DiscoverSources(inputs []string) ([]string, error) {
	collector := sourceCollector{paths: map[string]bool{}}
	for _, input := range inputs {
		if err := collector.collect(absolutePath(input), true); err != nil {
			return nil, err
		}
	}
	paths := sortedKeys(collector.paths)
	if len(paths) == 0 {
		return nil, fmt.Errorf("No Go or TypeScript source files found in the supplied source paths")
	}
	return paths, nil
}

// LoadSources discovers and reads all Go and TypeScript sources under inputs.
func LoadSources(inputs []string) ([]Source, error) {
	paths, err := DiscoverSources(inputs)
	if err != nil {
		return nil, err
	}
	result := make([]Source, 0, len(paths))
	for _, path := range paths {
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil, readErr
		}
		result = append(result, Source{path, string(content)})
	}
	return result, nil
}

// CheckClassPaths reads a class diagram and recursively discovered sources.
func CheckClassPaths(diagramPath string, inputs []string) ([]Diagnostic, []string, error) {
	return checkPaths(diagramPath, inputs, CheckClassDiagram)
}

// CheckFlowPaths reads a flowchart and recursively discovered sources.
func CheckFlowPaths(diagramPath string, inputs []string) ([]Diagnostic, []string, error) {
	return checkPaths(diagramPath, inputs, CheckFlowchart)
}

func checkPaths(diagramPath string, inputs []string, check func(string, []Source) ([]Diagnostic, error)) ([]Diagnostic, []string, error) {
	content, err := os.ReadFile(diagramPath)
	if err != nil {
		return nil, nil, err
	}
	sources, err := LoadSources(inputs)
	if err != nil {
		return nil, nil, err
	}
	diagnostics, err := check(string(content), sources)
	paths := make([]string, 0, len(sources))
	for _, source := range sources {
		paths = append(paths, source.Path)
	}
	return diagnostics, paths, err
}
