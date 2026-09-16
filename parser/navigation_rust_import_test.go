package parser

import "testing"

func TestRustNavigationImportsModulesGroupsAliasesAndReexports(t *testing.T) {
	content := `mod foo;
#[path = "custom/location.rs"]
mod custom;
mod inline { pub fn nested() {} }
use crate::foo::run;
use crate::foo::{Thing, other as renamed, self, *};
use super::shared;
use self::local as local_alias;
pub use crate::exports::Visible as PublicVisible;
fn use_all() {
    run();
    renamed();
    local_alias();
    Thing::new();
}
`
	graph := BuildNavigationGraph(content, "rust", "src/lib.rs")
	assertNavigationImport(t, graph, "foo", "self::foo", "*", 1)
	assertNavigationImport(t, graph, "run", "crate::foo::run", "run", 5)
	assertNavigationImport(t, graph, "Thing", "crate::foo::Thing", "Thing", 6)
	assertNavigationImport(t, graph, "renamed", "crate::foo::other", "other", 6)
	assertNavigationImport(t, graph, "foo", "crate::foo", "foo", 6)
	assertNavigationImport(t, graph, "*", "crate::foo", "*", 6)
	assertNavigationImport(t, graph, "shared", "super::shared", "shared", 7)
	assertNavigationImport(t, graph, "local_alias", "self::local", "local", 8)
	assertNavigationImport(t, graph, "PublicVisible", "crate::exports::Visible", "Visible", 9)
	assertRustModuleImport(t, graph, "custom", "", "custom/location.rs", false)
	assertRustModuleImport(t, graph, "inline", "", "", true)
	assertRustNavigationExport(t, graph, "PublicVisible", "crate::exports::Visible", "Visible")
	assertNavigationCallImport(t, graph, "run", "crate::foo::run", "run")
	assertNavigationCallImport(t, graph, "renamed", "crate::foo::other", "other")
	assertNavigationCallImport(t, graph, "local_alias", "self::local", "local")
	assertNavigationCallImport(t, graph, "Thing::new", "crate::foo::Thing", "new")
}

func TestRustNavigationConditionalPathModulesRemainUnresolved(t *testing.T) {
	content := "#[cfg_attr(unix, path = \"unix.rs\")]\nmod platform;\n"
	graph := BuildNavigationGraph(content, "rust", "src/lib.rs")
	if len(graph.Imports) != 0 {
		t.Fatalf("custom-path module imports=%#v", graph.Imports)
	}
}

func TestRustNavigationPathAttributePreservesSpaces(t *testing.T) {
	content := "#[path = \"platform/foo bar.rs\"]\nmod platform;\n"
	graph := BuildNavigationGraph(content, "rust", "src/lib.rs")
	assertRustModuleImport(t, graph, "platform", "", "platform/foo bar.rs", false)
}

func TestRustNavigationPathAttributeSupportsRawStrings(t *testing.T) {
	content := "#[path = r#\"platform/raw file.rs\"#]\nmod platform;\n"
	graph := BuildNavigationGraph(content, "rust", "src/lib.rs")
	assertRustModuleImport(t, graph, "platform", "", "platform/raw file.rs", false)
}

func TestRustNavigationMalformedRawPathRemainsUnresolved(t *testing.T) {
	content := "#[path = r#\"platform.rs\"]\nmod platform;\n"
	graph := BuildNavigationGraph(content, "rust", "src/lib.rs")
	if len(graph.Imports) != 0 {
		t.Fatalf("malformed raw-path module imports=%#v", graph.Imports)
	}
	if _, ok := rustNavigationStringLiteral(`r"platform.rs"trailing"`); ok {
		t.Fatal("raw path with an earlier terminator was accepted")
	}
}

func TestRustNavigationExportsOnlyPublicTopLevelItems(t *testing.T) {
	content := `pub struct Public;
struct Private;
pub(crate) fn crate_visible() {}
pub(super) fn parent_visible() {}
mod inline { pub fn nested() {} }
`
	graph := BuildNavigationGraph(content, "rust", "src/lib.rs")
	assertRustNavigationExport(t, graph, "Public", "", "")
	assertRustNavigationExport(t, graph, "crate_visible", "", "")
	assertRustNavigationExportAtScope(t, graph, "nested", "inline", "", "")
	for _, item := range graph.Exports {
		if item.Name == "Private" || item.Name == "parent_visible" {
			t.Fatalf("non-exported Rust item=%#v", item)
		}
	}
}

