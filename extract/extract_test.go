package extract

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixture(t *testing.T, name string) Source {
	t.Helper()
	p := filepath.Join("testdata", name)
	b, e := os.ReadFile(p)
	if e != nil {
		t.Fatal(e)
	}
	return Source{p, string(b)}
}
func diagram(t *testing.T, name string) string { return fixture(t, name).Text }
func TestClassFixture(t *testing.T) {
	src := fixture(t, "sample.ts")
	ds, err := CheckClassDiagram(diagram(t, "sample.mmd"), []Source{src})
	if err != nil {
		t.Fatal(err)
	}
	if len(ds) > 0 {
		t.Fatalf("diagnostics: %+v", ds)
	}
}
func TestBrokenClass(t *testing.T) {
	src := fixture(t, "broken.ts")
	ds, err := CheckClassDiagram(diagram(t, "sample.mmd"), []Source{src})
	if err != nil {
		t.Fatal(err)
	}
	got := ""
	for _, d := range ds {
		got += d.Message + "\n"
	}
	for _, want := range []string{"Expected 'Identifiable' to be an interface; found class", "Missing export for 'Identifiable'", "Missing import 'Clock' from './clock.ts'", "Expected public id: string on 'User'", "Expected public async refresh(): Promise<void>", "Expected async function createSession(User, Clock): Promise<Session>", "'User' must implement 'Identifiable'", "'Message' must extend 'BaseMessage'", "'User' must contain a collection of 'Session'"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
}
func TestGenericCollection(t *testing.T) {
	d := `classDiagram
 class Registry {
  +AgentTool~any~[] tools
 }
 class AgentTool
 Registry "1" --> "*" AgentTool : contains
`
	s := Source{"registry.ts", `class AgentTool {} class Registry { public tools: AgentTool<any>[] = []; }`}
	ds, e := CheckClassDiagram(d, []Source{s})
	if e != nil || len(ds) > 0 {
		t.Fatalf("%v %+v", e, ds)
	}
}
func TestFlowPaths(t *testing.T) {
	flow := `flowchart TD
 start["Start"]
 finish["Finish"]
 start --> finish
`
	src := Source{"flow.ts", `function start(): void { middle(); } function middle(): void { finish(); } function finish(): void {}`}
	ds, e := CheckFlowchart(flow, []Source{src})
	if e != nil || len(ds) > 0 {
		t.Fatalf("%v %+v", e, ds)
	}
}
func TestOrderedFlowAndMetadata(t *testing.T) {
	flow := `flowchart LR
 prepare["Prepare"]
 execute["Execute"]
 prepare --> execute
`
	src := Source{"flow.ts", `function orchestrate(): void { prepare(); execute(); } function prepare(): void {} function execute(): void {}`}
	ds, e := CheckFlowchart(flow, []Source{src})
	if e != nil || len(ds) > 0 {
		t.Fatalf("%v %+v", e, ds)
	}
	mapped := `flowchart TD
 entry["Service.run()"]
 helper["Helper"]
 %% grepple:symbol entry Service.run
 entry --> helper
`
	src.Text = `function helper(): void {} class Service { public run(): void { helper(); } }`
	ds, e = CheckFlowchart(mapped, []Source{src})
	if e != nil || len(ds) > 0 {
		t.Fatalf("%v %+v", e, ds)
	}
}
func TestMissingFlow(t *testing.T) {
	flow := `flowchart TD
 first["First"]
 second["Second"]
 missing["Missing"]
 first --> second
`
	ds, e := CheckFlowchart(flow, []Source{{"flow.ts", `function first(): void {} function second(): void {}`}})
	if e != nil {
		t.Fatal(e)
	}
	got := ""
	for _, d := range ds {
		got += d.Message
	}
	if !strings.Contains(got, "Missing code symbol 'missing'") || !strings.Contains(got, "No static call or ordered phase path") {
		t.Fatal(got)
	}
}
func TestGenerate(t *testing.T) {
	src := fixture(t, "sample.ts")
	a, e := GenerateClassDiagram("User", src, []Source{src}, GenerateOptions{})
	if e != nil {
		t.Fatal(e)
	}
	b, e := GenerateClassDiagram("User", src, []Source{src}, GenerateOptions{})
	if e != nil || a != b {
		t.Fatal("class output not deterministic", e)
	}
	for _, s := range []string{"class User {", "Identifiable <|.. User", "User \"1\" --> \"*\" Session"} {
		if !strings.Contains(a, s) {
			t.Errorf("missing %q", s)
		}
	}
	flowSrc := Source{Path: filepath.Join(t.TempDir(), "flow.ts"), Text: `function start(): void { prepare(); execute(); } function prepare(): void {} function execute(): void { finish(); } function finish(): void {}`}
	f, e := GenerateFlowchart("start", flowSrc.Path, []Source{flowSrc}, GenerateOptions{Depth: 3, DepthSet: true})
	if e != nil {
		t.Fatal(e)
	}
	for _, s := range []string{"start --> prepare", "start --> execute", "execute --> finish"} {
		if !strings.Contains(f, s) {
			t.Errorf("missing %q in %s", s, f)
		}
	}
}
func TestDiscovery(t *testing.T) {
	root := t.TempDir()
	for _, p := range []string{"a.ts", "x/b.tsx", "app.js", "x/view.jsx", "main.py", "x/types.pyi", "tool.pyw", "dist/no.ts", ".grepple/generated.go", "x/no.d.ts"} {
		path := filepath.Join(root, p)
		if e := os.MkdirAll(filepath.Dir(path), 0755); e != nil {
			t.Fatal(e)
		}
		if e := os.WriteFile(path, []byte(""), 0644); e != nil {
			t.Fatal(e)
		}
	}
	paths, e := DiscoverSources([]string{root})
	if e != nil {
		t.Fatal(e)
	}
	if len(paths) != 7 {
		t.Fatalf("got %v", paths)
	}
}
