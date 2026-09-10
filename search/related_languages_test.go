package search

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type navigationFixture struct {
	name, extension, content string
}

func TestRelatedNavigationAcrossSupportedLanguages(t *testing.T) {
	fixtures := []navigationFixture{
		{"javascript", ".js", `// helper documentation
const helper = () => "DECLARATION_NEEDLE";
function run() { return helper(); /* NAVIGATION_NEEDLE */ }
`},
		{"typescript", ".ts", `// helper documentation
function helper(): string { return "DECLARATION_NEEDLE"; }
function run(): string { return helper(); /* NAVIGATION_NEEDLE */ }
`},
		{"tsx", ".tsx", `// helper documentation
function helper(): string { return "DECLARATION_NEEDLE"; }
export function Run(): JSX.Element { return <div>{helper() /* NAVIGATION_NEEDLE */}</div>; }
`},
		{"python", ".py", `# helper documentation
def helper():
    return "DECLARATION_NEEDLE"
def run():
    return helper()  # NAVIGATION_NEEDLE
`},
		{"java", ".java", `class NavigationFixture {
    // helper documentation
    String helper() { return "DECLARATION_NEEDLE"; }
    String run() { return helper(); /* NAVIGATION_NEEDLE */ }
}
`},
		{"kotlin", ".kt", `class NavigationFixture {
    // helper documentation
    fun helper(): String = "DECLARATION_NEEDLE"
    fun run(): String = helper() // NAVIGATION_NEEDLE
}
`},
		{"csharp", ".cs", `class NavigationFixture {
    // helper documentation
    string Helper() { return "DECLARATION_NEEDLE"; }
    string Run() { return Helper(); /* NAVIGATION_NEEDLE */ }
}
`},
		{"c", ".c", `// helper documentation
const char *helper(void) { return "DECLARATION_NEEDLE"; }
const char *run(void) { return helper(); /* NAVIGATION_NEEDLE */ }
`},
		{"cpp", ".cpp", `// helper documentation
const char *helper() { return "DECLARATION_NEEDLE"; }
const char *run() { return helper(); /* NAVIGATION_NEEDLE */ }
`},
		{"rust", ".rs", `/// helper documentation
fn helper() -> &'static str { "DECLARATION_NEEDLE" }
fn run() -> &'static str { let _ = "NAVIGATION_NEEDLE"; helper() }
`},
		{"shell", ".sh", `# helper documentation
helper() { printf '%s' 'DECLARATION_NEEDLE'; }
run() { helper; : 'NAVIGATION_NEEDLE'; }
`},
	}

	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) { testRelatedLanguageFixture(t, fixture) })
	}
}

