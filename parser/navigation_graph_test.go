package parser

import (
	"encoding/json"
	"testing"
)

func TestNavigationGraphFromDocumentDoesNotReparse(t *testing.T) {
	content := "package sample\nfunc Start() { Finish() }\nfunc Finish() {}\n"
	before := parseInvocations.Load()
	document, err := ParseDocument("go", content)
	if err != nil {
		t.Fatal(err)
	}
	defer document.Close()
	afterParse := parseInvocations.Load()
	graph := NavigationGraphFromDocument(document, "sample/main.go")
	if got := parseInvocations.Load(); afterParse != before+1 || got != afterParse {
		t.Fatalf("parse invocations before=%d after-parse=%d after-graph=%d", before, afterParse, got)
	}
	if len(graph.Declarations) != 2 || len(graph.Calls) != 1 {
		t.Fatalf("graph shape = %d declarations, %d calls: %+v", len(graph.Declarations), len(graph.Calls), graph)
	}
}

func TestNavigationGraphCapturesTypeDeclarationRanges(t *testing.T) {
	content := "package sample\ntype Request struct { Name string }\nfunc Run(request Request) Request { return request }\n"
	graph := BuildNavigationGraph(content, "go", "sample/main.go")
	if len(graph.TypeDeclarations) != 1 {
		t.Fatalf("type declarations=%+v", graph.TypeDeclarations)
	}
	declaration := graph.TypeDeclarations[0]
	if declaration.Name != "Request" || declaration.Kind != "struct" || declaration.Path != "sample/main.go" || declaration.Start != 2 || declaration.End != 2 {
		t.Fatalf("type declaration=%+v", declaration)
	}
	if len(graph.TypeUsages) != 2 {
		t.Fatalf("type usages=%+v", graph.TypeUsages)
	}
}

func TestNavigationGraphLabelsGoReceiverTypeUsage(t *testing.T) {
	content := "package sample\ntype Service struct{}\nfunc (service Service) Run() {}\n"
	graph := BuildNavigationGraph(content, "go", "sample/main.go")
	roles := make(map[string]int)
	for _, usage := range graph.TypeUsages {
		roles[usage.Role]++
	}
	if roles["receiver"] != 1 || roles["parameter"] != 0 {
		t.Fatalf("type usage roles=%+v usages=%+v", roles, graph.TypeUsages)
	}
}

func TestBuildNavigationGraphRetainsSourceIdentity(t *testing.T) {
	content := "package sample\nfunc Start() { Finish() }\nfunc Finish() {}\n"
	graph := BuildNavigationGraph(content, "go", "sample/main.go")
	if len(graph.Declarations) != 2 || len(graph.Calls) != 1 {
		t.Fatalf("graph shape = %d declarations, %d calls: %+v", len(graph.Declarations), len(graph.Calls), graph)
	}
	for _, declaration := range graph.Declarations {
		if declaration.Language != "go" || declaration.Path != "sample/main.go" {
			t.Fatalf("declaration source identity = %+v", declaration)
		}
	}
	call := graph.Calls[0]
	if call.Name != "Finish" || call.Language != "go" || call.Path != "sample/main.go" {
		t.Fatalf("call = %+v", call)
	}
	if graph.Declarations[0].ID == "" || call.ID == "" || call.CallerID != graph.Declarations[0].ID {
		t.Fatalf("stable graph identities were not linked: %+v", graph)
	}
	repeated := BuildNavigationGraph(content, "go", "sample/main.go")
	if repeated.Declarations[0].ID != graph.Declarations[0].ID || repeated.Calls[0].ID != call.ID {
		t.Fatalf("graph identities are not deterministic: first=%+v repeated=%+v", graph, repeated)
	}
}

