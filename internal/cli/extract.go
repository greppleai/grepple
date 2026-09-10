package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/alexflint/go-arg"
	codeextract "github.com/greppleai/grepple/extract"
	codeparser "github.com/greppleai/grepple/parser"
)

type extractArgs struct {
	Entry     string   `arg:"--entry" placeholder:"SYMBOL" help:"entry type, function, or method"`
	At        string   `arg:"--at" placeholder:"PATH:LINE" help:"infer the entry declaration from a source location"`
	Source    []string `arg:"--source,separate" placeholder:"PATH" help:"source root; repeatable"`
	Depth     int      `arg:"--depth" default:"3" placeholder:"N" help:"dependency or call depth"`
	MaxNodes  int      `arg:"--max-nodes" default:"200" placeholder:"N" help:"maximum diagram nodes"`
	Output    string   `arg:"--output" placeholder:"PATH" help:"write output instead of stdout"`
	Bundle    bool     `arg:"--bundle" help:"structure: generate a canonical package bundle"`
	Workspace bool     `arg:"--workspace" help:"structure: generate a canonical workspace bundle"`
	Paths     []string `arg:"positional" placeholder:"PATH" help:"source file or directory"`
}

func (extractArgs) Description() string {
	return "Extract a validated, source-linked Mermaid structure or call-flow diagram from supported code."
}

func runExtract(args []string) error {
	if len(args) == 0 {
		return extractUsageError()
	}
	if args[0] == "check" {
		return runExtractCheck(args[1:])
	}
	if args[0] != "structure" && args[0] != "flow" {
		return extractUsageError()
	}
	mode, values, err := parseExtractArgs(args)
	if err != nil || values == nil {
		return err
	}
	if err := validateExtractArgs(mode, values); err != nil {
		return err
	}
	if mode == "structure" {
		return runExtractStructure(values)
	}
	return runExtractFlow(values)
}

func extractUsageError() error {
	return fmt.Errorf("usage: grepple extract <structure|flow|check> [flags] [PATH ...]")
}

func parseExtractArgs(args []string) (string, *extractArgs, error) {
	mode := args[0]
	values := &extractArgs{Depth: 3, MaxNodes: 200}
	parser, err := arg.NewParser(arg.Config{Program: "grepple extract " + mode}, values)
	if err != nil {
		return "", nil, err
	}
	if err := parser.Parse(args[1:]); err != nil {
		if errors.Is(err, arg.ErrHelp) {
			parser.WriteHelp(os.Stdout)
			return mode, nil, nil
		}
		return "", nil, err
	}
	return mode, values, nil
}

func validateExtractArgs(mode string, values *extractArgs) error {
	if values.Depth < 0 {
		return fmt.Errorf("--depth must not be negative")
	}
	if values.MaxNodes < 1 {
		return fmt.Errorf("--max-nodes must be positive")
	}
	if values.Entry != "" && values.At != "" {
		return fmt.Errorf("--entry and --at cannot be used together")
	}
	if mode == "flow" && (values.Bundle || values.Workspace) {
		return fmt.Errorf("--bundle and --workspace apply only to extract structure")
	}
	if values.Bundle && values.Workspace {
		return fmt.Errorf("--bundle and --workspace cannot be used together")
	}
	return nil
}

func runExtractStructure(values *extractArgs) error {
	path := firstExtractPath(values.Paths)
	if values.Workspace {
		bundle, err := codeextract.GenerateWorkspaceBundle(path)
		if err != nil {
			return err
		}
		if values.Output == "" {
			return fmt.Errorf("--output is required with --workspace")
		}
		return codeextract.WriteWorkspaceBundle(values.Output, bundle)
	}
	if values.Bundle {
		bundle, err := codeextract.GeneratePackageBundle(packageDirectory(path))
		if err != nil {
			return err
		}
		if values.Output == "" {
			return fmt.Errorf("--output is required with --bundle")
		}
		return codeextract.WritePackageBundle(values.Output, bundle)
	}
	if values.Entry != "" || values.At != "" {
		return runFocusedStructure(values)
	}
	diagram, err := codeextract.GeneratePackageDiagram(packageDirectory(path))
	if err != nil {
		return err
	}
	return writeExtractOutput(values.Output, diagram)
}

func runFocusedStructure(values *extractArgs) error {
	entryFile, entry, roots, err := resolveExtractEntry(values)
	if err != nil {
		return err
	}
	if separator := strings.Index(entry, "."); separator > 0 {
		entry = entry[:separator]
	}
	sources, err := loadExtractSources(roots)
	if err != nil {
		return err
	}
	source := extractSourceByPath(sources, entryFile)
	if source == nil {
		return fmt.Errorf("entry file is outside supplied source roots: %s", entryFile)
	}
	diagram, err := codeextract.GenerateClassDiagram(entry, *source, sources, extractGenerateOptions(values))
	if err != nil {
		return err
	}
	return writeExtractOutput(values.Output, diagram)
}

func runExtractFlow(values *extractArgs) error {
	if values.Entry == "" && values.At == "" {
		return fmt.Errorf("extract flow requires --entry SYMBOL or --at PATH:LINE")
	}
	entryFile, entry, roots, err := resolveExtractEntry(values)
	if err != nil {
		return err
	}
	sources, err := loadExtractSources(roots)
	if err != nil {
		return err
	}
	diagram, err := codeextract.GenerateFlowchart(entry, entryFile, sources, extractGenerateOptions(values))
	if err != nil {
		return err
	}
	return writeExtractOutput(values.Output, diagram)
}

