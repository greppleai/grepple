// Command generate writes parser language metadata derived from pinned Tree-sitter grammars.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"go/format"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
)

type grammarSpec struct {
	Language string
	Module   string
	Subdir   string
}

type moduleInfo struct {
	Dir string
}

type cardinality struct {
	Multiple bool `json:"multiple"`
}

type nodeReference struct {
	Type  string `json:"type"`
	Named bool   `json:"named"`
}

type nodeType struct {
	Type     string                 `json:"type"`
	Named    bool                   `json:"named"`
	Subtypes []nodeReference        `json:"subtypes"`
	Fields   map[string]cardinality `json:"fields"`
	Children *cardinality           `json:"children"`
}

type grammarMetadata struct {
	Fingerprint string
	Fields      map[string]map[string]bool
	Children    map[string]bool
	Subtypes    map[string]map[string]bool
}

var grammars = []grammarSpec{
	{Language: "go", Module: "github.com/tree-sitter/tree-sitter-go"},
	{Language: "java", Module: "github.com/tree-sitter/tree-sitter-java"},
	{Language: "kotlin", Module: "github.com/tree-sitter-grammars/tree-sitter-kotlin"},
	{Language: "dart", Module: "github.com/nielsenko/tree-sitter-dart"},
	{Language: "swift", Module: "github.com/alex-pinkus/tree-sitter-swift"},
	{Language: "javascript", Module: "github.com/tree-sitter/tree-sitter-javascript"},
	{Language: "hcl", Module: "github.com/tree-sitter-grammars/tree-sitter-hcl"},
	{Language: "typescript", Module: "github.com/tree-sitter/tree-sitter-typescript", Subdir: "typescript"},
	{Language: "tsx", Module: "github.com/tree-sitter/tree-sitter-typescript", Subdir: "tsx"},
	{Language: "python", Module: "github.com/tree-sitter/tree-sitter-python"},
	{Language: "csharp", Module: "github.com/tree-sitter/tree-sitter-c-sharp"},
	{Language: "c", Module: "github.com/tree-sitter/tree-sitter-c"},
	{Language: "cpp", Module: "github.com/tree-sitter/tree-sitter-cpp"},
	{Language: "rust", Module: "github.com/tree-sitter/tree-sitter-rust"},
	{Language: "php", Module: "github.com/tree-sitter/tree-sitter-php", Subdir: "php"},
	{Language: "shell", Module: "github.com/tree-sitter/tree-sitter-bash"},
}

func main() {
	check := flag.Bool("check", false, "verify generated metadata without writing it")
	flag.Parse()
	metadata := make(map[string]grammarMetadata, len(grammars))
	for _, grammar := range grammars {
		generated, err := inspectGrammar(grammar)
		if err != nil {
			fatalf("inspect %s: %v", grammar.Language, err)
		}
		metadata[grammar.Language] = generated
	}
	content, err := render(metadata)
	if err != nil {
		fatalf("render metadata: %v", err)
	}
	const outputPath = "language_metadata.go"
	if *check {
		existing, readErr := os.ReadFile(outputPath)
		if readErr != nil {
			fatalf("read metadata: %v", readErr)
		}
		if !bytes.Equal(existing, content) {
			fatalf("%s is stale; run go generate ./parser", outputPath)
		}
		return
	}
	if err := os.WriteFile(outputPath, content, 0o644); err != nil {
		fatalf("write metadata: %v", err)
	}
}