func TestNavigationGraphCapturesAdapterEvidencedGoEntrypoint(t *testing.T) {
	content := "package main\nfunc main() {}\nfunc helper() {}\n"
	graph := BuildNavigationGraph(content, "go", "cmd/server/main.go")
	if len(graph.Declarations) != 2 || graph.Declarations[0].Entrypoint != "process" || graph.Declarations[1].Entrypoint != "" {
		t.Fatalf("declarations=%+v", graph.Declarations)
	}
}

func TestNavigationGraphCapturesPythonProcessEntrypoints(t *testing.T) {
	for _, test := range []struct {
		name, path, content string
		want                int
	}{
		{name: "module guard", content: "def launch():\n    pass\nif __name__ == \"__main__\":\n    launch()\n", want: 1},
		{name: "reversed module guard", content: "def launch():\n    pass\nif '__main__' == __name__:\n    launch()\n", want: 1},
		{name: "name only", content: "def main():\n    pass\n"},
		{name: "wrong comparison", content: "def launch():\n    pass\nif __name__ != \"__main__\":\n    launch()\n"},
		{name: "indirect call", content: "def launch():\n    pass\nif __name__ == \"__main__\":\n    selected = launch\n    selected()\n"},
		{name: "class method", content: "class App:\n    def launch(self):\n        pass\nif __name__ == \"__main__\":\n    App().launch()\n"},
		{name: "duplicate definitions", content: "def launch():\n    pass\ndef launch():\n    pass\nif __name__ == \"__main__\":\n    launch()\n"},
		{name: "async call without await", content: "async def launch():\n    pass\nif __name__ == \"__main__\":\n    launch()\n"},
		{name: "type stub", path: "app.pyi", content: "def launch():\n    pass\nif __name__ == \"__main__\":\n    launch()\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := test.path
			if path == "" {
				path = "app.py"
			}
			graph := BuildNavigationGraph(test.content, "python", path)
			got := 0
			for _, declaration := range graph.Declarations {
				if declaration.Entrypoint == "process" {
					got++
				}
			}
			if got != test.want {
				t.Fatalf("process entrypoints=%d want=%d declarations=%+v", got, test.want, graph.Declarations)
			}
		})
	}
}

func TestNavigationGraphCapturesCommonJSProcessEntrypoints(t *testing.T) {
	for _, test := range []struct {
		name, language, path, content string
		want                          int
	}{
		{name: "javascript block guard", language: "javascript", path: "app.js", content: "function launch() {}\nif (require.main === module) { launch(); }\n", want: 1},
		{name: "javascript reversed direct guard", language: "javascript", path: "app.js", content: "function launch() {}\nif (module === require.main) launch();\n", want: 1},
		{name: "javascript exported function", language: "javascript", path: "app.js", content: "export function launch() {}\nif (require.main === module) { launch(); }\n", want: 1},
		{name: "typescript guard", language: "typescript", path: "app.ts", content: "function launch(): void {}\nif (require.main === module) { launch(); }\n", want: 1},
		{name: "typescript commonjs extension", language: "typescript", path: "app.cts", content: "function launch(): void {}\nif (require.main === module) { launch(); }\n", want: 1},
		{name: "name only", language: "javascript", path: "app.js", content: "function main() {}\n"},
		{name: "loose equality", language: "javascript", path: "app.js", content: "function launch() {}\nif (require.main == module) { launch(); }\n"},
		{name: "indirect call", language: "javascript", path: "app.js", content: "function launch() {}\nif (require.main === module) { const selected = launch; selected(); }\n"},
		{name: "duplicate declarations", language: "javascript", path: "app.js", content: "function launch() {}\nfunction launch() {}\nif (require.main === module) { launch(); }\n"},
		{name: "nested declaration", language: "javascript", path: "app.js", content: "function outer() { function launch() {} }\nif (require.main === module) { launch(); }\n"},
		{name: "explicit esm typescript", language: "typescript", path: "app.mts", content: "function launch(): void {}\nif (require.main === module) { launch(); }\n"},
		{name: "tsx guard", language: "tsx", path: "app.tsx", content: "function launch(): void {}\nif (require.main === module) { launch(); }\n", want: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			graph := BuildNavigationGraph(test.content, test.language, test.path)
			got := 0
			for _, declaration := range graph.Declarations {
				if declaration.Entrypoint == "process" {
					got++
				}
			}
			if got != test.want {
				t.Fatalf("process entrypoints=%d want=%d declarations=%+v", got, test.want, graph.Declarations)
			}
		})
	}
}

