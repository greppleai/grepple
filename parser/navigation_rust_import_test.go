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
	for _, item := range graph.Imports {
		if item.Alias == "custom" || item.Alias == "inline" {
			t.Fatalf("non-conventional module was emitted: %#v", item)
		}
	}
	assertRustNavigationExport(t, graph, "PublicVisible", "crate::exports::Visible", "Visible")
	assertNavigationCallImport(t, graph, "run", "crate::foo::run", "run")
	assertNavigationCallImport(t, graph, "renamed", "crate::foo::other", "other")
	assertNavigationCallImport(t, graph, "local_alias", "self::local", "local")
	assertNavigationCallImport(t, graph, "Thing::new", "crate::foo::Thing", "new")
}

func TestRustNavigationCustomPathModulesRemainUnresolved(t *testing.T) {
	content := "#[cfg_attr(unix, path = \"unix.rs\")]\nmod platform;\n"
	graph := BuildNavigationGraph(content, "rust", "src/lib.rs")
	if len(graph.Imports) != 0 {
		t.Fatalf("custom-path module imports=%#v", graph.Imports)
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
	for _, item := range graph.Exports {
		if item.Name == "Private" || item.Name == "nested" || item.Name == "parent_visible" {
			t.Fatalf("non-exported Rust item=%#v", item)
		}
	}
}

func TestCachedRustNavigationFactsInstantiateRequestedPath(t *testing.T) {
	t.Setenv(NavigationCacheDirectoryEnv, t.TempDir())
	content := "mod feature;\npub use crate::feature::run;\npub fn boot() { run(); }\n"
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

func assertRustNavigationExport(t *testing.T, graph NavigationGraph, name, importPath, imported string) {
	t.Helper()
	for _, item := range graph.Exports {
		if item.Name == name && item.ImportPath == importPath && item.ImportedName == imported {
			return
		}
	}
	t.Fatalf("missing Rust export name=%q path=%q imported=%q: %#v", name, importPath, imported, graph.Exports)
}
