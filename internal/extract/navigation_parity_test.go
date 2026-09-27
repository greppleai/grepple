package extract

import (
	"path/filepath"
	"strings"
	"testing"

	codeparser "github.com/greppleai/grepple/internal/parser"
)

func TestFocusedFlowsAgreeWithSharedNavigationGraph(t *testing.T) {
	tests := []struct {
		name, path, source, entry string
	}{
		{name: "go-function", path: "main.go", source: "package sample\nfunc Start() { Finish() }\nfunc Finish() {}\n", entry: "Start"},
		{name: "go-method", path: "main.go", source: "package sample\ntype Runner struct{}\nfunc (r *Runner) Start() { r.Finish() }\nfunc (r *Runner) Finish() {}\n", entry: "Runner.Start"},
		{name: "typescript-function", path: "main.ts", source: "export function start(): void { finish() }\nfunction finish(): void {}\n", entry: "start"},
		{name: "typescript-method", path: "main.ts", source: "class Runner { start(): void { this.finish() } finish(): void {} }\n", entry: "Runner.start"},
		{name: "python-function", path: "main.py", source: "def start():\n    finish()\n\ndef finish():\n    pass\n", entry: "start"},
		{name: "python-method", path: "main.py", source: "class Runner:\n    def start(self):\n        self.finish()\n\n    def finish(self):\n        pass\n", entry: "Runner.start"},
		{name: "java-method", path: "Runner.java", source: "class Runner { void start() { finish(); } void finish() {} }\n", entry: "Runner.start"},
		{name: "kotlin-method", path: "Runner.kt", source: "class Runner {\n    fun start() { finish() }\n    fun finish() {}\n}\n", entry: "Runner.start"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertFocusedFlowUsesNavigationGraph(t, Source{Path: test.path, Text: test.source}, test.entry)
		})
	}
}

func assertFocusedFlowUsesNavigationGraph(t *testing.T, source Source, entry string) {
	t.Helper()
	analysis, err := Analyze([]Source{source})
	if err != nil {
		t.Fatal(err)
	}
	diagram, err := GenerateFlowchart(entry, source.Path, []Source{source}, GenerateOptions{Depth: 3, MaxNodes: 20})
	if err != nil {
		t.Fatal(err)
	}
	flow, err := ParseFlowchart(diagram)
	if err != nil {
		t.Fatal(err)
	}
	if len(flow.Edges) == 0 {
		t.Fatalf("focused flow has no edges:\n%s", diagram)
	}
	actual := map[string]bool{}
	selected := map[string]bool{}
	for _, node := range flow.Nodes {
		selected[node.Symbol] = true
	}
	for _, edge := range flow.Edges {
		sourceName := flow.Nodes[edge.Source].Symbol
		targetName := flow.Nodes[edge.Target].Symbol
		actual[sourceName+"\x00"+targetName] = true
	}
	expected := resolvedNavigationEdges(analysis.Navigation, selected)
	if !equalStringSets(actual, expected) {
		t.Errorf("focused flow and shared graph differ: flow=%v graph=%v\n%s", sortedKeys(actual), sortedKeys(expected), diagram)
	}
}

func resolvedNavigationEdges(graph codeparser.NavigationGraph, selected map[string]bool) map[string]bool {
	declarations := map[string]codeparser.NavigationDeclaration{}
	for _, declaration := range graph.Declarations {
		declarations[declaration.ID] = declaration
	}
	result := map[string]bool{}
	for _, call := range graph.Calls {
		source, target := declarations[call.CallerID], declarations[call.TargetID]
		if call.TargetID != "" && selected[source.Name] && selected[target.Name] {
			result[source.Name+"\x00"+target.Name] = true
		}
	}
	return result
}

func equalStringSets(left, right map[string]bool) bool {
	if len(left) != len(right) {
		return false
	}
	for value := range left {
		if !right[value] {
			return false
		}
	}
	return true
}

func TestNavigationGraphCarriesResolvedSemanticContext(t *testing.T) {
	root := t.TempDir()
	app := Source{Path: filepath.Join(root, "app", "app.go"), Text: "package app\nimport \"example.com/helper\"\nfunc Start() { helper.Finish() }\n"}
	helper := Source{Path: filepath.Join(root, "helper", "helper.go"), Text: "package helper\nfunc Finish() {}\n"}
	analysis, err := Analyze([]Source{app, helper})
	if err != nil {
		t.Fatal(err)
	}
	declarations := map[string]codeparser.NavigationDeclaration{}
	for _, declaration := range analysis.Navigation.Declarations {
		declarations[declaration.ID] = declaration
	}
	for _, call := range analysis.Navigation.Calls {
		if call.Display != "helper.Finish" {
			continue
		}
		target := declarations[call.TargetID]
		if call.CallerID == "" || call.TargetID == "" || call.ResolvedName != "Finish" || call.Confidence != "import-resolved" {
			t.Fatalf("import call was not semantically resolved: %+v", call)
		}
		if target.Package != "helper" || target.PackageID == "" || target.Scope != target.PackageID {
			t.Fatalf("target declaration lacks package context: %+v", target)
		}
		return
	}
	t.Fatalf("import call was absent from navigation graph: %+v", analysis.Navigation)
}

type navigationFactoryFixture struct {
	name, entryPath, entryText, factoryPath, factoryText, entry, display, target string
}