func TestNavigationGraphCapturesRustProcessEntrypoints(t *testing.T) {
	for _, test := range []struct {
		name, path, content string
		want                int
	}{
		{name: "main crate", path: "app/src/main.rs", content: "fn main() {}", want: 1},
		{name: "bin file", path: "app/src/bin/server.rs", content: "fn main() {}", want: 1},
		{name: "bin directory", path: "app/src/bin/server/main.rs", content: "fn main() {}", want: 1},
		{name: "library", path: "app/src/lib.rs", content: "fn main() {}"},
		{name: "non-root file", path: "app/src/module.rs", content: "fn main() {}"},
		{name: "inline module", path: "app/src/main.rs", content: "mod nested { fn main() {} }"},
	} {
		t.Run(test.name, func(t *testing.T) {
			graph := BuildNavigationGraph(test.content, "rust", test.path)
			got := 0
			for _, declaration := range graph.Declarations {
				if declaration.Entrypoint == "process" {
					got++
				}
			}
			if got != test.want {
				t.Fatalf("process entrypoints=%d want=%d declarations=%+v", got, test.want, graph.Declarations)
			}
		})
	}
}

func TestNavigationGraphCapturesJVMProcessEntrypoints(t *testing.T) {
	for _, test := range []struct {
		name, language, path, content string
		want                          int
	}{
		{name: "java array", language: "java", path: "src/App.java", content: "class App { public static void main(String[] args) {} }", want: 1},
		{name: "java varargs", language: "java", path: "src/App.java", content: "class App { public static void main(String... args) {} }", want: 1},
		{name: "java instance", language: "java", path: "src/App.java", content: "class App { public void main(String[] args) {} }"},
		{name: "java non-public", language: "java", path: "src/App.java", content: "class App { static void main(String[] args) {} }"},
		{name: "java wrong return", language: "java", path: "src/App.java", content: "class App { public static int main(String[] args) { return 0; } }"},
		{name: "kotlin no arguments", language: "kotlin", path: "src/App.kt", content: "fun main() {}", want: 1},
		{name: "kotlin arguments", language: "kotlin", path: "src/App.kt", content: "fun main(args: Array<String>) {}", want: 1},
		{name: "kotlin unit expression", language: "kotlin", path: "src/App.kt", content: "fun main(): Unit = println(\"ok\")", want: 1},
		{name: "kotlin inferred expression", language: "kotlin", path: "src/App.kt", content: "fun main() = println(\"unknown return type\")"},
		{name: "kotlin private", language: "kotlin", path: "src/App.kt", content: "private fun main() {}"},
		{name: "kotlin class member", language: "kotlin", path: "src/App.kt", content: "class App { fun main() {} }"},
		{name: "kotlin script", language: "kotlin", path: "src/App.kts", content: "fun main() {}"},
		{name: "csharp no arguments", language: "csharp", path: "src/App.cs", content: "class App { static void Main() {} }", want: 1},
		{name: "csharp arguments", language: "csharp", path: "src/App.cs", content: "class App { public static int Main(string[] args) { return 0; } }", want: 1},
		{name: "csharp task", language: "csharp", path: "src/App.cs", content: "class App { static System.Threading.Tasks.Task Main(string[] args) => null; }", want: 1},
		{name: "csharp task result", language: "csharp", path: "src/App.cs", content: "class App { static System.Threading.Tasks.Task<int> Main() => null; }", want: 1},
		{name: "csharp unresolved task", language: "csharp", path: "src/App.cs", content: "class App { static Task<int> Main() => null; }"},
		{name: "csharp instance", language: "csharp", path: "src/App.cs", content: "class App { void Main() {} }"},
		{name: "csharp wrong parameter", language: "csharp", path: "src/App.cs", content: "class App { static void Main(string value) {} }"},
		{name: "csharp ref parameter", language: "csharp", path: "src/App.cs", content: "class App { static void Main(ref string[] args) {} }"},
	} {
		t.Run(test.name, func(t *testing.T) {
			graph := BuildNavigationGraph(test.content, test.language, test.path)
			got := 0
			for _, declaration := range graph.Declarations {
				if declaration.Entrypoint == "process" {
					got++
				}
			}
			if got != test.want {
				t.Fatalf("process entrypoints=%d want=%d declarations=%+v", got, test.want, graph.Declarations)
			}
		})
	}
}

