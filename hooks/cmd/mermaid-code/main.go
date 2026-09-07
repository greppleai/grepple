// Command mermaid-code validates and generates Mermaid code diagrams.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"grepple/hooks/internal/mermaidcode"
)

func usage() {
	fmt.Fprintln(os.Stderr, "Usage:\n  mermaid-code check class|structure|flow <diagram.mmd> <source-file-or-directory> [...]\n  mermaid-code check package <bundle-directory> [source-directory]\n  mermaid-code check workspace <bundle-directory> [root]\n  mermaid-code generate workspace <root> --output <bundle-directory>\n  mermaid-code generate package <source-directory> [--output file]\n  mermaid-code generate package <source-directory> --format bundle --output <bundle-directory>\n  mermaid-code generate class|structure|flow <entry.go|entry.ts> <ClassName|function|Class.method> [--source <path>] [--depth N] [--max-nodes N] [--output file]")
}

func main() { os.Exit(run(os.Args[1:])) }

func run(arguments []string) int {
	if len(arguments) < 2 {
		usage()
		return 2
	}
	switch arguments[0] {
	case "check":
		return check(arguments[1:])
	case "generate":
		return generate(arguments[1:])
	default:
		usage()
		return 2
	}
}

func validMode(mode string) bool { return mode == "class" || mode == "structure" || mode == "flow" }

func isClassMode(mode string) bool { return mode == "class" || mode == "structure" }

