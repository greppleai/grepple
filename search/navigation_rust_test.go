package search

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/greppleai/grepple/parser"
)

func TestRustModulesUsesReexportsAndSuperResolveWithinCrate(t *testing.T) {
	root := t.TempDir()
	lib := filepath.Join(root, "src", "lib.rs")
	foo := filepath.Join(root, "src", "foo.rs")
	nested := filepath.Join(root, "src", "nested", "mod.rs")
	child := filepath.Join(root, "src", "nested", "child.rs")
	paths := writeRustNavigationFiles(t, map[string]string{
		lib: `mod foo;
pub mod nested;
use crate::foo::{run, Worker};
use crate::nested::deep;
use serde::Serialize;
pub fn bootstrap() { run(); Worker::start(); deep(); }
pub fn typed(value: Worker) { value.work(); }
`,
		foo: `pub fn run() {}
pub struct Worker;
impl Worker { pub fn start() {} pub fn work(&self) {} }
`,
		nested: `pub mod child;
pub use self::child::deep;
pub fn helper() {}
`,
		child: `use super::helper;
pub fn deep() { helper(); }
`,
	})
	graph, _ := BuildNavigationGraphWithOptions(paths, NavigationBuildOptions{DisableCache: true})
	assertRustImportTargets(t, graph.Imports, lib, "self::foo", foo)
	assertRustImportTargets(t, graph.Imports, lib, "crate::foo::run", foo)
	assertRustImportTargets(t, graph.Imports, lib, "crate::foo::Worker", foo)
	assertRustImportTargets(t, graph.Imports, lib, "crate::nested::deep", nested)
	assertRustImportTargets(t, graph.Imports, child, "super::helper", nested)
	assertRustImportTargets(t, graph.Imports, lib, "serde::Serialize")
	assertRustResolvedCall(t, graph.Calls, lib, "run")
	assertRustResolvedCall(t, graph.Calls, lib, "Worker::start")
	assertRustResolvedCall(t, graph.Calls, lib, "value.work")
	assertRustResolvedCall(t, graph.Calls, lib, "deep")
	assertRustResolvedCall(t, graph.Calls, child, "helper")
}

func TestRustModulesPreserveAmbiguousConventionalTargets(t *testing.T) {
	root := t.TempDir()
	lib := filepath.Join(root, "src", "lib.rs")
	flat := filepath.Join(root, "src", "foo.rs")
	directory := filepath.Join(root, "src", "foo", "mod.rs")
	paths := writeRustNavigationFiles(t, map[string]string{
		lib:       "mod foo; pub fn use_it() { foo::run(); }",
		flat:      "pub fn run() {}",
		directory: "pub fn run() {}",
	})
	graph, _ := BuildNavigationGraphWithOptions(paths, NavigationBuildOptions{DisableCache: true})
	assertRustImportTargets(t, graph.Imports, lib, "self::foo", flat, directory)
	for _, call := range graph.Calls {
		if call.Path == lib && call.Display == "foo::run" && call.TargetID == "" && len(call.CandidateTargetIDs) == 2 && call.Confidence == "candidate" {
			return
		}
	}
	t.Fatalf("calls=%#v", graph.Calls)
}

func TestRustInlineAndExplicitPathModulesResolveScopedCalls(t *testing.T) {
	root := t.TempDir()
	lib := filepath.Join(root, "src", "lib.rs")
	platform := filepath.Join(root, "src", "platform", "unix.rs")
	external := filepath.Join(root, "src", "outer", "external.rs")
	paths := writeRustNavigationFiles(t, map[string]string{
		lib: `#[path = r#"platform/unix.rs"#]
		mod platform;
		mod outer {
			pub mod child { pub fn run() {} }
			pub fn run() {}
			pub fn call_child() { child::run(); }
			mod external;
			pub fn call_external() { external::run(); }
		}
		pub fn boot() { outer::run(); platform::start(); }
		`,
		platform: "pub fn start() {}",
		external: "pub fn run() {}",
	})
	graph, _ := BuildNavigationGraphWithOptions(paths, NavigationBuildOptions{DisableCache: true})
	assertRustImportTargets(t, graph.Imports, lib, "self::platform", platform)
	assertRustImportTargets(t, graph.Imports, lib, "self::outer", lib)
	assertRustImportTargets(t, graph.Imports, lib, "self::outer::child", lib)
	assertRustImportTargets(t, graph.Imports, lib, "self::outer::external", external)
	assertRustResolvedCallScope(t, graph, lib, "outer::run", lib, "outer")
	assertRustResolvedCallScope(t, graph, lib, "child::run", lib, "outer::child")
	assertRustResolvedCallScope(t, graph, lib, "external::run", external, "")
	assertRustResolvedCallScope(t, graph, lib, "platform::start", platform, "")
}