func TestNavigationGraphCapturesCFamilyProcessEntrypoints(t *testing.T) {
	for _, test := range []struct {
		language, content string
		want              int
	}{
		{language: "c", content: "int main(void) { return 0; }", want: 1},
		{language: "cpp", content: "int main() { return 0; }", want: 1},
		{language: "c", content: "void main(void) {}"},
		{language: "c", content: "static int main(void) { return 0; }"},
		{language: "cpp", content: "namespace app { int main() { return 0; } }"},
		{language: "cpp", content: "class App { int main() { return 0; } };"},
	} {
		graph := BuildNavigationGraph(test.content, test.language, "main."+test.language)
		got := 0
		for _, declaration := range graph.Declarations {
			if declaration.Entrypoint == "process" {
				got++
			}
		}
		if got != test.want {
			t.Errorf("language=%s content=%q process entrypoints=%d want=%d declarations=%+v", test.language, test.content, got, test.want, graph.Declarations)
		}
	}
}

func TestNavigationGraphDoesNotInferGoEntrypointsWithoutOwningSemantics(t *testing.T) {
	content := "package library\nfunc main() {}\n"
	graph := BuildNavigationGraph(content, "go", "library.go")
	if graph.Declarations[0].Entrypoint != "" {
		t.Fatalf("graph=%+v", graph)
	}
}

const navigationCacheTestContent = "package sample\nimport fmt \"fmt\"\nfunc Start() { Finish() }\nfunc Finish() {}\n"

func TestCachedNavigationGraphReusesContentFactsWithoutChangingOutput(t *testing.T) {
	t.Setenv(NavigationCacheDirectoryEnv, t.TempDir())
	before := parseInvocations.Load()
	cold, coldRecovered, coldHit := cachedTestNavigationGraph(t, navigationCacheTestContent, "sample/main.go")
	afterCold := parseInvocations.Load()
	warm, warmRecovered, warmHit := cachedTestNavigationGraph(t, navigationCacheTestContent, "sample/main.go")
	if coldRecovered || coldHit || warmRecovered || !warmHit {
		t.Fatalf("cache states cold=(%v,%v) warm=(%v,%v)", coldRecovered, coldHit, warmRecovered, warmHit)
	}
	if afterCold != before+1 || parseInvocations.Load() != afterCold {
		t.Fatalf("parse invocations before=%d cold=%d warm=%d", before, afterCold, parseInvocations.Load())
	}
	if navigationGraphJSON(cold) != navigationGraphJSON(warm) {
		t.Fatalf("cold and warm graphs differ:\ncold %s\nwarm %s", navigationGraphJSON(cold), navigationGraphJSON(warm))
	}
}

