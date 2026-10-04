package gritql

import "testing"

func TestSvelteNativeNodeQueriesAndSourceRanges(t *testing.T) {
	program, err := Compile([]byte("language svelte\nif_start(condition=$condition)"), CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	source := "<!-- café -->\n{#if ready}\n  <button>Ready</button>\n{/if}\n"
	result := EvaluateFile(t.Context(), program, FileInput{Path: "App.svelte", Language: "svelte", Content: []byte(source)}, EvaluateOptions{})
	if len(result.Diagnostics()) != 0 || len(result.Findings()) != 1 {
		t.Fatalf("findings=%v diagnostics=%v", result.Findings(), result.Diagnostics())
	}
	finding := result.Findings()[0]
	if finding.Text() != "{#if ready}" || finding.Start().Line != 2 {
		t.Fatalf("incorrect source projection: %+v", finding)
	}
	bindings := finding.Bindings()
	if len(bindings) != 1 || source[bindings[0].Range().StartByte:bindings[0].Range().EndByte] != "ready" {
		t.Fatalf("unexpected condition capture: %+v", bindings)
	}
}

func TestSvelteTemplateMatchesMarkupNotOpaqueScriptContents(t *testing.T) {
	program, err := Compile([]byte("language svelte\n`<button>$body</button>`"), CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	source := "<script>const html = '<button>Fake</button>';</script>\n<button>Real</button>\n<style>/* <button>Also fake</button> */</style>"
	result := EvaluateFile(t.Context(), program, FileInput{Path: "App.svelte", Language: "svelte", Content: []byte(source)}, EvaluateOptions{})
	assertTargetDiagnostics(t, result, nil)
	assertTargetFindings(t, result, []string{"<button>Real</button>"})
}

func TestSvelteDoesNotAdvertiseEmbeddedJavaScriptSyntax(t *testing.T) {
	if _, err := Compile([]byte("language svelte\nfunction_declaration()"), CompileOptions{}); err == nil {
		t.Fatal("JavaScript selector accepted without an embedded parser")
	}
}

func TestSvelteRawExpressionCaptureRemainsScalar(t *testing.T) {
	program, err := Compile([]byte("language svelte\n`<span>{$value}</span>`"), CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	source := "<span>{user.name}</span>"
	result := EvaluateFile(t.Context(), program, FileInput{Path: "App.svelte", Language: "svelte", Content: []byte(source)}, EvaluateOptions{})
	assertTargetDiagnostics(t, result, nil)
	assertTargetFindings(t, result, []string{source})
	bindings := result.Findings()[0].Bindings()
	if len(bindings) != 1 || bindings[0].Kind() != BindingNode {
		t.Fatalf("raw expression should be one syntax node: %+v", bindings)
	}
}