func TestCachedRustNavigationFactsInstantiateRequestedPath(t *testing.T) {
	t.Setenv(NavigationCacheDirectoryEnv, t.TempDir())
	content := "#[path = r#\"feature.rs\"#]\nmod feature;\npub use crate::feature::run;\npub fn boot() { run(); }\n"
	cold, _, coldHit, err := CachedNavigationGraph(content, "rust", "first/src/lib.rs")
	if err != nil {
		t.Fatal(err)
	}
	warm, _, warmHit, err := CachedNavigationGraph(content, "rust", "moved/src/lib.rs")
	if err != nil {
		t.Fatal(err)
	}
	if coldHit || !warmHit || len(cold.Imports) != 2 || len(warm.Imports) != 2 || len(cold.Exports) != 2 || len(warm.Exports) != 2 {
		t.Fatalf("coldHit=%v warmHit=%v cold=%#v warm=%#v", coldHit, warmHit, cold, warm)
	}
	for index := range cold.Imports {
		if cold.Imports[index].Path != "first/src/lib.rs" || warm.Imports[index].Path != "moved/src/lib.rs" {
			t.Fatalf("cold imports=%#v warm=%#v", cold.Imports, warm.Imports)
		}
	}
	for index := range cold.Exports {
		if cold.Exports[index].Path != "first/src/lib.rs" || warm.Exports[index].Path != "moved/src/lib.rs" {
			t.Fatalf("cold exports=%#v warm=%#v", cold.Exports, warm.Exports)
		}
	}
}

func TestRustNavigationScopesInlineModuleImportsAndCalls(t *testing.T) {
	content := `mod outer {
	use self::child::run;
	mod child {
		use super::helper;
		pub fn run() { helper(); }
	}
	pub fn helper() {}
	pub fn start() { run(); }
}
`
	graph := BuildNavigationGraph(content, "rust", "src/lib.rs")
	assertRustScopedImport(t, graph, "run", "outer", "self::outer::child::run")
	assertRustScopedImport(t, graph, "helper", "outer::child", "self::outer::helper")
	assertNavigationCallImport(t, graph, "run", "self::outer::child::run", "run")
	assertNavigationCallImport(t, graph, "helper", "self::outer::helper", "helper")
	for _, declaration := range graph.Declarations {
		if declaration.Name == "run" && declaration.Scope == "outer::child" || declaration.Name == "start" && declaration.Scope == "outer" {
			continue
		}
		if declaration.Name == "run" || declaration.Name == "start" {
			t.Fatalf("declaration scope=%#v", declaration)
		}
	}
}

func TestRustItemVisibilityUsesModuleAncestry(t *testing.T) {
	root := "src/lib.rs"
	module := func(path string) string { return rustModuleKey(root, rustModuleSegments(path)) }
	cases := []struct {
		visibility, declaration, source string
		visible                         bool
	}{
		{visibility: "pub", declaration: "model", source: "other", visible: true},
		{visibility: "pub(crate)", declaration: "model", source: "other", visible: true},
		{visibility: "", declaration: "model", source: "model::nested", visible: true},
		{visibility: "", declaration: "model", source: "other"},
		{visibility: "pub(super)", declaration: "parent::model", source: "parent::other", visible: true},
		{visibility: "pub(super)", declaration: "parent::model", source: "outside"},
		{visibility: "pub(self)", declaration: "model", source: "model::nested", visible: true},
		{visibility: "pub(self)", declaration: "model", source: "outside"},
		{visibility: "pub(in super)", declaration: "parent::model", source: "parent::other", visible: true},
		{visibility: "pub(in crate::allowed)", declaration: "allowed::model", source: "allowed::impls", visible: true},
		{visibility: "pub(in crate::allowed)", declaration: "allowed::model", source: "outside"},
		{visibility: "pub(in crate::outside)", declaration: "allowed::model", source: "outside"},
	}
	for _, test := range cases {
		if visible := RustItemVisibleFrom(module(test.declaration), module(test.source), test.visibility); visible != test.visible {
			t.Fatalf("visibility=%q declaration=%q source=%q visible=%v expected=%v", test.visibility, test.declaration, test.source, visible, test.visible)
		}
	}
}

func assertRustModuleImport(t *testing.T, graph NavigationGraph, alias, scope, hint string, inline bool) {
	t.Helper()
	for _, item := range graph.Imports {
		if item.Kind == "module" && item.Alias == alias && item.Scope == scope && item.TargetPathHint == hint && item.Inline == inline {
			return
		}
	}
	t.Fatalf("missing Rust module alias=%q scope=%q hint=%q inline=%v: %#v", alias, scope, hint, inline, graph.Imports)
}

func assertRustScopedImport(t *testing.T, graph NavigationGraph, alias, scope, importPath string) {
	t.Helper()
	for _, item := range graph.Imports {
		if item.Kind == "" && item.Alias == alias && item.Scope == scope && item.ImportPath == importPath {
			return
		}
	}
	t.Fatalf("missing Rust import alias=%q scope=%q path=%q: %#v", alias, scope, importPath, graph.Imports)
}

func assertRustNavigationExport(t *testing.T, graph NavigationGraph, name, importPath, imported string) {
	t.Helper()
	assertRustNavigationExportAtScope(t, graph, name, "", importPath, imported)
}

func assertRustNavigationExportAtScope(t *testing.T, graph NavigationGraph, name, scope, importPath, imported string) {
	t.Helper()
	for _, item := range graph.Exports {
		if item.Name == name && item.Scope == scope && item.ImportPath == importPath && item.ImportedName == imported {
			return
		}
	}
	t.Fatalf("missing Rust export name=%q scope=%q path=%q imported=%q: %#v", name, scope, importPath, imported, graph.Exports)
}