func TestCachedNavigationGraphInstantiatesPathsAndInvalidatesContent(t *testing.T) {
	t.Setenv(NavigationCacheDirectoryEnv, t.TempDir())
	cold, _, _ := cachedTestNavigationGraph(t, navigationCacheTestContent, "sample/main.go")
	moved, _, movedHit := cachedTestNavigationGraph(t, navigationCacheTestContent, "moved/main.go")
	if !movedHit || moved.Declarations[0].Path != "moved/main.go" || moved.Declarations[0].ID == cold.Declarations[0].ID || len(moved.Imports) != 1 || moved.Imports[0].Path != "moved/main.go" {
		t.Fatalf("path-instantiated cache hit=%v declaration=%+v", movedHit, moved.Declarations[0])
	}
	if moved.Calls[0].CallerID != moved.Declarations[0].ID {
		t.Fatalf("repathed call is not linked to declaration: %+v", moved.Calls[0])
	}
	_, _, changedHit := cachedTestNavigationGraph(t, navigationCacheTestContent+"\n", "sample/main.go")
	if changedHit {
		t.Fatal("changed content reused stale navigation facts")
	}
}

func TestCachedNavigationGraphFromDocumentReusesFacts(t *testing.T) {
	t.Setenv(NavigationCacheDirectoryEnv, t.TempDir())
	cold, _, _ := cachedTestNavigationGraph(t, navigationCacheTestContent, "sample/main.go")
	document, err := ParseDocument("go", navigationCacheTestContent)
	if err != nil {
		t.Fatal(err)
	}
	fromDocument, hit := CachedNavigationGraphFromDocument(document, "sample/main.go")
	document.Close()
	if !hit || navigationGraphJSON(fromDocument) != navigationGraphJSON(cold) {
		t.Fatalf("document cache reuse hit=%v graph=%s", hit, navigationGraphJSON(fromDocument))
	}
}

func cachedTestNavigationGraph(t *testing.T, content, path string) (NavigationGraph, bool, bool) {
	t.Helper()
	graph, recovered, hit, err := CachedNavigationGraph(content, "go", path)
	if err != nil {
		t.Fatal(err)
	}
	return graph, recovered, hit
}

func navigationGraphJSON(graph NavigationGraph) string {
	encoded, _ := json.Marshal(graph)
	return string(encoded)
}

func TestNavigationCompatibilityUsesNormalizedGraph(t *testing.T) {
	content := "function start(): void { finish() }\nfunction finish(): void {}\n"
	graph := BuildNavigationGraph(content, "typescript", "")
	declarations, calls := Navigation(content, "typescript")
	if len(declarations) != len(graph.Declarations) || len(calls) != len(graph.Calls) {
		t.Fatalf("compatibility result differs from graph: graph=%+v declarations=%+v calls=%+v", graph, declarations, calls)
	}
}

func TestNavigationGraphCapturesGoImportAndReceiverContext(t *testing.T) {
	content := `package app
import workers "example.com/project/worker"
type Service struct{}
func (service *Service) Run(client *workers.Client) {
client.Load()
workers.Start()
service.Stop()
}
`
	graph := BuildNavigationGraph(content, "go", "app/service.go")
	calls := navigationCallsByDisplay(graph.Calls)
	if call := calls["client.Load"]; call.ReceiverType != "Client" || call.ImportPath != "example.com/project/worker" {
		t.Fatalf("client receiver context = %+v", calls["client.Load"])
	}
	if calls["workers.Start"].ImportPath != "example.com/project/worker" {
		t.Fatalf("worker import context = %+v", calls["workers.Start"])
	}
	if calls["service.Stop"].ReceiverType != "Service" {
		t.Fatalf("method receiver context = %+v", calls["service.Stop"])
	}
	for _, declaration := range graph.Declarations {
		if declaration.Package != "app" {
			t.Fatalf("declaration package context = %+v", declaration)
		}
	}
}

