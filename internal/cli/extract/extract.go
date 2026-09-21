// Package extract implements focused Mermaid extraction commands.
package extract

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/alexflint/go-arg"
	codeextract "github.com/greppleai/grepple/extract"
	cliruntime "github.com/greppleai/grepple/internal/cliruntime"
	codeparser "github.com/greppleai/grepple/parser"
)

type extractArgs struct {
	Entry    string   `arg:"--entry" placeholder:"SYMBOL" help:"entry type, function, or method"`
	At       string   `arg:"--at" placeholder:"PATH:LINE" help:"infer the entry declaration from a source location"`
	Source   []string `arg:"--source,separate" placeholder:"PATH" help:"source root; repeatable"`
	Depth    int      `arg:"--depth" default:"3" placeholder:"N" help:"dependency or call depth"`
	MaxNodes int      `arg:"--max-nodes" default:"200" placeholder:"N" help:"maximum diagram nodes"`
	Output   string   `arg:"--output" placeholder:"PATH" help:"write output instead of stdout"`
	Paths    []string `arg:"positional" placeholder:"PATH" help:"source file or directory"`
}

func (extractArgs) Description() string {
	return "Extract a validated, source-linked Mermaid structure or call-flow diagram from local supported code; remote selectors are not supported."
}

const extractHelp = `Extract source-backed focused architecture projections.

Usage:
  grepple extract structure (--entry SYMBOL | --at PATH:LINE) [OPTIONS] [PATH ...]
  grepple extract flow (--entry SYMBOL | --at PATH:LINE) [OPTIONS] [PATH ...]
  grepple extract check <structure|flow> TARGET [SOURCE ...]

Modes:
  structure   Generate a focused type structure diagram
  flow        Generate a focused callable flow diagram
  check       Validate a focused diagram against source

Availability: local checkout only; remote selectors are not supported.

Use grepple architecture directory|resolve|why for language-neutral repository orientation.
Run grepple help extract MODE for mode-specific help.
`

func isExtractHelp(value string) bool {
	return value == "--help" || value == "-h" || value == "help"
}

func writeExtractHelp() error {
	return cliruntime.NewOutput(os.Stdout).WriteString(extractHelp)
}

func writeExtractCheckHelp(mode string) error {
	switch mode {
	case "":
		return cliruntime.NewOutput(os.Stdout).WriteString("Validate generated architecture against source.\nUsage: grepple extract check <structure|flow> TARGET [SOURCE ...]\n")
	case "structure", "flow":
		return cliruntime.NewOutput(os.Stdout).WriteString(fmt.Sprintf("Validate a generated %s diagram.\nUsage: grepple extract check %s TARGET [SOURCE ...]\n", mode, mode))
	default:
		return fmt.Errorf("unknown extract check mode %q", mode)
	}
}

// Run executes the extract command family. Deprecated: construct the command with New.
func Run(args []string, dependencies Dependencies) error { return New(dependencies).Run(args) }

// Run executes the extract command family.
func (command *command) Run(args []string) error {
	if len(args) == 0 {
		return extractUsageError()
	}
	if isExtractHelp(args[0]) {
		return writeExtractHelp()
	}
	if args[0] == "check" {
		return command.runCheck(args[1:])
	}
	if args[0] != "structure" && args[0] != "flow" {
		return extractUsageError()
	}
	mode, values, err := parseExtractArgs(args)
	if err != nil || values == nil {
		return err
	}
	if err := validateExtractArgs(values); err != nil {
		return err
	}
	if mode == "structure" {
		return command.runStructure(values)
	}
	return command.runFlow(values)
}