func inspectGrammar(spec grammarSpec) (grammarMetadata, error) {
	directory, err := moduleDirectory(spec.Module)
	if err != nil {
		return grammarMetadata{}, err
	}
	sourceDirectory := filepath.Join(directory, spec.Subdir, "src")
	nodeTypesPath := filepath.Join(sourceDirectory, "node-types.json")
	content, err := os.ReadFile(nodeTypesPath)
	if err != nil {
		return grammarMetadata{}, err
	}
	var nodes []nodeType
	if err := json.Unmarshal(content, &nodes); err != nil {
		return grammarMetadata{}, fmt.Errorf("decode node-types.json: %w", err)
	}
	metadata := grammarMetadata{Fields: make(map[string]map[string]bool), Children: make(map[string]bool), Subtypes: make(map[string]map[string]bool)}
	for _, node := range nodes {
		addNodeMetadata(&metadata, node)
	}
	metadata.Subtypes = transitiveSubtypeClosure(metadata.Subtypes)
	metadata.Fingerprint, err = grammarFingerprint(sourceDirectory)
	return metadata, err
}
func addNodeMetadata(metadata *grammarMetadata, node nodeType) {
	if !node.Named {
		return
	}
	if len(node.Subtypes) > 0 {
		metadata.Subtypes[node.Type] = make(map[string]bool, len(node.Subtypes))
		for _, subtype := range node.Subtypes {
			if subtype.Named {
				metadata.Subtypes[node.Type][subtype.Type] = true
			}
		}
	}
	if len(node.Fields) > 0 {
		metadata.Fields[node.Type] = make(map[string]bool, len(node.Fields))
		for field, shape := range node.Fields {
			metadata.Fields[node.Type][field] = shape.Multiple
		}
	}
	if node.Children != nil {
		metadata.Children[node.Type] = node.Children.Multiple
	}
}
func transitiveSubtypeClosure(direct map[string]map[string]bool) map[string]map[string]bool {
	closure := make(map[string]map[string]bool, len(direct))
	for supertype, children := range direct {
		reachable := make(map[string]bool)
		pending := sortedMapKeys(children)
		for len(pending) > 0 {
			index := len(pending) - 1
			subtype := pending[index]
			pending = pending[:index]
			if reachable[subtype] {
				continue
			}
			reachable[subtype] = true
			pending = append(pending, sortedMapKeys(direct[subtype])...)
		}
		closure[supertype] = reachable
	}
	return closure
}

func moduleDirectory(module string) (string, error) {
	command := exec.Command("go", "mod", "download", "-json", module)
	output, err := command.Output()
	if err != nil {
		return "", err
	}
	var info moduleInfo
	if err := json.Unmarshal(output, &info); err != nil {
		return "", err
	}
	if info.Dir == "" {
		return "", fmt.Errorf("module directory is empty")
	}
	return info.Dir, nil
}

func grammarFingerprint(directory string) (string, error) {
	names := []string{"parser.c", "scanner.c", "scanner.cc", "node-types.json"}
	hash := sha256.New()
	foundParser := false
	for _, name := range names {
		content, err := os.ReadFile(filepath.Join(directory, name))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", err
		}
		if name == "parser.c" {
			foundParser = true
		}
		fmt.Fprintf(hash, "%s\x00%d\x00", name, len(content))
		_, _ = hash.Write(content)
	}
	if !foundParser {
		return "", fmt.Errorf("parser.c is missing from %s", directory)
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}

func render(metadata map[string]grammarMetadata) ([]byte, error) {
	var output bytes.Buffer
	output.WriteString("// Code generated by go generate ./parser; DO NOT EDIT.\n\n")
	output.WriteString("package parser\n\n")
	output.WriteString("var generatedLanguageMetadata = map[string]languageGeneratedMetadata{\n")
	for _, language := range sortedMapKeys(metadata) {
		writeGrammarMetadata(&output, language, metadata[language])
	}
	output.WriteString("}\n")
	return format.Source(output.Bytes())
}

func writeGrammarMetadata(output *bytes.Buffer, language string, item grammarMetadata) {
	fmt.Fprintf(output, "\t%q: {\n\t\tfingerprint: %q,\n", language, item.Fingerprint)
	output.WriteString("\t\tfields: map[string]map[string]GrammarCardinality{\n")
	for _, parent := range sortedMapKeys(item.Fields) {
		fmt.Fprintf(output, "\t\t\t%q: {", parent)
		for _, field := range sortedMapKeys(item.Fields[parent]) {
			fmt.Fprintf(output, "%q: %s,", field, cardinalityName(item.Fields[parent][field]))
		}
		output.WriteString("},\n")
	}
	output.WriteString("\t\t},\n\t\tchildren: map[string]GrammarCardinality{\n")
	for _, parent := range sortedMapKeys(item.Children) {
		fmt.Fprintf(output, "\t\t\t%q: %s,\n", parent, cardinalityName(item.Children[parent]))
	}
	output.WriteString("\t\t},\n\t\tsubtypes: map[string]map[string]bool{\n")
	for _, supertype := range sortedMapKeys(item.Subtypes) {
		fmt.Fprintf(output, "\t\t\t%q: {", supertype)
		for _, subtype := range sortedMapKeys(item.Subtypes[supertype]) {
			fmt.Fprintf(output, "%q: true,", subtype)
		}
		output.WriteString("},\n")
	}
	output.WriteString("\t\t},\n\t},\n")
}

func cardinalityName(multiple bool) string {
	if multiple {
		return "GrammarCardinalityMany"
	}
	return "GrammarCardinalityOne"
}

func sortedMapKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func fatalf(format string, arguments ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", arguments...)
	os.Exit(1)
}