func resolveExtractEntry(values *extractArgs) (string, string, []string, error) {
	roots := append([]string(nil), values.Source...)
	if values.At != "" {
		path, line, err := extractAt(values.At)
		if err != nil {
			return "", "", nil, err
		}
		entry, err := callableAt(path, line)
		if err != nil {
			return "", "", nil, err
		}
		if len(roots) == 0 {
			roots = defaultExtractRoots(values.Paths, path)
		}
		return absoluteExtractPath(path), entry, roots, nil
	}
	if len(roots) == 0 {
		roots = defaultExtractRoots(values.Paths, "")
	}
	sources, err := loadExtractSources(roots)
	if err != nil {
		return "", "", nil, err
	}
	path, err := sourcePathForSymbol(sources, values.Entry)
	return path, values.Entry, roots, err
}

func sourcePathForSymbol(sources []codeextract.Source, name string) (string, error) {
	analysis, err := codeextract.Analyze(sources)
	if err != nil {
		return "", err
	}
	symbol := analysis.Symbols[name]
	if symbol == nil {
		return "", fmt.Errorf("entry symbol %q was not found or is ambiguous; use --at PATH:LINE", name)
	}
	paths := []string{}
	for _, location := range symbol.Locations {
		paths = append(paths, location.Path)
	}
	if len(paths) == 0 {
		return "", fmt.Errorf("entry symbol %q was not found", name)
	}
	if len(paths) > 1 {
		return "", fmt.Errorf("entry symbol %q is ambiguous; use --at PATH:LINE", name)
	}
	return absoluteExtractPath(paths[0]), nil
}

func callableAt(path string, line int) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	language := codeparser.LanguageFor(path)
	if language == "" {
		return "", fmt.Errorf("extract does not support the source language for %s", path)
	}
	declarations, _ := codeparser.Navigation(string(content), language)
	for _, declaration := range declarations {
		if line >= declaration.Start && line <= declaration.End {
			return declaration.Name, nil
		}
	}
	return "", fmt.Errorf("no callable declaration contains %s:%d", path, line)
}

func loadExtractSources(roots []string) ([]codeextract.Source, error) {
	return codeextract.LoadSources(roots)
}

func extractGenerateOptions(values *extractArgs) codeextract.GenerateOptions {
	return codeextract.GenerateOptions{Depth: values.Depth, DepthSet: true, MaxNodes: values.MaxNodes}
}

func defaultExtractRoots(paths []string, entryFile string) []string {
	if len(paths) > 0 {
		return paths
	}
	if entryFile != "" {
		return []string{filepath.Dir(entryFile)}
	}
	return []string{"."}
}

func firstExtractPath(paths []string) string {
	if len(paths) == 0 {
		return "."
	}
	return paths[0]
}

func packageDirectory(path string) string {
	info, err := os.Stat(path)
	if err == nil && !info.IsDir() {
		return filepath.Dir(path)
	}
	return path
}

func extractSourceByPath(sources []codeextract.Source, path string) *codeextract.Source {
	path = absoluteExtractPath(path)
	for index := range sources {
		if absoluteExtractPath(sources[index].Path) == path {
			return &sources[index]
		}
	}
	return nil
}

func extractAt(value string) (string, int, error) {
	index := strings.LastIndex(value, ":")
	if index < 1 {
		return "", 0, fmt.Errorf("--at must be PATH:LINE")
	}
	line, err := strconv.Atoi(value[index+1:])
	if err != nil || line < 1 {
		return "", 0, fmt.Errorf("--at line must be positive")
	}
	return value[:index], line, nil
}

func writeExtractOutput(path, content string) error {
	if path == "" {
		return stdoutWriter().writeString(content)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func runExtractCheck(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: grepple extract check <structure|flow|package|workspace> TARGET [SOURCE ...]")
	}
	mode, target := args[0], args[1]
	sources := args[2:]
	switch mode {
	case "package":
		return codeextract.CheckPackageBundle(target, optionalExtractSource(sources))
	case "workspace":
		return codeextract.CheckWorkspaceBundle(target, optionalExtractSource(sources))
	case "structure":
		return checkExtractDiagram(target, sources, true)
	case "flow":
		return checkExtractDiagram(target, sources, false)
	default:
		return fmt.Errorf("unknown extract check mode %q", mode)
	}
}

func checkExtractDiagram(path string, sources []string, structure bool) error {
	var diagnostics []codeextract.Diagnostic
	var err error
	if structure {
		diagnostics, _, err = codeextract.CheckClassPaths(path, sources)
	} else {
		diagnostics, _, err = codeextract.CheckFlowPaths(path, sources)
	}
	if err != nil {
		return err
	}
	if len(diagnostics) == 0 {
		return nil
	}
	return fmt.Errorf("%s:%d: %s (%d mismatch(es))", path, diagnostics[0].Line, diagnostics[0].Message, len(diagnostics))
}

func optionalExtractSource(sources []string) string {
	if len(sources) == 0 {
		return ""
	}
	return sources[0]
}

func absoluteExtractPath(path string) string {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return filepath.Clean(absolute)
}