func (command *command) runCheck(args []string) error { return runExtractCheck(args) }
func (command *command) runStructure(values *extractArgs) error {
	return runExtractStructure(values, command.dependencies)
}
func (command *command) runFlow(values *extractArgs) error {
	return runExtractFlow(values, command.dependencies)
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

func validateExtractArgs(values *extractArgs) error {
	if values.Depth < 0 {
		return fmt.Errorf("--depth must not be negative")
	}
	if values.MaxNodes < 1 {
		return fmt.Errorf("--max-nodes must be positive")
	}
	if values.Entry != "" && values.At != "" {
		return fmt.Errorf("--entry and --at cannot be used together")
	}
	return nil
}

func runExtractStructure(values *extractArgs, dependencies Dependencies) error {
	if values.Entry == "" && values.At == "" {
		return fmt.Errorf("extract structure requires --entry SYMBOL or --at PATH:LINE; use grepple architecture directory for repository orientation")
	}
	return runFocusedStructure(values, dependencies)
}

func runFocusedStructure(values *extractArgs, dependencies Dependencies) error {
	entryFile, entry, roots, err := resolveExtractEntry(values, true, dependencies)
	if err != nil {
		return err
	}
	if separator := strings.Index(entry, "."); separator > 0 {
		entry = entry[:separator]
	}
	sources, err := dependencies.loadSources(roots)
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
	return writeExtractOutput(values.Output, diagram, dependencies)
}

func runExtractFlow(values *extractArgs, dependencies Dependencies) error {
	if values.Entry == "" && values.At == "" {
		return fmt.Errorf("extract flow requires --entry SYMBOL or --at PATH:LINE")
	}
	entryFile, entry, roots, err := resolveExtractEntry(values, false, dependencies)
	if err != nil {
		return err
	}
	sources, err := dependencies.loadSources(roots)
	if err != nil {
		return err
	}
	diagram, err := codeextract.GenerateFlowchart(entry, entryFile, sources, extractGenerateOptions(values))
	if err != nil {
		return err
	}
	return writeExtractOutput(values.Output, diagram, dependencies)
}

func resolveExtractEntry(values *extractArgs, allowStructure bool, dependencies Dependencies) (string, string, []string, error) {
	roots := append([]string(nil), values.Source...)
	if values.At != "" {
		path, line, err := ParseAt(values.At)
		if err != nil {
			return "", "", nil, err
		}
		entry, err := declarationAt(path, line, allowStructure)
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
	sources, err := dependencies.loadSources(roots)
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

func declarationAt(path string, line int, allowStructure bool) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	language := codeparser.LanguageFor(path)
	if language == "" {
		return "", fmt.Errorf("extract does not support the source language for %s", path)
	}
	graph, _, _, err := codeparser.CachedNavigationGraph(string(content), language, path)
	if err != nil {
		return "", fmt.Errorf("parse %s: %w", path, err)
	}
	for _, declaration := range graph.Declarations {
		if line >= declaration.Start && line <= declaration.End {
			return declaration.Name, nil
		}
	}
	if allowStructure {
		if name := structureDeclarationAt(codeparser.OutlineFile(path, string(content)).Symbols, line); name != "" {
			return name, nil
		}
	}
	return "", fmt.Errorf("no callable declaration contains %s:%d", path, line)
}

func structureDeclarationAt(symbols []codeparser.Symbol, line int) string {
	for _, symbol := range symbols {
		if line < symbol.Start || line > symbol.End {
			continue
		}
		if nested := structureDeclarationAt(symbol.Children, line); nested != "" {
			return nested
		}
		switch symbol.Kind {
		case "class", "interface", "struct", "type", "alias", "enum", "record", "object", "trait":
			return symbol.Name
		}
	}
	return ""
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

func extractSourceByPath(sources []codeextract.Source, path string) *codeextract.Source {
	path = absoluteExtractPath(path)
	for index := range sources {
		if absoluteExtractPath(sources[index].Path) == path {
			return &sources[index]
		}
	}
	return nil
}

// ParseAt parses one PATH:LINE source selector.
func ParseAt(value string) (string, int, error) {
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

func writeExtractOutput(path, content string, dependencies Dependencies) error {
	if path == "" {
		return cliruntime.NewOutput(dependencies.stdout()).WriteString(content)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func runExtractCheck(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: grepple extract check <structure|flow> TARGET [SOURCE ...]")
	}
	if isExtractHelp(args[0]) {
		return writeExtractCheckHelp("")
	}
	if len(args) == 2 && isExtractHelp(args[1]) {
		return writeExtractCheckHelp(args[0])
	}
	if len(args) < 2 {
		return fmt.Errorf("usage: grepple extract check <structure|flow> TARGET [SOURCE ...]")
	}
	mode, target := args[0], args[1]
	sources := args[2:]
	switch mode {
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

func absoluteExtractPath(path string) string {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return filepath.Clean(absolute)
}
