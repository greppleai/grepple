package navigation

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/greppleai/grepple/internal/parser"
)

func TestNavigationLanguageIndexRegistryCoversSupportedFamilies(t *testing.T) {
	graph := parser.NavigationGraph{}
	corpus := &navigationCorpus{
		contents: map[string]string{},
		exports:  map[string][]navigationExport{},
		graph:    graph,
	}
	indexes, _ := newLanguageNavigationIndexes(corpus, nil)
	if !reflect.DeepEqual(sortedNavigationFamilies(indexes), sortedParserNavigationFamilies()) {
		t.Fatalf("resolver families=%v parser navigation families=%v", sortedNavigationFamilies(indexes), sortedParserNavigationFamilies())
	}
	for family, index := range indexes {
		if index == nil {
			t.Fatalf("language family %q has a nil navigation index", family)
		}
	}
}

func sortedNavigationFamilies(indexes map[string]languageNavigationIndex) []string {
	families := make([]string, 0, len(indexes))
	for family := range indexes {
		families = append(families, family)
	}
	sort.Strings(families)
	return families
}

func sortedParserNavigationFamilies() []string {
	seen := make(map[string]bool)
	for _, capability := range parser.NewParser().SupportedLanguages() {
		if capability.Navigation {
			seen[navigationLanguageFamily(capability.ID)] = true
		}
	}
	families := make([]string, 0, len(seen))
	for family := range seen {
		families = append(families, family)
	}
	sort.Strings(families)
	return families
}

// Both crate-module resolution and tsconfig aliases enter through the same
// languageImportResolver contract; language-specific policy stays in each index.
func TestLanguageImportResolverRoutesRustAndTypeScriptContext(t *testing.T) {
	root := t.TempDir()
	config := `{"compilerOptions":{"baseUrl":".","paths":{"@lib/*":["src/lib/*"]}}}`
	if err := os.WriteFile(filepath.Join(root, "tsconfig.json"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	rustFile := filepath.Join(root, "crate", "src", "lib.rs")
	rustModule := filepath.Join(root, "crate", "src", "helper.rs")
	tsFile := filepath.Join(root, "src", "main.ts")
	tsModule := filepath.Join(root, "src", "lib", "util.ts")
	graph := parser.BuildNavigationGraph("mod helper;\nuse self::helper::run;\n", "rust", rustFile)
	graph.Merge(parser.BuildNavigationGraph("pub fn run() {}\n", "rust", rustModule))
	corpus := &navigationCorpus{graph: graph, files: []string{rustFile, rustModule, tsFile, tsModule}}
	indexes, _ := newLanguageNavigationIndexes(corpus, corpus.files)
	for _, test := range []struct {
		family     string
		request    navigationImportRequest
		want       string
		wantScopes bool
	}{
		{"rust", navigationImportRequest{sourceFile: rustFile, importPath: "self::helper::run"}, rustModule, true},
		{"typescript", navigationImportRequest{sourceFile: tsFile, importPath: "@lib/util"}, tsModule, false},
	} {
		var resolver languageImportResolver = indexes[test.family]
		targets := resolver.importTargets(test.request)
		if !reflect.DeepEqual(targets.files, []string{test.want}) || (len(targets.scopes) > 0) != test.wantScopes {
			t.Errorf("%s import targets=%+v; want file %q and scopes=%v", test.family, targets, test.want, test.wantScopes)
		}
	}
}
