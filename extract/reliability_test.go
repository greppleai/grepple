package extract

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestGoPackageIsolationAndGeneration(t *testing.T) {
	root := t.TempDir()
	alphaPath := filepath.Join(root, "alpha", "model.go")
	betaPath := filepath.Join(root, "beta", "model.go")
	alpha := Source{alphaPath, `package alpha
type Worker struct { Name string }
func (w *Worker) Run() {}
func start() { finish() }
func finish() {}
`}
	beta := Source{betaPath, `package beta
type Worker struct { Count int }
func (w *Worker) Stop() {}
func start() {}
func finish() { start() }
`}
	sources := []Source{alpha, beta}
	ambiguous := "classDiagram\n class Worker\n <<struct>> Worker\n"
	diagnostics, err := CheckClassDiagram(ambiguous, sources)
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 1 || !strings.Contains(diagnostics[0].Message, "multiple language or Go package scopes") {
		t.Fatalf("unexpected ambiguity: %+v", diagnostics)
	}

	alphaDiagram := `classDiagram
 class Worker {
  +Name: string
  +Run()
 }
 <<struct>> Worker
 %% grepple:package Worker alpha
`
	diagnostics, err = CheckClassDiagram(alphaDiagram, sources)
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("package-qualified class: %v %+v", err, diagnostics)
	}
	generated, err := GenerateClassDiagram("Worker", alpha, sources, GenerateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(generated, "Count") || strings.Contains(generated, "Stop") {
		t.Fatalf("generation combined packages: %s", generated)
	}
	if !strings.Contains(generated, "%% grepple:package Worker alpha") {
		t.Fatalf("missing generated package metadata: %s", generated)
	}
	functionDiagram := "classDiagram\n class start {\n  -start()\n }\n <<function>> start\n <<go>> start\n"
	diagnostics, err = CheckClassDiagram(functionDiagram, sources)
	if err != nil || len(diagnostics) != 1 || !strings.Contains(diagnostics[0].Message, "Ambiguous code function") {
		t.Fatalf("package function ambiguity: %v %+v", err, diagnostics)
	}
	flowDiagnostics, flowErr := CheckFlowchart("flowchart TD\n start[\"start\"]\n %% grepple:language start go\n", sources)
	if flowErr != nil || len(flowDiagnostics) != 1 || !strings.Contains(flowDiagnostics[0].Message, "Ambiguous code symbol") {
		t.Fatalf("package flow ambiguity: %v %+v", flowErr, flowDiagnostics)
	}
}

func TestGoMembersDoNotLeakFromNestedAnonymousTypes(t *testing.T) {
	source := Source{"nested.go", `package nested
type Outer struct {
 Nested struct { Leaked string }
 Contract interface { Leak() }
}
`}
	analysis, err := Analyze([]Source{source})
	if err != nil {
		t.Fatal(err)
	}
	declaration := analysis.DeclarationVariants["go:Outer"]
	if declaration == nil {
		t.Fatal("missing Outer")
	}
	for _, member := range declaration.Members {
		if member.Name == "Leaked" || member.Name == "Leak" {
			t.Fatalf("nested member leaked: %+v", declaration.Members)
		}
	}
}

func TestGoEmbeddedMethodsExpandTransitively(t *testing.T) {
	source := Source{"embedded.go", `package embedded
type Runner interface { Run() error }
type Extended interface { Runner }
type Base struct{}
func (b *Base) Run() error { return nil }
type Service struct { Base }
`}
	diagram := `classDiagram
 class Extended {
  +Run(): error
 }
 <<interface>> Extended
 <<go>> Extended
 class Service {
  +Run(): error
 }
 <<struct>> Service
 Extended <|.. Service
`
	diagnostics, err := CheckClassDiagram(diagram, []Source{source})
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("transitive embedding: %v %+v", err, diagnostics)
	}
}

