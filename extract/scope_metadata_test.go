package extract

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTypeScriptClassGenerationTraversesImportsAndRoundTripsBroadRoot(t *testing.T) {
	root := t.TempDir()
	writeScopeFile(t, filepath.Join(root, "package.json"), `{}`)
	entry := Source{filepath.Join(root, "models", "user.ts"), `import { Profile } from "./profile"; export class User { profile: Profile; }`}
	profile := Source{filepath.Join(root, "models", "profile.ts"), `export interface Profile { label: string; }`}
	unrelated := Source{filepath.Join(root, "other", "user.ts"), `export class User { other: number; }`}
	sources := []Source{entry, profile, unrelated}
	diagram, err := GenerateClassDiagram("User", entry, sources, GenerateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"%% grepple:module User models/user.ts", "%% grepple:module Profile models/profile.ts", "User --> Profile"} {
		if !strings.Contains(diagram, expected) {
			t.Fatalf("missing %q:\n%s", expected, diagram)
		}
	}
	if diagnostics, checkErr := CheckClassDiagram(diagram, sources); checkErr != nil || len(diagnostics) != 0 {
		t.Fatalf("broad round trip: %v %+v", checkErr, diagnostics)
	}
}

func TestTypeScriptFlowGenerationTraversesAliasedRelativeImport(t *testing.T) {
	root := t.TempDir()
	writeScopeFile(t, filepath.Join(root, "tsconfig.json"), `{}`)
	entry := Source{filepath.Join(root, "app.ts"), `import { finish as done } from "./finish"; export function start(): void { done(); }`}
	finish := Source{filepath.Join(root, "finish.ts"), `export function finish(): void {}`}
	unrelated := Source{filepath.Join(root, "other", "finish.ts"), `export function finish(): void {}`}
	sources := []Source{entry, finish, unrelated}
	diagram, err := GenerateFlowchart("start", entry.Path, sources, GenerateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"%% grepple:module start app.ts", "%% grepple:module finish finish.ts", "start --> finish"} {
		if !strings.Contains(diagram, expected) {
			t.Fatalf("missing %q:\n%s", expected, diagram)
		}
	}
	if diagnostics, checkErr := CheckFlowchart(diagram, sources); checkErr != nil || len(diagnostics) != 0 {
		t.Fatalf("broad round trip: %v %+v", checkErr, diagnostics)
	}
}

func TestTypeScriptInterfaceMergingAndDuplicateDiagnostics(t *testing.T) {
	path := filepath.Join(t.TempDir(), "merged.ts")
	merged, err := Analyze([]Source{{path, `interface Combined { first: string } interface Combined { second: number }`}})
	if err != nil {
		t.Fatal(err)
	}
	if declaration := merged.ModuleDeclarations[absolutePath(path)+":Combined"]; declaration == nil || len(declaration.Members) != 2 {
		t.Fatalf("interface was not merged: %+v", declaration)
	}
	_, err = Analyze([]Source{{path, `class Conflict {} interface Conflict { value: string }`}})
	if err == nil || !strings.Contains(err.Error(), "incompatible TypeScript declarations Conflict") {
		t.Fatalf("missing duplicate diagnostic: %v", err)
	}
}

func TestGoGenerationUsesExactImportPathAndBroadRoot(t *testing.T) {
	root := t.TempDir()
	writeScopeFile(t, filepath.Join(root, "go.mod"), "module example.com/app\n")
	first := Source{filepath.Join(root, "one", "item.go"), "package one\ntype Item struct { One string }\n"}
	second := Source{filepath.Join(root, "two", "item.go"), "package two\ntype Item struct { Two string }\n"}
	diagram, err := GenerateClassDiagram("Item", first, []Source{first, second}, GenerateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(diagram, "%% grepple:package Item example.com/app/one") {
		t.Fatalf("missing import path:\n%s", diagram)
	}
	if diagnostics, checkErr := CheckClassDiagram(diagram, []Source{first, second}); checkErr != nil || len(diagnostics) != 0 {
		t.Fatalf("broad round trip: %v %+v", checkErr, diagnostics)
	}
}

func writeScopeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestTypeScriptRelativeImportResolutionVariants(t *testing.T) {
	root := t.TempDir()
	writeScopeFile(t, filepath.Join(root, "package.json"), `{}`)
	entry := Source{filepath.Join(root, "main.ts"), `import { A as explicit } from "./a.ts"; import { B } from "./b"; import { C } from "./folder"; import DefaultThing from "./default";`}
	sources := []Source{
		entry,
		{filepath.Join(root, "a.ts"), `export class A {}`},
		{filepath.Join(root, "b.tsx"), `export class B {}`},
		{filepath.Join(root, "folder", "index.mts"), `export class C {}`},
		{filepath.Join(root, "default.cts"), `export default class DefaultThing {}`},
	}
	analysis, err := Analyze(sources)
	if err != nil {
		t.Fatal(err)
	}
	bindings := analysis.ModuleImportBindings[absolutePath(entry.Path)]
	for _, name := range []string{"explicit", "B", "C", "DefaultThing"} {
		if bindings[name].ModuleID == "" {
			t.Fatalf("binding %s was not resolved: %+v", name, bindings[name])
		}
	}
	if bindings["explicit"].Resolved != "A" || bindings["DefaultThing"].Resolved != "DefaultThing" {
		t.Fatalf("import names not retained: %+v", bindings)
	}
}

func TestTypeScriptModuleSuffixAmbiguityIsDiagnostic(t *testing.T) {
	root := t.TempDir()
	sources := []Source{
		{filepath.Join(root, "a", "user.ts"), `class User {}`},
		{filepath.Join(root, "b", "user.ts"), `class User {}`},
	}
	diagram := "classDiagram\n class User\n <<typescript>> User\n %% grepple:module User user.ts\n"
	diagnostics, err := CheckClassDiagram(diagram, sources)
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 1 || !strings.Contains(diagnostics[0].Message, "Ambiguous") {
		t.Fatalf("expected ambiguity: %+v", diagnostics)
	}
}