func assertRustResolvedCallScope(t *testing.T, graph parser.NavigationGraph, source, display, targetPath, targetScope string) {
	t.Helper()
	declarations := make(map[string]parser.NavigationDeclaration, len(graph.Declarations))
	for _, declaration := range graph.Declarations {
		declarations[declaration.ID] = declaration
	}
	for _, call := range graph.Calls {
		target := declarations[call.TargetID]
		if call.Path == source && call.Display == display && target.Path == targetPath && target.Scope == targetScope && call.Confidence == "import-resolved" {
			return
		}
	}
	t.Fatalf("missing resolved Rust call source=%q display=%q target=%q scope=%q: %#v", source, display, targetPath, targetScope, graph.Calls)
}

func TestRustRestrictedVisibilityControlsImportsAndCalls(t *testing.T) {
	root := t.TempDir()
	lib := filepath.Join(root, "src", "lib.rs")
	parent := filepath.Join(root, "src", "parent", "mod.rs")
	model := filepath.Join(root, "src", "parent", "model.rs")
	allowed := filepath.Join(root, "src", "parent", "allowed", "mod.rs")
	allowedModel := filepath.Join(root, "src", "parent", "allowed", "model.rs")
	allowedCaller := filepath.Join(root, "src", "parent", "allowed", "caller.rs")
	branch := filepath.Join(root, "src", "parent", "branch.rs")
	holder := filepath.Join(root, "src", "parent", "holder", "mod.rs")
	holderScoped := filepath.Join(root, "src", "parent", "holder", "scoped.rs")
	outside := filepath.Join(root, "src", "outside.rs")
	paths := writeRustNavigationFiles(t, map[string]string{
		lib: "pub(crate) mod parent; mod outside;",
		parent: `pub(crate) mod model;
pub(crate) mod allowed;
pub(in crate::parent) mod branch;
pub(crate) mod holder;
pub(in crate::parent) use self::model::parent_only as parent_exposed;
use self::model::parent_only;
use self::allowed::model::branch_only;
use self::branch::module_run;
use self::holder::scoped::super_run;
pub fn allowed_parent() { parent_only(); parent_exposed(); module_run(); super_run(); }
pub fn denied_branch() { branch_only(); }
`,
		model:        "pub(super) fn parent_only() {}",
		allowed:      "pub(crate) mod model; pub(crate) mod caller;",
		allowedModel: "pub(in crate::parent::allowed) fn branch_only() {}",
		allowedCaller: `use super::model::branch_only;
pub fn allowed_branch() { branch_only(); }
`,
		branch:       "pub fn module_run() {}",
		holder:       "pub(super) mod scoped;",
		holderScoped: "pub fn super_run() {}",
		outside: `use crate::parent::model::parent_only;
use crate::parent::parent_exposed;
use crate::parent::branch::module_run;
use crate::parent::holder::scoped::super_run;
pub fn denied_outside() { parent_only(); parent_exposed(); module_run(); super_run(); }
`,
	})
	graph, _ := BuildNavigationGraphWithOptions(paths, NavigationBuildOptions{DisableCache: true})
	assertRustImportTargets(t, graph.Imports, parent, "self::model::parent_only", model)
	assertRustImportTargets(t, graph.Imports, allowedCaller, "super::model::branch_only", allowedModel)
	assertRustImportTargets(t, graph.Imports, parent, "self::allowed::model::branch_only")
	assertRustImportTargets(t, graph.Imports, outside, "crate::parent::parent_exposed")
	assertRustImportTargets(t, graph.Imports, parent, "self::branch::module_run", branch)
	assertRustImportTargets(t, graph.Imports, parent, "self::holder::scoped::super_run", holderScoped)
	assertRustImportTargets(t, graph.Imports, outside, "crate::parent::model::parent_only")
	assertRustImportTargets(t, graph.Imports, outside, "crate::parent::branch::module_run")
	assertRustImportTargets(t, graph.Imports, outside, "crate::parent::holder::scoped::super_run")
	assertRustResolvedCallTarget(t, graph, parent, "parent_only", model)
	assertRustResolvedCallTarget(t, graph, parent, "parent_exposed", model)
	assertRustResolvedCallTarget(t, graph, allowedCaller, "branch_only", allowedModel)
	assertRustResolvedCallTarget(t, graph, parent, "module_run", branch)
	assertRustResolvedCallTarget(t, graph, parent, "super_run", holderScoped)
	assertRustUnresolvedCall(t, graph.Calls, parent, "branch_only")
	assertRustUnresolvedCall(t, graph.Calls, outside, "parent_only")
	assertRustUnresolvedCall(t, graph.Calls, outside, "parent_exposed")
	assertRustUnresolvedCall(t, graph.Calls, outside, "module_run")
	assertRustUnresolvedCall(t, graph.Calls, outside, "super_run")
}