func TestQualifiedEmbeddedTypeIsNotLocalHeritage(t *testing.T) {
	source := Source{"qualified.go", `package local
import "example.com/external"
type Base struct{}
type Local struct { external.Base }
`}
	analysis, err := Analyze([]Source{source})
	if err != nil {
		t.Fatal(err)
	}
	local := analysis.DeclarationVariants["go:Local"]
	if !local.Extends["external.Base"] || local.Extends["Base"] {
		t.Fatalf("qualified heritage corrupted: %v", local.Extends)
	}
	generated, err := GenerateClassDiagram("Local", source, []Source{source}, GenerateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(generated, "Base <|-- Local") {
		t.Fatalf("invented local relationship: %s", generated)
	}
}

func TestGoFlowPackageResolution(t *testing.T) {
	root := t.TempDir()
	app := Source{filepath.Join(root, "app", "app.go"), `package app
import "example.com/helper"
func start() { local(); helper.Finish() }
func local() {}
`}
	helper := Source{filepath.Join(root, "helper", "helper.go"), `package helper
func Finish() {}
func local() { Finish() }
`}
	flow := `flowchart TD
 start["start"]
 local["local"]
 finish["Finish"]
 %% grepple:language start go
 %% grepple:package start app
 %% grepple:language local go
 %% grepple:package local app
 %% grepple:symbol finish Finish
 %% grepple:language finish go
 %% grepple:package finish helper
 start --> local
 start --> finish
`
	diagnostics, err := CheckFlowchart(flow, []Source{app, helper})
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("package flow: %v %+v", err, diagnostics)
	}
	limited, limitedErr := GenerateFlowchart("start", app.Path, []Source{app, helper}, GenerateOptions{MaxNodes: 1})
	if limitedErr != nil {
		t.Fatalf("bounded flow generation: %v", limitedErr)
	}
	if !strings.Contains(limited, "%% grepple:truncated max-nodes 1") {
		t.Fatalf("bounded flow missing truncation warning:\n%s", limited)
	}
	generated, generateErr := GenerateFlowchart("start", app.Path, []Source{app, helper}, GenerateOptions{})
	if generateErr != nil {
		t.Fatal(generateErr)
	}
	for _, expected := range []string{"%% grepple:package start app", "%% grepple:package Finish helper", "start --> Finish"} {
		if !strings.Contains(generated, expected) {
			t.Errorf("generated package flow missing %q:\n%s", expected, generated)
		}
	}
}

func TestGenerationEncodesComplexGoMemberTypes(t *testing.T) {
	source := Source{"bad.go", "package bad\ntype Bad struct { Handler func(int, string) error; Done chan struct{} }\n"}
	diagram, err := GenerateClassDiagram("Bad", source, []Source{source}, GenerateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"func&#40;int&#44;string&#41;error", "chan struct&#123;&#125;"} {
		if !strings.Contains(diagram, expected) {
			t.Errorf("generated diagram missing encoded type %q:\n%s", expected, diagram)
		}
	}
	diagnostics, err := CheckClassDiagram(diagram, []Source{source})
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("encoded complex types did not round-trip: %v %+v", err, diagnostics)
	}
}

func TestMalformedSourcesReportTreeSitterLocation(t *testing.T) {
	for _, source := range []Source{{"broken.go", "package broken\ntype Bad struct {"}, {"broken.ts", "class Bad { value: ; }"}, {"broken.py", "def broken(:\n    pass"}, {"Broken.java", "class Bad { void broken( }"}, {"Broken.kt", "class Bad { fun broken( }"}, {"broken.c", "struct Bad { int value;"}, {"broken.cpp", "class Bad { public: void broken( }"}} {
		_, err := Analyze([]Source{source})
		if err == nil || !strings.Contains(err.Error(), source.Path+":") || !strings.Contains(err.Error(), "malformed syntax") {
			t.Errorf("%s: %v", source.Path, err)
		}
	}
}

func TestTypeScriptPrivatePropertyIdentifiersArePrivate(t *testing.T) {
	source := Source{"private.ts", `class Secret { #token: string = "x"; #reset(): void {} }`}
	diagram := `classDiagram
 class Secret {
  -#token: string
  -#reset(): void
 }
 <<typescript>> Secret
`
	diagnostics, err := CheckClassDiagram(diagram, []Source{source})
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("private identifiers: %v %+v", err, diagnostics)
	}
}
