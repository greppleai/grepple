package mermaidcode

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const goSample = `package sample

import "example.com/clock"

type Runner interface {
	Run(string) error
}
type Advanced interface {
	Runner
	Stop()
}

type Base struct { ID string }
type Item struct { Name string }
type Worker struct {
	Base
	Items []*Item
	Lookup map[string]*Item
	Left, Right *Item
	clock clock.Clock
	Count int
	Ratio float
}

func (w *Worker) Run(value string) error { w.prepare(); helper(); return nil }
func (w *Worker) Add(values ...int) int { return len(values) }
func (w *Worker) prepare() { finish() }
func helper() { finish() }
func finish() {}
func Build(name string) *Worker { return &Worker{} }
func Pair() (string, error) { return "", nil }
`

func TestGoClassValidation(t *testing.T) {
	diagram := `classDiagram
 class Runner {
  +Run(string): error
 }
 <<interface>> Runner
 <<go>> Runner
 class Advanced {
  +Stop()
 }
 <<interface>> Advanced
 <<go>> Advanced
 class Base {
  +ID: string
 }
 <<struct>> Base
 <<go>> Base
 class Item
 <<struct>> Item
 <<go>> Item
 class Worker {
  +Items: []*Item
  +Lookup: map[string]*Item
  +Left: *Item
  +Right: *Item
  +Run(string): error
  -prepare()
  +Count: int
  +Ratio: float
  +Add(...int): int
 }
 <<struct>> Worker
 <<go>> Worker
 class Build {
  +Build(string): *Worker
 }
 <<function>> Build
 <<go>> Build
 <<export>> Build
 class Pair {
  +Pair(): tuple~string,error~
 }
 <<function>> Pair
 <<go>> Pair
 <<export>> Pair
 class clock
 <<import>> clock
 <<go>> clock
 %% grepple:import clock from "example.com/clock"
 Base <|-- Worker
 Runner <|-- Advanced
 Runner <|.. Worker
 Worker "1" --> "*" Item
`
	diagnostics, err := CheckClassDiagram(diagram, []Source{{"sample.go", goSample}})
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %+v", diagnostics)
	}
}

func TestGoClassFailures(t *testing.T) {
	diagram := `classDiagram
 class Worker {
  +Missing: string
  +Run(number): error
 }
 <<struct>> Worker
 <<go>> Worker
 class Runner { +Run(string): error }
 <<interface>> Runner
 <<go>> Runner
 Runner <|.. Worker
`
	_, err := ParseClassDiagram(diagram)
	if err == nil {
		t.Fatal("expected unsupported inline body to fail")
	}
	diagram = strings.Replace(diagram, "class Runner { +Run(string): error }", "class Runner {\n  +Run(string): error\n }", 1)
	diagnostics, err := CheckClassDiagram(diagram, []Source{{"sample.go", strings.Replace(goSample, "Run(value string) error", "Execute(value string) error", 1)}})
	if err != nil {
		t.Fatal(err)
	}
	messages := diagnosticsText(diagnostics)
	for _, expected := range []string{"Expected public Missing: string", "Expected public Run(number): error", "must implement 'Runner'"} {
		if !strings.Contains(messages, expected) {
			t.Errorf("missing %q in %s", expected, messages)
		}
	}
}

func TestGoFlowDirectTransitiveAndReceiver(t *testing.T) {
	flow := `flowchart TD
 run["Worker.Run"]
 prepare["Worker.prepare"]
 finish["finish"]
 %% grepple:symbol run Worker.Run
 %% grepple:language run go
 %% grepple:symbol prepare Worker.prepare
 %% grepple:language prepare go
 %% grepple:language finish go
 run --> prepare
 run --> finish
`
	diagnostics, err := CheckFlowchart(flow, []Source{{"sample.go", goSample}})
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %+v", diagnostics)
	}
}

func TestGoGenerationDeterministicAndValidated(t *testing.T) {
	source := Source{filepath.Join(t.TempDir(), "sample.go"), goSample}
	classFirst, err := GenerateClassDiagram("Worker", source, []Source{source}, GenerateOptions{Depth: 2, DepthSet: true})
	if err != nil {
		t.Fatal(err)
	}
	classSecond, err := GenerateClassDiagram("Worker", source, []Source{source}, GenerateOptions{Depth: 2, DepthSet: true})
	if err != nil || classFirst != classSecond {
		t.Fatalf("class generation is not deterministic: %v", err)
	}
	for _, expected := range []string{"<<struct>> Worker", "Count: int", "Ratio: float", "Add(...int): int", "<<go>> Worker", "Base <|-- Worker", "Runner <|.. Worker", `Worker "1" --> "*" Item`} {
		if !strings.Contains(classFirst, expected) {
			t.Errorf("missing %q in %s", expected, classFirst)
		}
	}
	flowFirst, err := GenerateFlowchart("Worker.Run", source.Path, []Source{source}, GenerateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	flowSecond, err := GenerateFlowchart("Worker.Run", source.Path, []Source{source}, GenerateOptions{})
	if err != nil || flowFirst != flowSecond {
		t.Fatalf("flow generation is not deterministic: %v", err)
	}
	if !strings.Contains(flowFirst, "%% grepple:language Worker_Run go") {
		t.Fatalf("missing Go language metadata: %s", flowFirst)
	}
}

func TestMixedSourceDiscovery(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{"a.go": "package a", "b.ts": "class B {}", "view.tsx": "export const x = <div />", "vendor/no.go": "package no", "zz_generated.deepcopy.go": "package a", "types.d.ts": "declare class X {}", "types.d.mts": "declare class Y {}", "auto.go": "// Code generated by tool. DO NOT EDIT.\npackage a", "a_test.go": "package a"}
	for name, content := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	paths, err := DiscoverSources([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 3 {
		t.Fatalf("got %v", paths)
	}
}

func diagnosticsText(diagnostics []Diagnostic) string {
	var result []string
	for _, diagnostic := range diagnostics {
		result = append(result, diagnostic.Message)
	}
	return strings.Join(result, "\n")
}