func TestNavigationGraphResolvesImportedFactoryReturnTypes(t *testing.T) {
	root := t.TempDir()
	tests := []navigationFactoryFixture{
		{
			name: "go", entryPath: filepath.Join(root, "app", "app.go"), entry: "Start", display: "returned.Load", target: "Client.Load",
			entryText:   "package app\nimport workers \"example.com/worker\"\nfunc Start() { returned := workers.NewClient(); returned.Load() }\n",
			factoryPath: filepath.Join(root, "worker", "worker.go"), factoryText: "package worker\ntype Client struct{}\nfunc NewClient() *Client { return &Client{} }\nfunc (*Client) Load() {}\n",
		},
		{
			name: "typescript", entryPath: filepath.Join(root, "app", "app.ts"), entry: "start", display: "returned.load", target: "Client.load",
			entryText:   "import { makeClient as create } from '../worker';\nfunction start(): void { const returned = create(); returned.load(); }\n",
			factoryPath: filepath.Join(root, "worker.ts"), factoryText: "export class Client { load(): void {} }\nexport function makeClient(): Client { return new Client(); }\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertImportedFactoryReturnType(t, test)
		})
	}
}

func assertImportedFactoryReturnType(t *testing.T, test navigationFactoryFixture) {
	t.Helper()
	sources := []Source{{Path: test.entryPath, Text: test.entryText}, {Path: test.factoryPath, Text: test.factoryText}}
	analysis, err := Analyze(sources)
	if err != nil {
		t.Fatal(err)
	}
	declarations := map[string]codeparser.NavigationDeclaration{}
	for _, declaration := range analysis.Navigation.Declarations {
		declarations[declaration.ID] = declaration
	}
	for _, call := range analysis.Navigation.Calls {
		if call.Display == test.display && declarations[call.TargetID].Name == test.target {
			return
		}
	}
	t.Fatalf("factory return call was not resolved in shared graph: %+v", analysis.Navigation.Calls)
}

func TestTypeScriptStructureFollowsMethodTypes(t *testing.T) {
	source := Source{Path: "main.ts", Text: "export interface Store { load(input: Item): Item }\nexport class Item { value: string }\n"}
	diagram, err := GenerateClassDiagram("Store", source, []Source{source}, GenerateOptions{Depth: 2, MaxNodes: 20})
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{"class Store", "class Item", "Store ..> Item"} {
		if !strings.Contains(diagram, fragment) {
			t.Errorf("focused TypeScript structure missing %q:\n%s", fragment, diagram)
		}
	}
}

func TestFlowNodeLimitProducesValidatedTruncation(t *testing.T) {
	tests := []struct {
		name, path, source, entry string
		order                     []string
	}{
		{name: "go", path: "main.go", source: "package sample\nfunc Start() { One(); Two() }\nfunc One() {}\nfunc Two() {}\n", entry: "Start", order: []string{"Start", "One", "Two"}},
		{name: "typescript", path: "main.ts", source: "function start(): void { one(); two() }\nfunction one(): void {}\nfunction two(): void {}\n", entry: "start", order: []string{"start", "one", "two"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertValidatedFlowTruncation(t, Source{Path: test.path, Text: test.source}, test.entry, test.order)
		})
	}
}

func assertValidatedFlowTruncation(t *testing.T, source Source, entry string, expectedOrder []string) {
	t.Helper()
	first, err := GenerateFlowchart(entry, source.Path, []Source{source}, GenerateOptions{Depth: 3, MaxNodes: 2})
	if err != nil {
		t.Fatal(err)
	}
	second, err := GenerateFlowchart(entry, source.Path, []Source{source}, GenerateOptions{Depth: 3, MaxNodes: 2})
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("truncated flow is not deterministic")
	}
	if !strings.Contains(first, "%% grepple:truncated max-nodes 2") {
		t.Fatalf("truncated flow has no warning metadata:\n%s", first)
	}
	assertFlowSymbolOrder(t, first, expectedOrder[:2])
	complete, err := GenerateFlowchart(entry, source.Path, []Source{source}, GenerateOptions{Depth: 3, MaxNodes: 3})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(complete, "%% grepple:truncated") {
		t.Fatalf("complete boundary was incorrectly marked truncated:\n%s", complete)
	}
	assertFlowSymbolOrder(t, complete, expectedOrder)
	diagnostics, err := CheckFlowchart(first, []Source{source})
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("truncated flow did not validate: %v %+v\n%s", err, diagnostics, first)
	}
}

func assertFlowSymbolOrder(t *testing.T, diagram string, expected []string) {
	t.Helper()
	flow, err := ParseFlowchart(diagram)
	if err != nil {
		t.Fatal(err)
	}
	actual := make([]string, 0, len(flow.Order))
	for _, identifier := range flow.Order {
		actual = append(actual, flow.Nodes[identifier].Symbol)
	}
	if strings.Join(actual, "\x00") != strings.Join(expected, "\x00") {
		t.Fatalf("flow order = %v, want %v\n%s", actual, expected, diagram)
	}
}

func TestStructureNodeLimitProducesValidatedTruncation(t *testing.T) {
	tests := []struct {
		name, path, source, entry string
	}{
		{name: "go", path: "main.go", source: "package sample\ntype Root struct { One One; Two Two }\ntype One struct{}\ntype Two struct{}\n", entry: "Root"},
		{name: "typescript", path: "main.ts", source: "interface Root { one: One; two: Two }\nclass One {}\nclass Two {}\n", entry: "Root"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := Source{Path: test.path, Text: test.source}
			diagram, err := GenerateClassDiagram(test.entry, source, []Source{source}, GenerateOptions{Depth: 3, MaxNodes: 2})
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(diagram, "%% grepple:truncated max-nodes 2") {
				t.Fatalf("truncated structure has no warning metadata:\n%s", diagram)
			}
			diagnostics, err := CheckClassDiagram(diagram, []Source{source})
			if err != nil || len(diagnostics) != 0 {
				t.Fatalf("truncated structure did not validate: %v %+v\n%s", err, diagnostics, diagram)
			}
		})
	}
}