func check(arguments []string) int {
	if len(arguments) > 0 && arguments[0] == "workspace" {
		return checkWorkspace(arguments[1:])
	}
	if (len(arguments) == 2 || len(arguments) == 3) && arguments[0] == "package" {
		source := ""
		if len(arguments) == 3 {
			source = absolute(arguments[2])
		}
		if err := mermaidcode.CheckPackageBundle(absolute(arguments[1]), source); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fmt.Println("Package bundle matches canonical generated artifacts.")
		return 0
	}
	if len(arguments) < 3 || !validMode(arguments[0]) {
		usage()
		return 2
	}
	diagnostics, paths, err := checkMode(arguments[0], arguments[1], arguments[2:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if len(diagnostics) == 0 {
		printCheckSuccess(arguments[0], len(paths))
		return 0
	}
	printDiagnostics(arguments[0], arguments[1], diagnostics)
	return 1
}

func checkWorkspace(arguments []string) int {
	if len(arguments) < 1 || len(arguments) > 2 {
		usage()
		return 2
	}
	root := ""
	if len(arguments) == 2 {
		root = absolute(arguments[1])
	}
	if err := mermaidcode.CheckWorkspaceBundle(absolute(arguments[0]), root); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println("Workspace bundle matches canonical generated artifacts.")
	return 0
}

func checkMode(mode, diagram string, sources []string) ([]mermaidcode.Diagnostic, []string, error) {
	if isClassMode(mode) {
		return mermaidcode.CheckClassPaths(diagram, sources)
	}
	return mermaidcode.CheckFlowPaths(diagram, sources)
}

func printCheckSuccess(mode string, count int) {
	if isClassMode(mode) {
		fmt.Printf("Class diagram matches; scanned %d source file(s).\n", count)
		return
	}
	fmt.Printf("Flowchart matches; scanned %d source file(s).\n", count)
}

func printDiagnostics(mode, path string, diagnostics []mermaidcode.Diagnostic) {
	for _, diagnostic := range diagnostics {
		fmt.Fprintf(os.Stderr, "%s:%d: %s\n", path, diagnostic.Line, diagnostic.Message)
	}
	kind := "flowchart"
	if isClassMode(mode) {
		kind = "class diagram"
	}
	fmt.Fprintf(os.Stderr, "%d %s mismatch(es).\n", len(diagnostics), kind)
}

type generateArguments struct {
	mode, format, entryFile, name, output string
	roots                                 []string
	options                               mermaidcode.GenerateOptions
}

func generate(arguments []string) int {
	if len(arguments) > 0 && arguments[0] == "workspace" {
		return generateWorkspace(arguments[1:])
	}
	if len(arguments) > 0 && arguments[0] == "package" {
		return generatePackage(arguments[1:])
	}
	options, err := parseGenerateArguments(arguments)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	sources, err := mermaidcode.LoadSources(options.roots)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	entry := findEntrySource(options.entryFile, sources)
	if entry == nil {
		fmt.Fprintf(os.Stderr, "Entry file is outside the supplied source paths: %s\n", options.entryFile)
		return 2
	}
	diagram, err := generateDiagram(options, *entry, sources)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	return writeDiagram(options, diagram)
}

func generateWorkspace(arguments []string) int {
	if len(arguments) != 3 || arguments[1] != "--output" || arguments[2] == "" {
		usage()
		return 2
	}
	root, output := absolute(arguments[0]), absolute(arguments[2])
	bundle, err := mermaidcode.GenerateWorkspaceBundle(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if err := mermaidcode.WriteWorkspaceBundle(output, bundle); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	fmt.Printf("Generated validated workspace bundle: %s\n", output)
	return 0
}

func generatePackage(arguments []string) int {
	if len(arguments) < 1 {
		usage()
		return 2
	}
	options := generateArguments{mode: "package", entryFile: absolute(arguments[0])}
	if err := parsePackageFlags(arguments[1:], &options); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if options.format == "bundle" {
		if options.output == "" {
			fmt.Fprintln(os.Stderr, "--output is required when --format bundle is used")
			return 2
		}
		bundle, err := mermaidcode.GeneratePackageBundle(options.entryFile)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
		if err := mermaidcode.WritePackageBundle(options.output, bundle); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
		fmt.Printf("Generated validated package bundle: %s\n", options.output)
		return 0
	}
	diagram, err := mermaidcode.GeneratePackageDiagram(options.entryFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	return writeDiagram(options, diagram)
}

func parsePackageFlags(flags []string, result *generateArguments) error {
	for index := 0; index < len(flags); index += 2 {
		if index+1 >= len(flags) {
			return fmt.Errorf("Missing value for %s", flags[index])
		}
		switch flags[index] {
		case "--output":
			result.output = absolute(flags[index+1])
		case "--format":
			if flags[index+1] != "bundle" {
				return fmt.Errorf("Unknown package format: %s", flags[index+1])
			}
			result.format = flags[index+1]
		default:
			return fmt.Errorf("Unknown package generation option: %s", flags[index])
		}
	}
	return nil
}

func parseGenerateArguments(arguments []string) (generateArguments, error) {
	if len(arguments) < 3 || !validMode(arguments[0]) {
		usage()
		return generateArguments{}, fmt.Errorf("invalid generate arguments")
	}
	result := generateArguments{mode: arguments[0], entryFile: absolute(arguments[1]), name: arguments[2]}
	if err := parseGenerateFlags(arguments[3:], &result); err != nil {
		return generateArguments{}, err
	}
	if len(result.roots) == 0 {
		result.roots = []string{filepath.Dir(result.entryFile)}
	}
	return result, nil
}

func parseGenerateFlags(flags []string, result *generateArguments) error {
	for index := 0; index < len(flags); index += 2 {
		if index+1 >= len(flags) {
			return fmt.Errorf("Missing value for %s", flags[index])
		}
		if err := applyGenerateFlag(flags[index], flags[index+1], result); err != nil {
			return err
		}
	}
	return nil
}

func applyGenerateFlag(flag, value string, result *generateArguments) error {
	switch flag {
	case "--source":
		result.roots = append(result.roots, value)
	case "--output":
		result.output = absolute(value)
	case "--depth":
		return setDepth(value, &result.options)
	case "--max-nodes":
		return setNodeLimit(value, &result.options)
	default:
		return fmt.Errorf("Unknown option: %s", flag)
	}
	return nil
}

func setDepth(value string, options *mermaidcode.GenerateOptions) error {
	depth, err := strconv.Atoi(value)
	if err != nil || depth < 0 {
		return fmt.Errorf("--depth must be an integer")
	}
	options.Depth, options.DepthSet = depth, true
	return nil
}

func setNodeLimit(value string, options *mermaidcode.GenerateOptions) error {
	nodeLimit, err := strconv.Atoi(value)
	if err != nil || nodeLimit < 1 {
		return fmt.Errorf("--max-nodes must be an integer")
	}
	options.MaxNodes = nodeLimit
	return nil
}

func findEntrySource(path string, sources []mermaidcode.Source) *mermaidcode.Source {
	for index := range sources {
		if absolute(sources[index].Path) == path {
			return &sources[index]
		}
	}
	return nil
}

func generateDiagram(options generateArguments, entry mermaidcode.Source, sources []mermaidcode.Source) (string, error) {
	if isClassMode(options.mode) {
		return mermaidcode.GenerateClassDiagram(options.name, entry, sources, options.options)
	}
	return mermaidcode.GenerateFlowchart(options.name, options.entryFile, sources, options.options)
}

func writeDiagram(options generateArguments, diagram string) int {
	if options.output == "" {
		fmt.Print(diagram)
		return 0
	}
	if err := os.MkdirAll(filepath.Dir(options.output), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if err := os.WriteFile(options.output, []byte(diagram), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	fmt.Printf("Generated validated %s diagram: %s\n", options.mode, options.output)
	return 0
}

func absolute(path string) string {
	result, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return result
}
