package search

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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

	params := Params{Query: `return helper("needle")`, MaxSegments: 20, Related: true}
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

	results := BuildResults(matches, 0, 0, 20, true)
	if len(results[0].Related) != 1 || results[0].Related[0].Name != "helper" {
		t.Fatalf("related result was not preserved: %#v", results[0].Related)
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
	params := Params{Query: "needle", MaxSegments: 20, Related: true}
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
	matches, err := Files(Params{Query: "needle", MaxSegments: 20, Related: true}, []string{path})
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

	params := Params{Query: "return service.Load()", MaxSegments: 20, Related: true}
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

	params := Params{Query: "needle", MaxSegments: 20, Related: true}
	matches, err := Files(params, []string{path})
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || len(matches[0].Related) != 2 {
		t.Fatalf("expected interface method and function field, got %#v", matches)
	}
	if matches[0].Related[0].Kind != "field" || matches[0].Related[1].Kind != "method" {
		t.Fatalf("unexpected callable kinds: %#v", matches[0].Related)
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

	params := Params{Query: `return "needle"`, MaxSegments: 20, Related: true}
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

	params := Params{Query: "needle", MaxSegments: 20, Related: true, FollowRelated: 2}
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
func run(client *workers.Client) {
workers.Start()
client.Load() // CALLER_NEEDLE
}
`)
	files := []string{caller, other, worker}
	matches, err := Files(Params{Query: "CALLER_NEEDLE", MaxSegments: 20, Related: true}, files)
	if err != nil {
		t.Fatal(err)
	}
	start := findRelatedPoint(t, matches[0].Related, "workers.Start → Start", "callee")
	if start.Confidence != "import-resolved" || !strings.HasSuffix(start.Path, "worker/worker.go") {
		t.Fatalf("unexpected imported function resolution: %#v", start)
	}
	load := findRelatedPoint(t, matches[0].Related, "client.Load → Client.Load", "callee")
	if load.Confidence != "import-resolved" || !strings.HasSuffix(load.Path, "worker/worker.go") {
		t.Fatalf("unexpected receiver resolution: %#v", load)
	}

	otherMatches, err := Files(Params{Query: "OTHER_NEEDLE", MaxSegments: 20, Related: true}, files)
	if err != nil {
		t.Fatal(err)
	}
	if len(otherMatches) != 1 || len(otherMatches[0].Related) != 0 {
		t.Fatalf("import-resolved call was attributed to the other package: %#v", otherMatches)
	}
	otherMethodMatches, err := Files(Params{Query: "OTHER_METHOD_NEEDLE", MaxSegments: 20, Related: true}, files)
	if err != nil {
		t.Fatal(err)
	}
	if len(otherMethodMatches) != 1 || len(otherMethodMatches[0].Related) != 0 {
		t.Fatalf("receiver-resolved call was attributed to the other package: %#v", otherMethodMatches)
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
