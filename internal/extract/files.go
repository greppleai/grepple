package extract

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/greppleai/grepple/internal/pathfilter"
	sourcedomain "github.com/greppleai/grepple/internal/sources"
)

var excludedDirectories = map[string]bool{
	".git": true, ".grepple": true, ".worktrees": true, "dist": true, "generated": true,
	"node_modules": true, "vendor": true,
}

func isTypeScriptDeclaration(path string) bool {
	return strings.HasSuffix(path, ".d.ts") || strings.HasSuffix(path, ".d.mts") || strings.HasSuffix(path, ".d.cts")
}

// DiscoveryOptions applies repository-relative exclusions during recursive discovery.
// Explicitly supplied files bypass IgnorePaths.
type DiscoveryOptions struct {
	IgnoreRoot     string
	IgnorePaths    []string
	ProductionOnly bool
}

type sourceCollector struct {
	paths          map[string]bool
	ignore         pathfilter.Config
	productionOnly bool
}

func (collector *sourceCollector) collect(path string, explicit bool) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return invalidSourceInput(path, explicit)
	}
	if collector.unconditionallyExcluded(path) {
		if explicit {
			return fmt.Errorf("source input is unconditionally excluded: %s", path)
		}
		return nil
	}
	if info.IsDir() {
		return invalidSourceInput(path, explicit)
	}
	if collector.ignoredFile(path, explicit) {
		return nil
	}
	return collector.collectFile(path, info, explicit)
}

func (collector *sourceCollector) unconditionallyExcluded(candidate string) bool {
	relative, ok := collector.ignore.Relative(candidate)
	if !ok {
		return false
	}
	return pathfilter.Match(".git/**", relative) || pathfilter.Match(".grepple/**", relative) || pathfilter.Match(".worktrees/**", relative)
}

func (collector *sourceCollector) ignoredFile(path string, explicit bool) bool {
	return !explicit && (collector.ignore.Ignored(path) || collector.productionOnly && !sourcedomain.IsProduction(path, collector.ignore.Root))
}

func (collector *sourceCollector) collectFile(path string, info os.FileInfo, explicit bool) error {
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

// DiscoverSources recursively finds files supported by the registered language adapters.
func DiscoverSources(inputs []string) ([]string, error) {
	return DiscoverSourcesWithOptions(inputs, DiscoveryOptions{})
}

// DiscoverSourcesWithOptions discovers sources with repository-owned exclusions.
func DiscoverSourcesWithOptions(inputs []string, options DiscoveryOptions) ([]string, error) {
	collector := sourceCollector{paths: map[string]bool{}, ignore: pathfilter.Config{Root: options.IgnoreRoot, Patterns: options.IgnorePaths}, productionOnly: options.ProductionOnly}
	for _, input := range inputs {
		path := absolutePath(input)
		info, err := os.Lstat(path)
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, invalidSourceInput(path, true)
		}
		if !info.IsDir() {
			if err := collector.collect(path, true); err != nil {
				return nil, err
			}
			continue
		}
		candidates, err := sourcedomain.Candidates(context.Background(), []string{path}, sourcedomain.DiscoveryOptions{IgnoreRoot: options.IgnoreRoot, IgnorePaths: options.IgnorePaths, ProductionOnly: options.ProductionOnly})
		if err != nil {
			return nil, err
		}
		for _, candidate := range candidates {
			if containsExcludedDirectory(path, candidate) {
				continue
			}
			if err := collector.collect(candidate, false); err != nil {
				return nil, err
			}
		}
	}
	paths := sortedKeys(collector.paths)
	if len(paths) == 0 {
		return nil, fmt.Errorf("no supported source files found in the supplied paths (%s)", supportedSourceDescription())
	}
	return paths, nil
}

func containsExcludedDirectory(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	parts := strings.Split(filepath.Clean(relative), string(filepath.Separator))
	for _, part := range parts[:len(parts)-1] {
		if excludedDirectories[part] {
			return true
		}
	}
	return false
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
