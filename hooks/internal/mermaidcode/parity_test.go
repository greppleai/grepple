package mermaidcode

import (
	"reflect"
	"strings"
	"testing"
)

func TestGenericHeritageExcludesTypeArguments(t *testing.T) {
	source := Source{"heritage.ts", `
interface Parent<T> {}
interface Other<T> {}
interface Child<T> extends Parent<T>, Other<T> {}
class Worker<T> implements Parent<T>, Other<T> {}
`}
	analysis, err := Analyze([]Source{source})
	if err != nil {
		t.Fatal(err)
	}
	assertKeys(t, analysis.Declarations["Child"].Extends, []string{"Other", "Parent"})
	assertKeys(t, analysis.Declarations["Worker"].Implements, []string{"Other", "Parent"})
}

func TestImportBindingParity(t *testing.T) {
	source := Source{"imports.ts", `
import DefaultName from "default-module";
import * as NamespaceName from "namespace-module";
import { Original as LocalName, type Typed as LocalType, Plain } from "named-module";
import type { TypeOnly } from "types-module";
`}
	analysis, err := Analyze([]Source{source})
	if err != nil {
		t.Fatal(err)
	}
	assertKeys(t, analysis.Imports, []string{"DefaultName", "LocalName", "LocalType", "NamespaceName", "Plain", "TypeOnly"})
	if analysis.Imports["Original"] != nil || analysis.Imports["Typed"] != nil {
		t.Fatal("imported names were incorrectly recorded as local bindings")
	}
	if !analysis.Imports["LocalType"][0].TypeOnly || !analysis.Imports["TypeOnly"][0].TypeOnly {
		t.Fatal("type-only import was not retained")
	}
	if analysis.Imports["LocalName"][0].Source != "named-module" {
		t.Fatal("module specifier was not retained")
	}
}

func TestExportAndDefaultExportParity(t *testing.T) {
	source := Source{"exports.ts", `
class A {}
class B {}
class C {}
export { A, B as PublicB, C as default };
export default class D {}
`}
	analysis, err := Analyze([]Source{source})
	if err != nil {
		t.Fatal(err)
	}
	assertKeys(t, analysis.Exports, []string{"A", "B", "D"})
	assertKeys(t, analysis.DefaultExports, []string{"C", "D"})
}

func TestFunctionOverloadsMatchAnySignature(t *testing.T) {
	source := Source{"overloads.ts", `
export function convert(value: string): number;
export function convert(value: number): string;
export function convert(value: string | number): string | number { return value; }
`}
	diagram := `classDiagram
 class convert {
  +convert(string value): number
 }
 <<function>> convert
 <<export>> convert
`
	diagnostics, err := CheckClassDiagram(diagram, []Source{source})
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 0 {
		analysis, _ := Analyze([]Source{source})
		t.Fatalf("unexpected diagnostics: %+v; functions: %+v", diagnostics, analysis.Functions["convert"])
	}
	analysis, err := Analyze([]Source{source})
	if err != nil {
		t.Fatal(err)
	}
	if len(analysis.Functions["convert"]) != 3 {
		t.Fatalf("got %d overload declarations", len(analysis.Functions["convert"]))
	}
}

func TestGenerationIndependentOfMapIteration(t *testing.T) {
	source := Source{"deterministic.ts", `
interface Zed {}
interface Alpha {}
class Root implements Zed, Alpha { zed: Zed; alpha: Alpha; }
function start(): void { zebra(); alpha(); }
function zebra(): void {}
function alpha(): void {}
`}
	classFirst, err := GenerateClassDiagram("Root", source, []Source{source}, GenerateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	flowFirst, err := GenerateFlowchart("start", source.Path, []Source{source}, GenerateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for iteration := 0; iteration < 100; iteration++ {
		classNext, classErr := GenerateClassDiagram("Root", source, []Source{source}, GenerateOptions{})
		flowNext, flowErr := GenerateFlowchart("start", source.Path, []Source{source}, GenerateOptions{})
		if classErr != nil || flowErr != nil {
			t.Fatalf("generation failed: %v, %v", classErr, flowErr)
		}
		if classNext != classFirst || flowNext != flowFirst {
			t.Fatal("generation changed between iterations")
		}
	}
	if strings.Index(classFirst, "class Alpha") > strings.Index(classFirst, "class Zed") {
		t.Fatal("dependencies are not sorted")
	}
}

func assertKeys[V any](t *testing.T, values map[string]V, expected []string) {
	t.Helper()
	actual := sortedKeys(values)
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("keys: got %v, want %v", actual, expected)
	}
}