func TestNavigationGraphCapturesTypeScriptImportAndReceiverContext(t *testing.T) {
	content := `import { load as fetch, Client } from "./worker";
import * as worker from "./worker";
class Runner {
run(client: Client): void {
fetch();
worker.load();
client.load();
this.stop();
}
stop(): void {}
}
`
	graph := BuildNavigationGraph(content, "typescript", "src/runner.ts")
	calls := navigationCallsByDisplay(graph.Calls)
	if call := calls["fetch"]; call.ImportPath != "./worker" || call.ResolvedName != "load" {
		t.Fatalf("named import context = %+v", call)
	}
	if call := calls["worker.load"]; call.ImportPath != "./worker" || call.ResolvedName != "load" {
		t.Fatalf("namespace import context = %+v", call)
	}
	if call := calls["client.load"]; call.ReceiverType != "Client" || call.ImportPath != "./worker" {
		t.Fatalf("parameter receiver context = %+v", calls["client.load"])
	}
	if calls["this.stop"].ReceiverType != "Runner" {
		t.Fatalf("this receiver context = %+v", calls["this.stop"])
	}
}

func TestNavigationGraphPropagatesLocalAndFieldBindings(t *testing.T) {
	goContent := `package app
import workers "example.com/project/worker"
type Service struct { client *workers.Client }
func (service *Service) Run() {
local := &workers.Client{}
var explicit *workers.Client
local.Load()
explicit.Load()
service.client.Load()
}
`
	goGraph := BuildNavigationGraph(goContent, "go", "app/service.go")
	goCalls := navigationCallsByDisplay(goGraph.Calls)
	for _, display := range []string{"local.Load", "explicit.Load", "service.client.Load"} {
		call := goCalls[display]
		if call.ReceiverType != "Client" || call.ImportPath != "example.com/project/worker" {
			t.Fatalf("Go %s binding context = %+v", display, call)
		}
	}
	assertGoNavigationFieldFacts(t, goGraph.Fields)

	typeScriptContent := `import { Client } from "./worker";
class Runner {
client: Client;
run(): void {
const local = new Client();
let explicit: Client;
local.load();
explicit.load();
this.client.load();
}
}
`
	typeScriptCalls := navigationCallsByDisplay(BuildNavigationGraph(typeScriptContent, "typescript", "src/runner.ts").Calls)
	for _, display := range []string{"local.load", "explicit.load", "this.client.load"} {
		call := typeScriptCalls[display]
		if call.ReceiverType != "Client" || call.ImportPath != "./worker" {
			t.Fatalf("TypeScript %s binding context = %+v", display, call)
		}
	}
	forwardReference := `import { Client } from "./worker";
function run(): void {
local.load();
const local = new Client();
}
`
	forwardCall := navigationCallsByDisplay(BuildNavigationGraph(forwardReference, "typescript", "src/forward.ts").Calls)["local.load"]
	if forwardCall.ReceiverType != "" || forwardCall.ImportPath != "" {
		t.Fatalf("future local declaration leaked into earlier call: %+v", forwardCall)
	}
	nestedReference := `import { Client } from "./worker";
function run(): void {
if (true) {
const hidden = new Client();
hidden.load();
}
hidden.load();
}
`
	nestedCalls := BuildNavigationGraph(nestedReference, "typescript", "src/nested.ts").Calls
	insideCall := navigationCallAtLine(nestedCalls, "hidden.load", 5)
	if insideCall.ReceiverType != "Client" || insideCall.ImportPath != "./worker" {
		t.Fatalf("nested local binding was not applied inside its lexical scope: %+v", insideCall)
	}
	outsideCall := navigationCallAtLine(nestedCalls, "hidden.load", 7)
	if outsideCall.ReceiverType != "" || outsideCall.ImportPath != "" {
		t.Fatalf("nested local binding leaked outside its lexical scope: %+v", outsideCall)
	}
}

