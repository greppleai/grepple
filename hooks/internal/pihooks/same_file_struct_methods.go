// Package pihooks contains repository-specific checks used by the pi hooks.
package pihooks

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	codeparser "github.com/greppleai/grepple/parser"
)

const sameFileStructMethodsRule = "same-file-struct-methods"

var skippedDirectories = map[string]struct{}{
	"vendor":       {},
	"examples":     {},
	"testdata":     {},
	"node_modules": {},
	".git":         {},
}

// Diagnostic has the same shape as a diagnostic emitted by revive's JSON
// formatter. Keeping this shape lets project checks share the revive feedback
// and grouping pipeline.
type Diagnostic struct {
	Severity        string             `json:"Severity"`
	Failure         string             `json:"Failure"`
	RuleName        string             `json:"RuleName"`
	Category        string             `json:"Category"`
	Position        DiagnosticPosition `json:"Position"`
	Confidence      float64            `json:"Confidence"`
	ReplacementLine string             `json:"ReplacementLine"`
}

// DiagnosticPosition is the source range attached to a Diagnostic.
type DiagnosticPosition struct {
	Start SourcePosition `json:"Start"`
	End   SourcePosition `json:"End"`
}

// SourcePosition mirrors the position fields in a revive diagnostic.
type SourcePosition struct {
	Filename string `json:"Filename"`
	Offset   int    `json:"Offset"`
	Line     int    `json:"Line"`
	Column   int    `json:"Column"`
}

type structDeclaration struct {
	file string
}

type methodDeclaration struct {
	typeName   string
	methodName string
	file       string
	line       int
}

type packageDeclarations struct {
	structs map[string]structDeclaration
	methods []methodDeclaration
}

// AnalyzeRepository reports methods whose receiver struct is declared in a
// different Go file in the same directory. Unreadable directories are skipped,
// matching the hook's best-effort repository traversal; file read and parser
// setup failures are returned to the caller.
func AnalyzeRepository(root string) ([]Diagnostic, error) {
	filesByDirectory, directories := groupGoFilesByDirectory(goFiles(root))
	diagnostics := make([]Diagnostic, 0)
	for _, directory := range directories {
		found, err := analyzeDirectory(filesByDirectory[directory])
		if err != nil {
			return nil, err
		}
		diagnostics = append(diagnostics, found...)
	}
	return diagnostics, nil
}

func groupGoFilesByDirectory(files []string) (map[string][]string, []string) {
	filesByDirectory := make(map[string][]string)
	var directories []string
	for _, file := range files {
		directory := filepath.Dir(file)
		if _, exists := filesByDirectory[directory]; !exists {
			directories = append(directories, directory)
		}
		filesByDirectory[directory] = append(filesByDirectory[directory], file)
	}
	sort.Strings(directories)
	return filesByDirectory, directories
}

func analyzeDirectory(files []string) ([]Diagnostic, error) {
	packages := make(map[string]*packageDeclarations)
	for _, file := range files {
		if err := collectFileDeclarations(file, packages); err != nil {
			return nil, err
		}
	}
	var diagnostics []Diagnostic
	for _, declarations := range packages {
		for _, method := range declarations.methods {
			declaration, exists := declarations.structs[method.typeName]
			if exists && declaration.file != "" && declaration.file != method.file {
				diagnostics = append(diagnostics, sameFileDiagnostic(method, declaration))
			}
		}
	}
	sort.Slice(diagnostics, func(i, j int) bool {
		left, right := diagnostics[i].Position.Start, diagnostics[j].Position.Start
		return left.Filename < right.Filename || left.Filename == right.Filename && left.Line < right.Line
	})
	return diagnostics, nil
}

func collectFileDeclarations(file string, packages map[string]*packageDeclarations) error {
	content, err := os.ReadFile(file)
	if err != nil {
		return fmt.Errorf("read %s: %w", file, err)
	}
	document, err := codeparser.ParseDocument("go", string(content))
	if err != nil {
		return fmt.Errorf("parse %s: %w", file, err)
	}
	defer document.Close()
	root := document.Root()
	packageName := goPackageName(root, content)
	if packageName == "" {
		return nil
	}
	declarations := packages[packageName]
	if declarations == nil {
		declarations = &packageDeclarations{structs: make(map[string]structDeclaration)}
		packages[packageName] = declarations
	}
	collectGoDeclarations(root, content, file, declarations.structs, &declarations.methods)
	return nil
}