func testRelatedLanguageFixture(t *testing.T, fixture navigationFixture) {
	t.Helper()
	directory := t.TempDir()
	path := filepath.Join(directory, "navigation"+fixture.extension)
	if err := os.WriteFile(path, []byte(fixture.content), 0o600); err != nil {
		t.Fatal(err)
	}

	callerMatches, err := Files(Params{Query: "NAVIGATION_NEEDLE", MaxSegments: 20, Related: true}, []string{path})
	if err != nil {
		t.Fatal(err)
	}
	if len(callerMatches) != 1 {
		t.Fatalf("expected caller match, got %#v", callerMatches)
	}
	callee := findRelatedPoint(t, callerMatches[0].Related, expectedHelperName(fixture.name), "callee")
	if callee.Confidence != "unique-terminal" {
		t.Fatalf("expected unique callee, got %#v", callee)
	}

	declarationMatches, err := Files(Params{Query: "DECLARATION_NEEDLE", MaxSegments: 20, Related: true}, []string{path})
	if err != nil {
		t.Fatal(err)
	}
	caller := findRelatedPoint(t, declarationMatches[0].Related, expectedCallerName(fixture.name), "caller")
	if caller.Confidence != "unique-terminal" {
		t.Fatalf("expected unique caller, got %#v", caller)
	}

	helperLine := lineContaining(fixture.content, "DECLARATION_NEEDLE")
	exact, err := At(Params{At: fmt.Sprintf("%s:%d", path, helperLine), MaxSegments: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(exact.Segments) != 1 || exact.Segments[0].Start > helperLine || exact.Segments[0].End < helperLine {
		t.Fatalf("exact declaration did not contain line %d: %#v", helperLine, exact.Segments)
	}
	if exact.Segments[0].Start >= helperLine {
		t.Fatalf("leading documentation was not attached: %#v", exact.Segments[0])
	}
}
func TestRelatedNavigationResolvesConstructors(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "Navigation.java")
	content := `class Widget { Widget() {} }
class Runner { Object run() { return new Widget(); /* needle */ } }
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	matches, err := Files(Params{Query: "needle", MaxSegments: 20, Related: true}, []string{path})
	if err != nil {
		t.Fatal(err)
	}
	point := findRelatedPoint(t, matches[0].Related, "Widget → Widget.Widget", "callee")
	if point.Kind != "constructor" || point.Confidence != "unique-terminal" {
		t.Fatalf("unexpected constructor navigation: %#v", point)
	}
}

func TestFollowRelatedJavaScriptCallChain(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "chain.js")
	content := `function second() { return "done"; }
function first() { return second(); }
function root() { return first() + "NAVIGATION_NEEDLE"; }
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	matches, err := Files(Params{Query: "NAVIGATION_NEEDLE", MaxSegments: 20, Related: true, FollowRelated: 2}, []string{path})
	if err != nil {
		t.Fatal(err)
	}
	first := findRelatedPoint(t, matches[0].Related, "first", "callee")
	if first.Preview == nil {
		t.Fatal("first JavaScript callee was not expanded")
	}
	second := findRelatedPoint(t, first.Preview.Related, "second", "callee")
	if second.Preview == nil {
		t.Fatal("second JavaScript callee was not expanded")
	}
}

func TestRelatedNamesAreUniqueWithinLanguage(t *testing.T) {
	directory := t.TempDir()
	javascript := filepath.Join(directory, "navigation.js")
	python := filepath.Join(directory, "navigation.py")
	if err := os.WriteFile(javascript, []byte("function helper() {}\nfunction run() { helper(); /* needle */ }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(python, []byte("def helper():\n    pass\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	matches, err := Files(Params{Query: "needle", MaxSegments: 20, Related: true}, []string{javascript, python})
	if err != nil {
		t.Fatal(err)
	}
	point := findRelatedPoint(t, matches[0].Related, "helper", "callee")
	if point.Confidence != "unique-terminal" {
		t.Fatalf("cross-language declaration changed confidence: %#v", point)
	}
}

func TestRelatedNavigationConnectsTypeScriptAndTSX(t *testing.T) {
	directory := t.TempDir()
	helper := filepath.Join(directory, "helper.ts")
	caller := filepath.Join(directory, "caller.tsx")
	if err := os.WriteFile(helper, []byte("export function helper(): string { return 'value'; }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(caller, []byte("function Run() { return <div>{helper() /* needle */}</div>; }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	matches, err := Files(Params{Query: "needle", MaxSegments: 20, Related: true}, []string{caller, helper})
	if err != nil {
		t.Fatal(err)
	}
	point := findRelatedPoint(t, matches[0].Related, "helper", "callee")
	if point.Confidence != "unique-terminal" || !strings.HasSuffix(point.Path, "helper.ts") {
		t.Fatalf("unexpected TypeScript-family point: %#v", point)
	}
}
func TestRelatedNavigationResolvesTypeScriptInterfaceMethods(t *testing.T) {
	directory := t.TempDir()
	declaration := filepath.Join(directory, "clock.ts")
	caller := filepath.Join(directory, "caller.ts")
	if err := os.WriteFile(declaration, []byte("interface Clock { now(): Date; }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(caller, []byte("function run(clock: Clock) { return clock.now(); /* needle */ }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	matches, err := Files(Params{Query: "needle", MaxSegments: 20, Related: true}, []string{caller, declaration})
	if err != nil {
		t.Fatal(err)
	}
	point := findRelatedPoint(t, matches[0].Related, "clock.now → Clock.now", "callee")
	if point.Kind != "method" || point.Confidence != "context-resolved" {
		t.Fatalf("unexpected interface method point: %#v", point)
	}
}

func TestRelatedTypeScriptCallsUseImportAndReceiverTypeContext(t *testing.T) {
	directory := t.TempDir()
	first := filepath.Join(directory, "first.ts")
	second := filepath.Join(directory, "second.ts")
	caller := filepath.Join(directory, "caller.ts")
	if err := os.WriteFile(first, []byte(`export function load(): void {}
export class Client { load(): void {} }
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte(`export function load(): void { const marker = "SECOND_NEEDLE"; }
export class Client { load(): void { const marker = "SECOND_METHOD_NEEDLE"; } }
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(caller, []byte(`import { load as fetch, Client } from "./first";
export function run(client: Client): void {
fetch();
client.load(); // CALLER_NEEDLE
}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	files := []string{caller, first, second}
	matches, err := Files(Params{Query: "CALLER_NEEDLE", MaxSegments: 20, Related: true}, files)
	if err != nil {
		t.Fatal(err)
	}
	imported := findRelatedPoint(t, matches[0].Related, "fetch → load", "callee")
	if imported.Confidence != "import-resolved" || !strings.HasSuffix(imported.Path, "first.ts") {
		t.Fatalf("unexpected named import resolution: %#v", imported)
	}
	method := findRelatedPoint(t, matches[0].Related, "client.load → Client.load", "callee")
	if method.Confidence != "import-resolved" || !strings.HasSuffix(method.Path, "first.ts") {
		t.Fatalf("unexpected TypeScript receiver resolution: %#v", method)
	}

	secondMatches, err := Files(Params{Query: "SECOND_NEEDLE", MaxSegments: 20, Related: true}, files)
	if err != nil {
		t.Fatal(err)
	}
	if len(secondMatches) != 1 || len(secondMatches[0].Related) != 0 {
		t.Fatalf("import-resolved call was attributed to the other module: %#v", secondMatches)
	}
	secondMethodMatches, err := Files(Params{Query: "SECOND_METHOD_NEEDLE", MaxSegments: 20, Related: true}, files)
	if err != nil {
		t.Fatal(err)
	}
	if len(secondMethodMatches) != 1 || len(secondMethodMatches[0].Related) != 0 {
		t.Fatalf("receiver-resolved call was attributed to the other module: %#v", secondMethodMatches)
	}
}

func expectedHelperName(language string) string {
	switch language {
	case "java", "kotlin":
		return "helper → NavigationFixture.helper"
	case "csharp":
		return "Helper → NavigationFixture.Helper"
	default:
		return "helper"
	}
}

func expectedCallerName(language string) string {
	switch language {
	case "java", "kotlin":
		return "NavigationFixture.run"
	case "csharp":
		return "NavigationFixture.Run"
	case "tsx":
		return "Run"
	default:
		return "run"
	}
}

func lineContaining(content, marker string) int {
	for index, line := range strings.Split(content, "\n") {
		if strings.Contains(line, marker) {
			return index + 1
		}
	}
	return 0
}
