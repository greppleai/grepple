package extract

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestCFocusedStructureAndFlow(t *testing.T) {
	root := t.TempDir()
	source := Source{Path: filepath.Join(root, "service.c"), Text: `typedef struct InternalUser {
    int id;
    const char *name;
    long first, second;
} User;

enum State { READY, FAILED = 2 };

int helper(int value) { return value; }
int run(User *user) { return helper(user->id); }
`}
	analysis, err := Analyze([]Source{source})
	if err != nil {
		t.Fatal(err)
	}
	user := analysis.TSDeclarations[absolutePath(source.Path)+":User"]
	if user == nil || user.Language != "c" || user.Kind != "class" {
		t.Fatalf("C declaration=%#v", user)
	}
	assertJVMMember(t, user.Members, Member{Name: "id", Kind: "property", Type: "int", Visibility: "public"})
	assertJVMMember(t, user.Members, Member{Name: "name", Kind: "property", Type: "constchar*", Visibility: "public"})
	assertJVMMember(t, user.Members, Member{Name: "first", Kind: "property", Type: "long", Visibility: "public"})
	assertJVMMember(t, user.Members, Member{Name: "second", Kind: "property", Type: "long", Visibility: "public"})
	state := analysis.TSDeclarations[absolutePath(source.Path)+":State"]
	assertJVMMember(t, state.Members, Member{Name: "READY", Kind: "property", Type: "State", Visibility: "public"})
	assertJVMMember(t, state.Members, Member{Name: "FAILED", Kind: "property", Type: "State", Visibility: "public"})

	structure, err := GenerateClassDiagram("User", source, []Source{source}, GenerateOptions{Depth: 2, MaxNodes: 20})
	if err != nil {
		t.Fatal(err)
	}
	assertContainsAll(t, "C structure", structure, []string{"class User {", "<<c>> User", "+id: int", "+name: constchar*"})
	flow, err := GenerateFlowchart("run", source.Path, []Source{source}, GenerateOptions{Depth: 2, MaxNodes: 20})
	if err != nil {
		t.Fatal(err)
	}
	assertContainsAll(t, "C flow", flow, []string{"%% grepple:language run c", "run --> helper"})
	assertArchitectureRoundTrip(t, structure, flow, []Source{source})
}

func TestCPPFocusedStructureAndFlow(t *testing.T) {
	root := t.TempDir()
	source := Source{Path: filepath.Join(root, "service.cpp"), Text: `namespace app {
class Base { public: int id; };
class Worker : public Base {
public:
    int run(int value) { return helper(value); }
    static int count;
    void reset();
private:
    int helper(int value) { return value; }
    const char *name;
};
}
`}
	analysis, err := Analyze([]Source{source})
	if err != nil {
		t.Fatal(err)
	}
	worker := analysis.TSDeclarations[absolutePath(source.Path)+":app::Worker"]
	if worker == nil || worker.Language != "cpp" || worker.Kind != "class" {
		t.Fatalf("C++ declaration=%#v", worker)
	}
	if !worker.Extends["Base"] {
		t.Fatalf("C++ inheritance=%#v", worker.Extends)
	}
	assertJVMMember(t, worker.Members, Member{Name: "run", Kind: "method", Type: "int", Visibility: "public", Parameters: []string{"int"}})
	assertJVMMember(t, worker.Members, Member{Name: "count", Kind: "property", Type: "int", Visibility: "public", Static: true})
	assertJVMMember(t, worker.Members, Member{Name: "reset", Kind: "method", Type: "void", Visibility: "public"})
	assertJVMMember(t, worker.Members, Member{Name: "helper", Kind: "method", Type: "int", Visibility: "private", Parameters: []string{"int"}})
	assertJVMMember(t, worker.Members, Member{Name: "name", Kind: "property", Type: "constchar*", Visibility: "private"})

	structure, err := GenerateClassDiagram("Worker", source, []Source{source, source}, GenerateOptions{Depth: 2, MaxNodes: 20})
	if err != nil {
		t.Fatal(err)
	}
	assertContainsAll(t, "C++ structure", structure, []string{"class Worker {", "class Base {", "<<cpp>> Worker", "Base <|-- Worker", "+run(int): int", "-helper(int): int", "+count: int", "-name: constchar*"})
	flow, err := GenerateFlowchart("Worker.run", source.Path, []Source{source}, GenerateOptions{Depth: 2, MaxNodes: 20})
	if err != nil {
		t.Fatal(err)
	}
	assertContainsAll(t, "C++ flow", flow, []string{"%% grepple:language Worker_run cpp", "Worker_run --> Worker_helper"})
	assertArchitectureRoundTrip(t, structure, flow, []Source{source})
}

func TestCPPOverloadsRemainAmbiguous(t *testing.T) {
	source := Source{Path: "overload.cpp", Text: `class Worker {
public:
    int run() { return helper(1); }
private:
    int helper(int value) { return value; }
    int helper(const char *value) { return 0; }
};
`}
	analysis, err := Analyze([]Source{source})
	if err != nil {
		t.Fatal(err)
	}
	for _, call := range analysis.Navigation.Calls {
		if call.Display == "helper" && call.TargetID != "" {
			t.Fatalf("overloaded C++ call was guessed: %#v", call)
		}
	}
	flow, err := GenerateFlowchart("Worker.run", source.Path, []Source{source}, GenerateOptions{Depth: 2, MaxNodes: 20})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(flow, "Worker_run --> Worker_helper") {
		t.Fatalf("overloaded C++ edge was generated:\n%s", flow)
	}
}

func TestCFamilyLeavesPreprocessorAndTemplatesUnmodeled(t *testing.T) {
	cSource := Source{Path: "model.c", Text: `#include "generated.h"
union Value { int number; const char *text; };
`}
	cAnalysis, err := Analyze([]Source{cSource})
	if err != nil {
		t.Fatal(err)
	}
	if cAnalysis.TSDeclarations[absolutePath(cSource.Path)+":Value"] == nil || len(cAnalysis.Navigation.Imports) != 0 {
		t.Fatalf("C conservative analysis declarations=%#v imports=%#v", cAnalysis.TSDeclarations, cAnalysis.Navigation.Imports)
	}

	cppSource := Source{Path: "model.cpp", Text: `template <typename T> class Box { T value; };
class Plain { int value; };
`}
	cppAnalysis, err := Analyze([]Source{cppSource})
	if err != nil {
		t.Fatal(err)
	}
	if cppAnalysis.Declarations["Box"] != nil || cppAnalysis.Declarations["Plain"] == nil {
		t.Fatalf("C++ template projection=%#v", cppAnalysis.Declarations)
	}
}
