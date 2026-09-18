package search

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/greppleai/grepple/parser"
)

func TestRelatedGoCallsResolveProjectDeclaration(t *testing.T) {
	directory := t.TempDir()
	caller := writeGoFixture(t, directory, "caller.go", `package related
// ADVANCED_DOC: run delegates to helper.
func run() string {
	return helper("needle")
}
func unrelated() { ignored() }
`)
	helper := writeGoFixture(t, directory, "helper.go", `package related
func helper(value string) string {
	return value
}
func ignored() {}
`)

	params := Params{Query: `return helper("needle")`, Related: true}
	matches, err := Files(params, []string{caller, helper})
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("matches = %d, want 1", len(matches))
	}
	if len(matches[0].Related) != 1 {
		t.Fatalf("related = %#v, want one helper", matches[0].Related)
	}
	point := matches[0].Related[0]
	if point.Name != "helper" || point.Kind != "func" || point.Confidence != "unique-terminal" || point.CallLine != 4 {
		t.Fatalf("unexpected related point %#v", point)
	}
	if !strings.HasSuffix(point.Path, "helper.go") || point.Start != 2 || point.End != 4 {
		t.Fatalf("unexpected helper location %#v", point)
	}

	results := BuildResults(matches, 0, 0, true)
	if len(results[0].Related) != 1 || results[0].Related[0].Name != "helper" {
		t.Fatalf("related result was not preserved: %#v", results[0].Related)
	}
}

