package rustmodule

import (
	"path/filepath"
	"testing"

	"github.com/greppleai/grepple/internal/parser"
)

func TestBuildRustModuleIndexResolvesSelectedModules(t *testing.T) {
	root := filepath.Join(t.TempDir(), "src")
	mainFile, helperFile := filepath.Join(root, "lib.rs"), filepath.Join(root, "helper.rs")
	graph := parser.BuildNavigationGraph("mod helper;\nuse self::helper::run;\nfn main() { run(); }\n", "rust", mainFile)
	graph.Merge(parser.BuildNavigationGraph("pub fn run() {}\n", "rust", helperFile))
	index := BuildRustModuleIndex(graph, []string{mainFile, helperFile})
	targets := index.ResolveImportFrom(mainFile, "", "self::helper::run")
	if len(targets) != 1 || targets[0].Path != helperFile || targets[0].ModulePath != "helper" {
		t.Fatalf("resolved imports=%+v", targets)
	}
	if keys := index.ModuleKeys(helperFile, ""); len(keys) != 1 || keys[0] != RustModuleTargetModuleKey(targets[0]) {
		t.Fatalf("module keys=%v targets=%+v", keys, targets)
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
		{visibility: "pub(super)", declaration: "", source: "other"},
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

func TestRustCrateRootPath(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{filepath.Join("project", "src", "lib.rs"), true},
		{filepath.Join("project", "src", "main.rs"), true},
		{filepath.Join("project", "src", "bin", "cli.rs"), true},
		{filepath.Join("project", "src", "bin", "cli", "main.rs"), true},
		{filepath.Join("project", "src", "helper.rs"), false},
		{filepath.Join("project", "tests", "main.rs"), false},
		{filepath.Join("project", "src", "lib.go"), false},
	}
	for _, test := range cases {
		if got := rustCrateRootPath(test.path); got != test.want {
			t.Errorf("rustCrateRootPath(%q)=%v; want %v", test.path, got, test.want)
		}
	}
}
