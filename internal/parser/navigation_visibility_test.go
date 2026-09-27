package parser

import "testing"

func TestNavigationDeclarationVisibilityAcrossLanguages(t *testing.T) {
	tests := []struct {
		language string
		content  string
		want     map[string]NavigationVisibility
	}{
		{language: "go", content: "package p\nfunc Public() {}\nfunc private() {}\n", want: map[string]NavigationVisibility{"Public": NavigationVisibilityPublic, "private": NavigationVisibilityNonPublic}},
		{language: "python", content: "def public():\n    pass\ndef _private():\n    pass\n", want: map[string]NavigationVisibility{"public": NavigationVisibilityPublic, "_private": NavigationVisibilityNonPublic}},
		{language: "rust", content: "pub fn public() {}\nfn private() {}\n", want: map[string]NavigationVisibility{"public": NavigationVisibilityPublic, "private": NavigationVisibilityNonPublic}},
		{language: "java", content: "class App { public void publicCall() {} private void privateCall() {} }", want: map[string]NavigationVisibility{"App.publicCall": NavigationVisibilityPublic, "App.privateCall": NavigationVisibilityNonPublic}},
		{language: "kotlin", content: "class App {\n fun publicCall() {}\n private fun privateCall() {}\n}\n", want: map[string]NavigationVisibility{"App.publicCall": NavigationVisibilityPublic, "App.privateCall": NavigationVisibilityNonPublic}},
		{language: "csharp", content: "class App { public void PublicCall() {} private void PrivateCall() {} }", want: map[string]NavigationVisibility{"App.PublicCall": NavigationVisibilityPublic, "App.PrivateCall": NavigationVisibilityNonPublic}},
		{language: "typescript", content: "export function publicCall() {}\nfunction privateCall() {}\n", want: map[string]NavigationVisibility{"publicCall": NavigationVisibilityPublic, "privateCall": NavigationVisibilityNonPublic}},
		{language: "c", content: "void call(void) {}\n", want: map[string]NavigationVisibility{"call": NavigationVisibilityUnknown}},
	}
	for _, test := range tests {
		t.Run(test.language, func(t *testing.T) {
			graph := BuildNavigationGraph(test.content, test.language, "sample")
			got := make(map[string]NavigationVisibility, len(graph.Declarations))
			for _, declaration := range graph.Declarations {
				got[declaration.Name] = declaration.Visibility
			}
			for name, visibility := range test.want {
				if got[name] != visibility {
					t.Fatalf("%s visibility = %q, want %q; declarations=%#v", name, got[name], visibility, graph.Declarations)
				}
			}
		})
	}
}
