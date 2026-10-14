package navigation

import (
	"github.com/greppleai/grepple/internal/parser"
	"testing"
)

// Run actual extraction/resolution before differential traversal. These cases
// deliberately include field/type usage and different declaration scopes.
func TestSnapshotDifferentialLanguageAdapters(t *testing.T) {
	cases := map[string]TextSource{
		"go":         {"main.go", "package main\ntype Widget struct { next *Widget }\nfunc finish() {}\nfunc start() { finish() }\n"},
		"typescript": {"main.ts", "export class Widget { next: Widget; finish() {} start() { this.finish(); } }"},
		"javascript": {"main.js", "export function finish() {}\nexport function start() { finish(); }"},
		"tsx":        {"main.tsx", "function finish() {}\nexport function start() { finish(); return <div/>; }"},
		"python":     {"main.py", "class Widget:\n    def finish(self):\n        pass\n    def start(self):\n        self.finish()\n"},
		"rust":       {"main.rs", "struct Widget { next: Option<Box<Widget>> }\nfn finish() {}\nfn start() { finish(); }"},
		"java":       {"Main.java", "class Main { Main next; void finish() {} void start() { finish(); } }"},
		"kotlin":     {"Main.kt", "class Main { fun finish() {} fun start() { finish() } }"},
		"scala":      {"Main.scala", "object Main { def finish(): Unit = {}\n def start(): Unit = { finish() } }"},
		"csharp":     {"Main.cs", "class Main { Main next; void Finish() {} void Start() { Finish(); } }"},
		"c":          {"main.c", "void finish(void) {}\nvoid start(void) { finish(); }"},
		"cpp":        {"main.cpp", "class Main { public: void finish() {} void start() { finish(); } };"},
		"php":        {"main.php", "<?php function finish() {} function start() { finish(); }"},
		"ruby":       {"main.rb", "def finish\nend\ndef start\n finish\nend\n"},
		"swift":      {"main.swift", "func finish() {}\nfunc start() { finish() }"},
		"dart":       {"main.dart", "void finish() {}\nvoid start() { finish(); }"},
		"svelte":     {"main.svelte", "<script lang=\"ts\">function finish() {} function start() { finish(); }</script><button onclick={start}>Go</button>"},
		"hcl":        {"main.tf", "resource \"widget\" \"one\" { name = \"one\" }\nresource \"widget\" \"two\" { name = widget.one.name }"},
		"css":        {"main.css", "@keyframes fade { from { opacity: 0; } to { opacity: 1; } }\n.widget { animation: fade 1s; }"},
		"shell":      {"main.sh", "finish() { echo done; }\nstart() { finish; }"},
	}
	capabilities := parser.NewParser().SupportedLanguages()
	for _, capability := range capabilities {
		if !capability.Navigation {
			continue
		}
		source, ok := cases[capability.ID]
		if !ok {
			t.Fatalf("no snapshot parity fixture for supported adapter %s", capability.ID)
		}
		t.Run(capability.ID, func(t *testing.T) {
			analysis, stats := NewGraphEngine(BuildOptions{DisableCache: true}).BuildTextSources([]TextSource{source})
			graph := analysis.Graph()
			if stats.Parsed != 1 || len(graph.Declarations) == 0 {
				t.Fatalf("adapter fixture failed: %+v declarations=%d", stats, len(graph.Declarations))
			}
			assertSnapshotParity(t, graph)
			parts := NewGraphOperations().PartitionLanguages(graph)
			if len(parts) != 1 {
				t.Fatalf("standalone adapter split into %d groups", len(parts))
			}
			assertSnapshotParity(t, parts[0].Graph)
			assertPagedSnapshotParity(t, parts[0].Graph)
		})
	}
}
