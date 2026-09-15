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

func TestNavigationGraphCapturesAdapterEvidencedGoEntrypointAndRoutes(t *testing.T) {
	content := "package main\nimport web \"net/http\"\nfunc main() { web.HandleFunc(\"GET /users\", users) }\nfunc users(web.ResponseWriter, *web.Request) {}\n"
	graph := BuildNavigationGraph(content, "go", "cmd/server/main.go")
	if len(graph.Declarations) != 2 || graph.Declarations[0].Entrypoint != "process" || graph.Declarations[1].Entrypoint != "" {
		t.Fatalf("declarations=%+v", graph.Declarations)
	}
	if len(graph.Routes) != 1 {
		t.Fatalf("routes=%+v", graph.Routes)
	}
	route := graph.Routes[0]
	if route.Method != "GET" || route.Pattern != "/users" || route.Handler != "users" || route.Framework != "net/http" || route.Path != "cmd/server/main.go" || route.CallerID != graph.Declarations[0].ID {
		t.Fatalf("route=%+v", route)
	}
}

func TestCachedNavigationGraphInstantiatesRoutePathsAndOwners(t *testing.T) {
	t.Setenv(NavigationCacheDirectoryEnv, t.TempDir())
	content := "package main\nimport web \"net/http\"\nfunc main() { web.HandleFunc(\"/ready\", ready) }\nfunc ready(web.ResponseWriter, *web.Request) {}\n"
	cold, _, _ := cachedTestNavigationGraph(t, content, "cmd/a/main.go")
	moved, _, hit := cachedTestNavigationGraph(t, content, "cmd/b/main.go")
	if !hit || len(cold.Routes) != 1 || len(moved.Routes) != 1 || moved.Routes[0].Path != "cmd/b/main.go" || moved.Routes[0].CallerID != moved.Declarations[0].ID || moved.Routes[0].CallerID == cold.Routes[0].CallerID {
		t.Fatalf("cold=%+v moved=%+v hit=%v", cold, moved, hit)
	}
}

func TestNavigationGraphDoesNotInferGoEntrypointsOrRoutesWithoutOwningSemantics(t *testing.T) {
	content := "package library\nimport web \"example.com/web\"\nfunc main() { web.HandleFunc(\"/users\", users) }\nfunc users() {}\n"
	graph := BuildNavigationGraph(content, "go", "library.go")
	if graph.Declarations[0].Entrypoint != "" || len(graph.Routes) != 0 {
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
