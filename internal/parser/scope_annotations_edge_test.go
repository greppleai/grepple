package parser

import "testing"

func TestCodeLineScopeAnnotationsWhileForAndSingleLine(t *testing.T) {
	csharp := "class A {\n void Run() {\n  while (ready) {\n   work();\n  }\n }\n}\n"
	a := CodeLineScopeAnnotations(csharp, "csharp", map[int]bool{4: true})[4]
	if a.Nearest == nil || *a.Nearest != (ScopeLineRange{"while", 3, 5}) || a.Scope == nil || *a.Scope != (ScopeLineRange{"method", 2, 6}) {
		t.Fatalf("csharp while: %+v", a)
	}
	python := "def run():\n    for item in items:\n        work(item)\n    done()\n"
	a = CodeLineScopeAnnotations(python, "python", map[int]bool{3: true})[3]
	if a.Nearest == nil || *a.Nearest != (ScopeLineRange{"for", 2, 3}) || a.Scope == nil || *a.Scope != (ScopeLineRange{"func", 1, 4}) {
		t.Fatalf("python for: %+v", a)
	}
	oneLine := "package p\nfunc run() { work() }\n"
	a = CodeLineScopeAnnotations(oneLine, "go", map[int]bool{2: true})[2]
	if a.Nearest == nil || *a.Nearest != (ScopeLineRange{"func", 2, 2}) || a.Scope == nil || *a.Scope != *a.Nearest {
		t.Fatalf("one-line function declaration: %+v", a)
	}
}

func TestCodeLineScopeAnnotationsRecoveredSiblingDoesNotTaintValidFunction(t *testing.T) {
	content := "package p\nfunc good() {\n work()\n}\nfunc bad( {\n"
	got := CodeLineScopeAnnotations(content, "go", map[int]bool{3: true, 5: true})
	if a := got[3]; a.Scope == nil || *a.Scope != (ScopeLineRange{"func", 2, 4}) {
		t.Fatalf("valid sibling lost: %+v", a)
	}
	if a := got[5]; a.Nearest != nil || a.Scope != nil {
		t.Fatalf("recovered syntax gained unproven bounds: %+v", a)
	}
}