func TestNavigationImportsResolveLocalTargetPaths(t *testing.T) {
	root := t.TempDir()
	for _, directory := range []string{"search", "rules"} {
		if err := os.MkdirAll(filepath.Join(root, directory), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	target := writeGoFixture(t, root, "search/request.go", "package search\ntype Request struct{}\n")
	caller := writeGoFixture(t, root, "rules/rule.go", "package rules\nimport \"example.com/project/search\"\nfunc Validate(search.Request) {}\n")
	graph, stats := BuildNavigationGraphWithOptions([]string{caller, target}, NavigationBuildOptions{DisableCache: true})
	if stats.Parsed != 2 || len(graph.Imports) != 1 {
		t.Fatalf("stats=%+v imports=%#v", stats, graph.Imports)
	}
	if len(graph.Imports[0].TargetPaths) != 1 || graph.Imports[0].TargetPaths[0] != target {
		t.Fatalf("resolved import=%#v", graph.Imports[0])
	}
}

func TestRelatedGoTypesExpandImportedParametersAndResults(t *testing.T) {
	root := t.TempDir()
	mustMkdirAll(t, filepath.Join(root, "navigation"))
	mustMkdirAll(t, filepath.Join(root, "parser"))
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeGoFixture(t, root, "parser/types.go", `package parser

type Request struct {
	Name string
}
type Response struct {
	Accepted bool
}
`)
	writeGoFixture(t, root, "navigation/run.go", `package navigation
import "example.com/project/parser"
func Run(request parser.Request) parser.Response {
	_ = request // run-needle
	return parser.Response{Accepted: true}
}
func Echo(request parser.Request) parser.Request {
	_ = request // echo-needle
	return request
}
`)
	t.Chdir(root)
	matches, err := Files(Params{Query: "run-needle", Root: root, Globs: []string{"navigation/**"}, Related: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || len(matches[0].Related) != 2 {
		t.Fatalf("related type points=%#v", matches)
	}
	parameter, result := matches[0].Related[0], matches[0].Related[1]
	if parameter.Name != "Request" || parameter.Direction != "type" || parameter.Role != "parameter" || parameter.Confidence != "import-resolved" || parameter.Preview == nil || parameter.Preview.Start != 3 || parameter.Preview.End != 5 {
		t.Fatalf("parameter type=%#v", parameter)
	}
	if result.Name != "Response" || result.Direction != "type" || result.Role != "result" || result.Confidence != "import-resolved" || result.Preview == nil || result.Preview.Start != 6 || result.Preview.End != 8 {
		t.Fatalf("result type=%#v", result)
	}
	wire := BuildResults(matches, 0, 0, true)[0].Related
	if len(wire) != 2 || wire[0].Role != "parameter" || len(wire[0].Segments) != 1 || !strings.Contains(wire[0].Segments[0].Text, "type Request struct") {
		t.Fatalf("wire type previews=%#v", wire)
	}
	echoMatches, err := Files(Params{Query: "echo-needle", Root: root, Globs: []string{"navigation/**"}, Related: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(echoMatches) != 1 || len(echoMatches[0].Related) != 2 || echoMatches[0].Related[0].Role != "parameter" || echoMatches[0].Related[1].Role != "result" {
		t.Fatalf("same declaration type roles=%#v", echoMatches)
	}
}

func TestRelatedGoCallsPreferFunctionForUnqualifiedCall(t *testing.T) {
	directory := t.TempDir()
	path := writeGoFixture(t, directory, "analysis.go", `package related
type First struct{}
func (First) Analyze() {}
type Second struct{}
func (Second) Analyze() {}
func Analyze() {}
func run() { Analyze() } // needle
`)
	params := Params{Query: "needle", Related: true}
	matches, err := Files(params, []string{path})
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || len(matches[0].Related) != 1 {
		t.Fatalf("expected one contextual candidate: %#v", matches)
	}
	point := matches[0].Related[0]
	if point.Name != "Analyze" || point.Kind != "func" || point.Confidence != "context-resolved" {
		t.Fatalf("unexpected contextual resolution: %#v", point)
	}
}

func TestRelatedGoCallsUseExactQualifiedIdentity(t *testing.T) {
	directory := t.TempDir()
	path := writeGoFixture(t, directory, "qualified.go", `package related
type First struct{}
func (First) Load() {}
type Second struct{}
func (Second) Load() {}
func run() { First.Load(); /* needle */ }
`)
	matches, err := Files(Params{Query: "needle", Related: true}, []string{path})
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || len(matches[0].Related) != 1 {
		t.Fatalf("expected one exact qualified target: %#v", matches)
	}
	point := matches[0].Related[0]
	if point.Name != "First.Load" || point.Confidence != "exact" {
		t.Fatalf("unexpected exact qualified resolution: %#v", point)
	}
}

func TestRelatedGoCallsLabelAmbiguousMethodsAsCandidates(t *testing.T) {
	directory := t.TempDir()
	caller := writeGoFixture(t, directory, "caller.go", `package related
func run(service interface{ Load() error }) error {
	return service.Load()
}
`)
	first := writeGoFixture(t, directory, "first.go", `package related
type First struct{}
func (*First) Load() error { return nil }
`)
	second := writeGoFixture(t, directory, "second.go", `package related
type Second struct{}
func (*Second) Load() error { return nil }
`)

	params := Params{Query: "return service.Load()", Related: true}
	matches, err := Files(params, []string{caller, first, second})
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || len(matches[0].Related) != 2 {
		t.Fatalf("matches or candidates unexpected: %#v", matches)
	}
	for _, point := range matches[0].Related {
		if point.Confidence != "candidate" || !strings.HasPrefix(point.Name, "service.Load → ") {
			t.Fatalf("unexpected ambiguous method point %#v", point)
		}
	}
	reversed, err := Files(params, []string{second, first, caller})
	if err != nil {
		t.Fatal(err)
	}
	if len(reversed) != 1 || len(reversed[0].Related) != len(matches[0].Related) {
		t.Fatalf("reversed candidate shape changed: %#v", reversed)
	}
	for index, point := range matches[0].Related {
		other := reversed[0].Related[index]
		if point.Name != other.Name || point.Path != other.Path || point.Confidence != other.Confidence {
			t.Fatalf("candidate order depends on input order: first=%#v reversed=%#v", matches[0].Related, reversed[0].Related)
		}
	}
}
func TestRelatedGoCallsResolveInterfaceMethodsAndFunctionFields(t *testing.T) {
	directory := t.TempDir()
	path := writeGoFixture(t, directory, "service.go", `package related
type Recorder interface { Record(string) }
type Service struct { recorder Recorder; deliver func(string) }
func (s Service) run(value string) {
	s.recorder.Record(value)
	s.deliver(value) // needle
}
`)

	params := Params{Query: "needle", Related: true}
	matches, err := Files(params, []string{path})
	if err != nil {
		t.Fatal(err)
	}
	calls := relatedCallPoints(matches[0].Related)
	if len(matches) != 1 || len(calls) != 2 {
		t.Fatalf("expected interface method and function field, got %#v", matches)
	}
	if calls[0].Kind != "field" || calls[1].Kind != "method" {
		t.Fatalf("unexpected callable kinds: %#v", matches[0].Related)
	}
	serviceType := findRelatedPoint(t, matches[0].Related, "Service", "type")
	if serviceType.Role != "receiver" {
		t.Fatalf("Go receiver type role=%#v", serviceType)
	}
}

func TestRelatedGoCallsIncludeCallers(t *testing.T) {
	directory := t.TempDir()
	caller := writeGoFixture(t, directory, "caller.go", `package related
func run() string { return helper() }
`)
	helper := writeGoFixture(t, directory, "helper.go", `package related
func helper() string {
    return "needle"
}
`)

	params := Params{Query: `return "needle"`, Related: true}
	matches, err := Files(params, []string{caller, helper})
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || len(matches[0].Related) != 1 {
		t.Fatalf("expected one caller, got %#v", matches)
	}
	point := matches[0].Related[0]
	if point.Direction != "caller" || point.Name != "run" || point.CallLine != 2 {
		t.Fatalf("unexpected caller %#v", point)
	}
}

func TestFollowRelatedExpandsBoundedCallChain(t *testing.T) {
	directory := t.TempDir()
	root := writeGoFixture(t, directory, "root.go", `package related
func root() string { return first() + "needle" }
`)
	first := writeGoFixture(t, directory, "first.go", `package related
func first() string { return second() }
`)
	second := writeGoFixture(t, directory, "second.go", `package related
func second() string { return "done" }
`)

	params := Params{Query: "needle", Related: true, FollowRelated: 2}
	matches, err := Files(params, []string{root, first, second})
	if err != nil {
		t.Fatal(err)
	}
	firstPoint := findRelatedPoint(t, matches[0].Related, "first", "callee")
	if firstPoint.Preview == nil {
		t.Fatal("first callee was not expanded")
	}
	secondPoint := findRelatedPoint(t, firstPoint.Preview.Related, "second", "callee")
	if secondPoint.Preview == nil {
		t.Fatal("second callee was not expanded at depth two")
	}
}

func TestFollowRelatedExpandsCallersAndCallees(t *testing.T) {
	directory := t.TempDir()
	caller := writeGoFixture(t, directory, "caller.go", "package related\nfunc caller() string { return target() }\n")
	target := writeGoFixture(t, directory, "target.go", "package related\nfunc target() string { return callee() + \"FOLLOW_BOTH_NEEDLE\" }\n")
	callee := writeGoFixture(t, directory, "callee.go", "package related\nfunc callee() string { return \"done\" }\n")
	matches, err := Files(Params{Query: "FOLLOW_BOTH_NEEDLE", Related: true, FollowRelated: 1}, []string{caller, target, callee})
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("matches=%#v", matches)
	}
	callerPoint := findRelatedPoint(t, matches[0].Related, "caller", "caller")
	calleePoint := findRelatedPoint(t, matches[0].Related, "callee", "callee")
	if callerPoint.Preview == nil || calleePoint.Preview == nil {
		t.Fatalf("caller/callee previews caller=%#v callee=%#v", callerPoint, calleePoint)
	}
}

func findRelatedPoint(t *testing.T, points []RelatedPoint, name, direction string) RelatedPoint {
	t.Helper()
	for _, point := range points {
		if point.Name == name && point.Direction == direction {
			return point
		}
	}
	t.Fatalf("related point %s/%s not found in %#v", direction, name, points)
	return RelatedPoint{}
}

func relatedCallPoints(points []RelatedPoint) []RelatedPoint {
	var calls []RelatedPoint
	for _, point := range points {
		if point.Direction == "caller" || point.Direction == "callee" {
			calls = append(calls, point)
		}
	}
	return calls
}

func TestRelatedGoCallsUseImportAndReceiverTypeContext(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	workerDirectory := filepath.Join(directory, "worker")
	otherDirectory := filepath.Join(directory, "other")
	appDirectory := filepath.Join(directory, "app")
	for _, path := range []string{workerDirectory, otherDirectory, appDirectory} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	worker := writeGoFixture(t, workerDirectory, "worker.go", `package worker
func Start() {}
type Client struct{}
func (*Client) Load() {}
`)
	other := writeGoFixture(t, otherDirectory, "other.go", `package worker
func Start() { _ = "OTHER_NEEDLE" }
type Client struct{}
func (*Client) Load() { _ = "OTHER_METHOD_NEEDLE" }
`)
	caller := writeGoFixture(t, appDirectory, "caller.go", `package app
import workers "example.com/project/worker"
type Runner struct { client *workers.Client }
func (runner *Runner) run(client *workers.Client) {
local := &workers.Client{}
workers.Start()
client.Load()
local.Load()
runner.client.Load() // CALLER_NEEDLE
if true {
nested := &workers.Client{}
nested.Load() // NESTED_CALLER_NEEDLE
}
}
`)
	files := []string{caller, other, worker}
	matches, err := Files(Params{Query: "CALLER_NEEDLE", Related: true}, files)
	if err != nil {
		t.Fatal(err)
	}
	start := findRelatedPoint(t, matches[0].Related, "workers.Start → Start", "callee")
	if start.Confidence != "import-resolved" || !strings.HasSuffix(start.Path, "worker/worker.go") {
		t.Fatalf("unexpected imported function resolution: %#v", start)
	}
	load := findRelatedPoint(t, matches[0].Related, "runner.client.Load → Client.Load", "callee")
	if load.Confidence != "import-resolved" || !strings.HasSuffix(load.Path, "worker/worker.go") {
		t.Fatalf("unexpected receiver resolution: %#v", load)
	}

	assertNestedGoReceiverResolution(t, files)

	otherMatches, err := Files(Params{Query: "OTHER_NEEDLE", Related: true}, files)
	if err != nil {
		t.Fatal(err)
	}
	if len(otherMatches) != 1 || len(otherMatches[0].Related) != 0 {
		t.Fatalf("import-resolved call was attributed to the other package: %#v", otherMatches)
	}
	otherMethodMatches, err := Files(Params{Query: "OTHER_METHOD_NEEDLE", Related: true}, files)
	if err != nil {
		t.Fatal(err)
	}
	if len(otherMethodMatches) != 1 || len(otherMethodMatches[0].Related) != 0 {
		t.Fatalf("receiver-resolved call was attributed to the other package: %#v", otherMethodMatches)
	}
}

func TestRelatedGoCallsPropagateTypedFieldsAcrossFiles(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	workerDirectory := filepath.Join(directory, "worker")
	appDirectory := filepath.Join(directory, "app")
	for _, path := range []string{workerDirectory, appDirectory} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	worker := writeGoFixture(t, workerDirectory, "worker.go", `package worker
type Client struct{}
func (*Client) Load() {}
`)
	fields := writeGoFixture(t, appDirectory, "fields.go", `package app
import workers "example.com/project/worker"
type Runner struct { client *workers.Client }
`)
	caller := writeGoFixture(t, appDirectory, "caller.go", `package app
func (runner *Runner) run() {
runner.client.Load() // CROSS_FILE_FIELD
}
`)
	graph := BuildNavigationGraph([]string{caller, fields, worker})
	var resolved parser.NavigationCall
	for _, call := range graph.Calls {
		if call.Display == "runner.client.Load" {
			resolved = call
			break
		}
	}
	if resolved.ReceiverType != "Client" || resolved.ImportPath != "example.com/project/worker" || resolved.Confidence != "import-resolved" || resolved.TargetID == "" {
		t.Fatalf("cross-file typed field call=%+v", resolved)
	}
}

func TestRelatedGoCallsResolvePromotedMethods(t *testing.T) {
	directory := t.TempDir()
	base := writeGoFixture(t, directory, "base.go", `package promoted
type Base struct{}
func (*Base) Load() {}
type Other struct{}
func (*Other) Load() {}
`)
	outer := writeGoFixture(t, directory, "outer.go", `package promoted
type Outer struct { *Base }
`)
	caller := writeGoFixture(t, directory, "caller.go", `package promoted
func run(value *Outer) { value.Load() }
`)
	graph := BuildNavigationGraph([]string{base, outer, caller})
	var resolved parser.NavigationCall
	for _, call := range graph.Calls {
		if call.Display == "value.Load" {
			resolved = call
			break
		}
	}
	if resolved.Confidence != "context-resolved" || resolved.TargetID == "" || len(resolved.CandidateTargetIDs) != 0 {
		t.Fatalf("promoted method call=%+v", resolved)
	}
	target := map[string]parser.NavigationDeclaration{}
	for _, declaration := range graph.Declarations {
		target[declaration.ID] = declaration
	}
	if target[resolved.TargetID].Name != "Base.Load" {
		t.Fatalf("promoted target=%+v", target[resolved.TargetID])
	}
}

func TestRelatedGoCallsUseImportedReturnTypes(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	workerDirectory := filepath.Join(directory, "worker")
	modelDirectory := filepath.Join(directory, "model")
	for _, path := range []string{workerDirectory, modelDirectory} {
		if err := os.Mkdir(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	model := writeGoFixture(t, modelDirectory, "model.go", `package model
type Client struct{}
func (*Client) Load() {}
`)
	worker := writeGoFixture(t, workerDirectory, "worker.go", `package worker
import models "example.com/project/model"
func NewClient() *models.Client { return &models.Client{} }
`)
	caller := writeGoFixture(t, directory, "caller.go", `package app
import workers "example.com/project/worker"
func run() {
returned := workers.NewClient()
returned.Load() // RETURN_CALLER_NEEDLE
}
`)
	matches, err := Files(Params{Query: "RETURN_CALLER_NEEDLE", Related: true}, []string{caller, model, worker})
	if err != nil {
		t.Fatal(err)
	}
	point := findRelatedPoint(t, matches[0].Related, "returned.Load → Client.Load", "callee")
	if point.Confidence != "import-resolved" || !strings.HasSuffix(point.Path, "model/model.go") {
		t.Fatalf("unexpected call-return receiver resolution: %#v", point)
	}
}

func assertNestedGoReceiverResolution(t *testing.T, files []string) {
	t.Helper()
	matches, err := Files(Params{Query: "NESTED_CALLER_NEEDLE", Related: true}, files)
	if err != nil {
		t.Fatal(err)
	}
	nested := findRelatedPoint(t, matches[0].Related, "nested.Load → Client.Load", "callee")
	if nested.Confidence != "import-resolved" || !strings.HasSuffix(nested.Path, "worker/worker.go") {
		t.Fatalf("unexpected nested receiver resolution: %#v", nested)
	}
}

func TestRelatedReportsOmittedCallerAndCalleeCounts(t *testing.T) {
	directory := t.TempDir()
	path := writeGoFixture(t, directory, "many.go", `package related
func focus() { // TARGET_NEEDLE
callee0(); callee1(); callee2(); callee3(); callee4(); callee5()
}
func callee0() {}
func callee1() {}
func callee2() {}
func callee3() {}
func callee4() {}
func callee5() {}
func caller0() { focus() }
func caller1() { focus() }
func caller2() { focus() }
func caller3() { focus() }
func caller4() { focus() }
func caller5() { focus() }
`)
	matches, err := Files(Params{Query: "TARGET_NEEDLE", Related: true}, []string{path})
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || len(matches[0].Related) != 2*maxRelatedPoints {
		t.Fatalf("related matches = %#v", matches)
	}
	if matches[0].OmittedRelatedCallers != 1 || matches[0].OmittedRelatedCallees != 1 {
		t.Fatalf("omitted callers=%d callees=%d", matches[0].OmittedRelatedCallers, matches[0].OmittedRelatedCallees)
	}
	result := BuildResults(matches, 0, 0, true)[0]
	if result.OmittedRelatedCallers != 1 || result.OmittedRelatedCallees != 1 {
		t.Fatalf("result omissions = %#v", result)
	}
}
func TestBuildNavigationGraphEnrichesGoRepositoryIdentity(t *testing.T) {
	root := t.TempDir()
	writeGoFixture(t, root, "go.mod", "module example.com/project\n")
	packageDir := filepath.Join(root, "parser")
	if err := os.MkdirAll(packageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := writeGoFixture(t, packageDir, "document.go", "package parser\nfunc Parse() {}\n")

	graph := BuildNavigationGraph([]string{source})
	if len(graph.Declarations) != 1 {
		t.Fatalf("declarations = %#v", graph.Declarations)
	}
	declaration := graph.Declarations[0]
	if declaration.ModuleID != "example.com/project" || declaration.PackageID != "example.com/project/parser" {
		t.Fatalf("repository identity = module %q package %q", declaration.ModuleID, declaration.PackageID)
	}
}

func TestAnalyzeTypeBoundariesTreatsCurrentModuleImportAsFirstParty(t *testing.T) {
	root := t.TempDir()
	writeGoFixture(t, root, "go.mod", "module example.com/project\n")
	parserDir := filepath.Join(root, "parser")
	consumerDir := filepath.Join(root, "consumer")
	if err := os.MkdirAll(parserDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(consumerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	parserFile := writeGoFixture(t, parserDir, "document.go", "package parser\ntype Document struct{}\nfunc New() Document { return Document{} }\n")
	first := writeGoFixture(t, consumerDir, "first.go", "package consumer\nimport \"example.com/project/parser\"\nfunc First(v parser.Document) {}\n")
	second := writeGoFixture(t, consumerDir, "second.go", "package consumer\nimport \"example.com/project/parser\"\nfunc Second(v parser.Document) {}\n")

	graph := BuildNavigationGraph([]string{parserFile, first, second})
	spreads, err := AnalyzeTypeBoundaries(graph, 2)
	if err != nil {
		t.Fatal(err)
	}
	for _, spread := range spreads {
		if spread.ImportPath == "example.com/project/parser" {
			if spread.Origin != BoundaryTypeOriginFirstParty {
				t.Fatalf("origin = %q, want first-party: %#v", spread.Origin, spread)
			}
			return
		}
	}
	t.Fatalf("first-party parser spread missing: %#v", spreads)
}

func TestRelatedUsesAbsoluteRepositoryRootForDisplayPaths(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "owner", "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	writeGoFixture(t, repo, "caller.go", "package sample\nfunc caller() { helper() } // REMOTE_NEEDLE\n")
	writeGoFixture(t, repo, "helper.go", "package sample\nfunc helper() {}\n")
	matches, err := Files(Params{Root: root, Repo: []string{"owner/repo"}, Query: "REMOTE_NEEDLE", Related: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || matches[0].DisplayPath != "owner/repo/caller.go" || len(matches[0].Related) == 0 {
		t.Fatalf("remote-root navigation = %#v", matches)
	}
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func writeGoFixture(t *testing.T, directory, name, content string) string {
	t.Helper()
	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