func TestNavigationGraphPropagatesNestedGoBindings(t *testing.T) {
	content := `package app
import workers "example.com/project/worker"
func Run() {
{
local := &workers.Client{}
local.Load()
}
local.Load()
}
`
	calls := BuildNavigationGraph(content, "go", "app/run.go").Calls
	insideCall := navigationCallAtLine(calls, "local.Load", 6)
	if insideCall.ReceiverType != "Client" || insideCall.ImportPath != "example.com/project/worker" {
		t.Fatalf("nested Go binding was not applied inside its lexical scope: %+v", insideCall)
	}
	outsideCall := navigationCallAtLine(calls, "local.Load", 8)
	if outsideCall.ReceiverType != "" || outsideCall.ImportPath != "" {
		t.Fatalf("nested Go binding leaked outside its lexical scope: %+v", outsideCall)
	}
}

func TestNavigationGraphInfersDirectCallReturnBindings(t *testing.T) {
	tests := []struct {
		name, language, path, content, display, receiverType, importPath string
		line                                                             int
	}{
		{
			name: "go", language: "go", path: "app/run.go", display: "local.Load", line: 5, receiverType: "Client", importPath: "example.com/project/worker",
			content: `package app
import workers "example.com/project/worker"
func run() {
local := makeClient()
local.Load()
}
func makeClient() *workers.Client { return &workers.Client{} }
`,
		},
		{
			name: "typescript", language: "typescript", path: "src/run.ts", display: "local.load", line: 5, receiverType: "Client", importPath: "./worker",
			content: `import { Client } from "./worker";
function run(): void {
const marker = true;
const local = makeClient();
local.load();
}
function makeClient(): Client { return new Client(); }
`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			call := navigationCallAtLine(BuildNavigationGraph(test.content, test.language, test.path).Calls, test.display, test.line)
			if call.ReceiverType != test.receiverType || call.ImportPath != test.importPath {
				t.Fatalf("call-return binding context = %+v", call)
			}
		})
	}
}

func TestNavigationGraphDoesNotLeakNestedReturnBindings(t *testing.T) {
	content := `class Client { load(): void {} }
function owner(): void {
function hidden(): Client { return new Client(); }
}
function run(): void {
const local = hidden();
local.load();
}
`
	call := navigationCallAtLine(BuildNavigationGraph(content, "typescript", "src/run.ts").Calls, "local.load", 7)
	if call.ReceiverType != "" || call.ImportPath != "" {
		t.Fatalf("nested return binding leaked into sibling callable: %+v", call)
	}
}

func navigationCallAtLine(calls []NavigationCall, display string, line int) NavigationCall {
	for _, call := range calls {
		if call.Display == display && call.Line == line {
			return call
		}
	}
	return NavigationCall{}
}

func navigationCallsByDisplay(calls []NavigationCall) map[string]NavigationCall {
	result := make(map[string]NavigationCall, len(calls))
	for _, call := range calls {
		result[call.Display] = call
	}
	return result
}

func assertGoNavigationFieldFacts(t *testing.T, fields []NavigationField) {
	t.Helper()
	if len(fields) != 1 || fields[0].OwnerType != "Service" || fields[0].Name != "client" || fields[0].Type != "Client" || fields[0].ImportPath != "example.com/project/worker" || fields[0].Line != 3 || fields[0].Visibility != NavigationVisibilityNonPublic {
		t.Fatalf("Go field facts = %+v", fields)
	}
}

func TestNavigationFieldVisibilityRequiresPublicOwnerAndField(t *testing.T) {
	graph := BuildNavigationGraph(`package sample
type Public struct { Exported string; hidden string }
type private struct { Exported string }
`, "go", "model.go")
	visibility := map[string]NavigationVisibility{}
	for _, field := range graph.Fields {
		visibility[field.OwnerType+"."+field.Name] = field.Visibility
	}
	if visibility["Public.Exported"] != NavigationVisibilityPublic || visibility["Public.hidden"] != NavigationVisibilityNonPublic || visibility["private.Exported"] != NavigationVisibilityNonPublic {
		t.Fatalf("field visibility=%v", visibility)
	}
}
