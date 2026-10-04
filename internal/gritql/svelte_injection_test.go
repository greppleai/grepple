package gritql

import "testing"

func TestSvelteNativeEmbeddedSelectorsAndCoordinates(t *testing.T) {
	source := "<!-- café -->\n<script lang='ts'>function greet(name: string) { return name; }</script>\n<style>.button { color: red; }</style>\n<p>function fake()</p>"
	for query, want := range map[string]string{
		"function_declaration(name=$name)": "function greet(name: string) { return name; }",
		"declaration()":                    "color: red;",
	} {
		program, err := Compile([]byte("language svelte\n"+query), CompileOptions{})
		if err != nil {
			t.Fatal(err)
		}
		result := EvaluateFile(t.Context(), program, FileInput{Path: "App.svelte", Language: "svelte", Content: []byte(source)}, EvaluateOptions{})
		assertTargetDiagnostics(t, result, nil)
		assertTargetFindings(t, result, []string{want})
		for _, finding := range result.Findings() {
			rangeValue := finding.Range()
			if source[rangeValue.StartByte:rangeValue.EndByte] != finding.Text() {
				t.Fatal("incorrect source range")
			}
		}
	}
}

func TestSvelteEmbeddedTemplates(t *testing.T) {
	source := "<script lang='ts'>const ignored = 'target(123)'; target(value);</script><style>.x { color: red; }</style>"
	for query, want := range map[string]string{"`target($args)`": "target(value)", "`color: $value;`": "color: red;"} {
		program, err := Compile([]byte("language svelte\n"+query), CompileOptions{})
		if err != nil {
			t.Fatal(err)
		}
		result := EvaluateFile(t.Context(), program, FileInput{Path: "App.svelte", Language: "svelte", Content: []byte(source)}, EvaluateOptions{})
		assertTargetDiagnostics(t, result, nil)
		assertTargetFindings(t, result, []string{want})
	}
}
