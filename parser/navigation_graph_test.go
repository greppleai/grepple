package parser

import "testing"

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
	goCalls := navigationCallsByDisplay(BuildNavigationGraph(goContent, "go", "app/service.go").Calls)
	for _, display := range []string{"local.Load", "explicit.Load", "service.client.Load"} {
		call := goCalls[display]
		if call.ReceiverType != "Client" || call.ImportPath != "example.com/project/worker" {
			t.Fatalf("Go %s binding context = %+v", display, call)
		}
	}

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
