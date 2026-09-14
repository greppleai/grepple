package extract

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/greppleai/grepple/internal/pathfilter"
)

var excludedDirectories = map[string]bool{
	".git": true, ".grepple": true, ".worktrees": true, "dist": true, "generated": true,
	"node_modules": true, "vendor": true,
}

func isSourceFile(path string) bool {
	definition, ok := languageDefinitionForPath(path)
	return ok && definition.acceptsSource(path)
}

func isTypeScriptDeclaration(path string) bool {
	return strings.HasSuffix(path, ".d.ts") || strings.HasSuffix(path, ".d.mts") || strings.HasSuffix(path, ".d.cts")
}

// DiscoveryOptions applies repository-relative exclusions during recursive discovery.
// Explicitly supplied files bypass IgnorePaths.
type DiscoveryOptions struct {
	IgnoreRoot  string
	IgnorePaths []string
}

type sourceCollector struct {
	paths  map[string]bool
	ignore pathfilter.Config
}

func (collector *sourceCollector) collect(path string, explicit bool) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return invalidSourceInput(path, explicit)
	}
	if info.IsDir() {
		if !explicit && collector.ignore.Ignored(path) && !collector.ignore.HasNegation() {
			return nil
		}
		return collector.collectDirectory(path)
	}
	if !explicit && collector.ignore.Ignored(path) {
		return nil
	}
	definition, supported := languageDefinitionForPath(path)
	if !info.Mode().IsRegular() || !supported || !definition.acceptsSource(path) {
		return invalidCodeInput(path, explicit)
	}
	if definition.acceptsInput != nil {
		accepted, err := definition.acceptsInput(path, explicit)
		if err != nil {
			return err
		}
		if !accepted {
			return nil
		}
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
		return fmt.Errorf("Source input is not a supported source file (%s): %s", supportedSourceDescription(), path)
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

// DiscoverSources recursively finds files supported by the registered language adapters.
func DiscoverSources(inputs []string) ([]string, error) {
	return DiscoverSourcesWithOptions(inputs, DiscoveryOptions{})
}

// DiscoverSourcesWithOptions discovers sources with repository-owned exclusions.
func DiscoverSourcesWithOptions(inputs []string, options DiscoveryOptions) ([]string, error) {
	collector := sourceCollector{paths: map[string]bool{}, ignore: pathfilter.Config{Root: options.IgnoreRoot, Patterns: options.IgnorePaths}}
	for _, input := range inputs {
		if err := collector.collect(absolutePath(input), true); err != nil {
			return nil, err
		}
	}
	paths := sortedKeys(collector.paths)
	if len(paths) == 0 {
		return nil, fmt.Errorf("no supported source files found in the supplied paths (%s)", supportedSourceDescription())
	}
	return paths, nil
}

// LoadSources discovers and reads sources supported by the registered language adapters.
func LoadSources(inputs []string) ([]Source, error) {
	return LoadSourcesWithOptions(inputs, DiscoveryOptions{})
}

// LoadSourcesWithOptions discovers and reads sources with repository-owned exclusions.
func LoadSourcesWithOptions(inputs []string, options DiscoveryOptions) ([]Source, error) {
	paths, err := DiscoverSourcesWithOptions(inputs, options)
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