func goPackageName(root codeparser.Node, content []byte) string {
	for _, clause := range root.NamedChildren() {
		if clause.Kind() != "package_clause" || clause.NamedChildCount() == 0 {
			continue
		}
		return nodeContent(clause.NamedChild(0), content)
	}
	return ""
}

// analyzeRepository keeps the analyzer convenient to exercise from tests in
// this package while AnalyzeRepository is the entry point for hook callers.
func analyzeRepository(root string) ([]Diagnostic, error) {
	return AnalyzeRepository(root)
}

func goFiles(directory string) []string {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil
	}
	var files []string
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if _, skip := skippedDirectories[name]; skip {
			continue
		}
		path := filepath.Join(directory, name)
		if entry.IsDir() {
			files = append(files, goFiles(path)...)
			continue
		}
		if strings.HasSuffix(name, ".go") {
			files = append(files, path)
		}
	}
	return files
}

func collectGoDeclarations(root codeparser.Node, content []byte, file string, structs map[string]structDeclaration, methods *[]methodDeclaration) {
	for _, child := range root.NamedChildren() {
		switch child.Kind() {
		case "type_declaration":
			collectStructs(child, content, file, structs)
		case "method_declaration":
			if method, ok := methodFromNode(child, content, file); ok {
				*methods = append(*methods, method)
			}
		}
	}
}

func collectStructs(declaration codeparser.Node, content []byte, file string, structs map[string]structDeclaration) {
	for _, spec := range declaration.NamedChildren() {
		if spec.Kind() != "type_spec" {
			continue
		}
		typeNode := spec.ChildByFieldName("type")
		nameNode := spec.ChildByFieldName("name")
		if !typeNode.Valid() || typeNode.Kind() != "struct_type" || !nameNode.Valid() {
			continue
		}
		name := nodeContent(nameNode, content)
		if name == "" {
			continue
		}
		if previous, exists := structs[name]; exists && previous.file != file {
			structs[name] = structDeclaration{} // Ambiguous build variants are ignored conservatively.
			continue
		}
		structs[name] = structDeclaration{file: file}
	}
}

func methodFromNode(node codeparser.Node, content []byte, file string) (methodDeclaration, bool) {
	nameNode := node.ChildByFieldName("name")
	receiver := node.ChildByFieldName("receiver")
	if !nameNode.Valid() || !receiver.Valid() {
		return methodDeclaration{}, false
	}

	var receiverType codeparser.Node
	for _, parameter := range receiver.NamedChildren() {
		if parameter.Kind() == "parameter_declaration" {
			receiverType = parameter.ChildByFieldName("type")
			break
		}
	}
	base := receiverBaseType(receiverType)
	if !base.Valid() {
		return methodDeclaration{}, false
	}

	typeName := nodeContent(base, content)
	methodName := nodeContent(nameNode, content)
	if typeName == "" || methodName == "" {
		return methodDeclaration{}, false
	}
	return methodDeclaration{
		typeName:   typeName,
		methodName: methodName,
		file:       file,
		line:       node.Range().Start.Line,
	}, true
}

func receiverBaseType(node codeparser.Node) codeparser.Node {
	if !node.Valid() {
		return codeparser.Node{}
	}
	switch node.Kind() {
	case "type_identifier":
		return node
	case "pointer_type":
		if node.NamedChildCount() == 1 {
			return receiverBaseType(node.NamedChild(0))
		}
	case "generic_type":
		return receiverBaseType(node.ChildByFieldName("type"))
	}
	return codeparser.Node{}
}

func nodeContent(node codeparser.Node, _ []byte) string {
	return node.Text()
}

func sameFileDiagnostic(method methodDeclaration, declaration structDeclaration) Diagnostic {
	return Diagnostic{
		Severity: "error",
		Failure: fmt.Sprintf(
			"method %s on %s is declared here, but the type lives in %s — keep all methods of a struct in the file that declares the type",
			method.methodName,
			method.typeName,
			filepath.Base(declaration.file),
		),
		RuleName: sameFileStructMethodsRule,
		Category: "layout",
		Position: DiagnosticPosition{
			Start: SourcePosition{Filename: method.file, Offset: 0, Line: method.line, Column: 1},
			End:   SourcePosition{Filename: method.file, Offset: 1, Line: method.line, Column: 2},
		},
		Confidence:      1,
		ReplacementLine: "",
	}
}