func assertRustUnresolvedCall(t *testing.T, calls []parser.NavigationCall, source, display string) {
	t.Helper()
	for _, call := range calls {
		if call.Path == source && call.Display == display {
			if call.TargetID != "" || len(call.CandidateTargetIDs) != 0 || call.Confidence != "candidate" {
				t.Fatalf("Rust call unexpectedly resolved source=%q display=%q: %#v", source, display, call)
			}
			return
		}
	}
	t.Fatalf("missing Rust call source=%q display=%q: %#v", source, display, calls)
}

func TestRustRestrictedVisibilityUsesInlineCallerScope(t *testing.T) {
	root := t.TempDir()
	lib := filepath.Join(root, "src", "lib.rs")
	paths := writeRustNavigationFiles(t, map[string]string{
		lib: `mod outer {
	pub mod child { pub(super) fn limited() {} }
	pub fn allowed() { child::limited(); }
}
pub fn denied() { outer::child::limited(); }
`,
	})
	graph, _ := BuildNavigationGraphWithOptions(paths, NavigationBuildOptions{DisableCache: true})
	assertRustResolvedCallScope(t, graph, lib, "child::limited", lib, "outer::child")
	assertRustUnresolvedCall(t, graph.Calls, lib, "outer::child::limited")
}

func TestRustUnqualifiedCallsDoNotCrossCrateRoots(t *testing.T) {
	root := t.TempDir()
	firstRoot := filepath.Join(root, "first", "src", "lib.rs")
	secondRoot := filepath.Join(root, "second", "src", "lib.rs")
	paths := writeRustNavigationFiles(t, map[string]string{
		firstRoot:  "pub fn boot() { external_only(); }",
		secondRoot: "pub fn external_only() {}",
	})
	graph, _ := BuildNavigationGraphWithOptions(paths, NavigationBuildOptions{DisableCache: true})
	assertRustUnresolvedCall(t, graph.Calls, firstRoot, "external_only")
}

func TestRustModulesKeepCrateRootsIsolated(t *testing.T) {
	root := t.TempDir()
	firstRoot := filepath.Join(root, "first", "src", "lib.rs")
	firstTarget := filepath.Join(root, "first", "src", "feature.rs")
	secondRoot := filepath.Join(root, "second", "src", "lib.rs")
	secondTarget := filepath.Join(root, "second", "src", "feature.rs")
	paths := writeRustNavigationFiles(t, map[string]string{
		firstRoot:    "mod feature; use crate::feature::run; pub fn boot() { run(); }",
		firstTarget:  "pub fn run() {}",
		secondRoot:   "mod feature; use crate::feature::run; pub fn boot() { run(); }",
		secondTarget: "pub fn run() {}",
	})
	graph, _ := BuildNavigationGraphWithOptions(paths, NavigationBuildOptions{DisableCache: true})
	assertRustImportTargets(t, graph.Imports, firstRoot, "crate::feature::run", firstTarget)
	assertRustImportTargets(t, graph.Imports, secondRoot, "crate::feature::run", secondTarget)
	assertRustResolvedCallTarget(t, graph, firstRoot, "run", firstTarget)
	assertRustResolvedCallTarget(t, graph, secondRoot, "run", secondTarget)
}

func assertRustResolvedCallTarget(t *testing.T, graph parser.NavigationGraph, source, display, targetPath string) {
	t.Helper()
	declarations := make(map[string]parser.NavigationDeclaration, len(graph.Declarations))
	for _, declaration := range graph.Declarations {
		declarations[declaration.ID] = declaration
	}
	for _, call := range graph.Calls {
		if call.Path == source && call.Display == display && declarations[call.TargetID].Path == targetPath {
			return
		}
	}
	t.Fatalf("missing resolved Rust call source=%q display=%q target=%q: %#v", source, display, targetPath, graph.Calls)
}

func writeRustNavigationFiles(t *testing.T, files map[string]string) []string {
	t.Helper()
	paths := make([]string, 0, len(files))
	for path, content := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
	}
	return paths
}

func assertRustImportTargets(t *testing.T, imports []parser.NavigationImport, source, importPath string, expected ...string) {
	t.Helper()
	for _, item := range imports {
		if item.Path == source && item.ImportPath == importPath && equalStrings(item.TargetPaths, expected) {
			return
		}
	}
	t.Fatalf("missing Rust import source=%q path=%q targets=%#v: %#v", source, importPath, expected, imports)
}

func assertRustResolvedCall(t *testing.T, calls []parser.NavigationCall, source, display string) {
	t.Helper()
	for _, call := range calls {
		strong := call.Confidence == "exact" || call.Confidence == "import-resolved" || call.Confidence == "context-resolved"
		if call.Path == source && call.Display == display && call.TargetID != "" && strong {
			return
		}
	}
	t.Fatalf("missing resolved Rust call source=%q display=%q: %#v", source, display, calls)
}
