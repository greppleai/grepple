package navigation

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/parser"
)

func TestNavigationCorpusSharesPathLookupAcrossLanguages(t *testing.T) {
	files := []string{"src/mod.ts", "src/mod/index.js", "src/main.go", "src/sub/main.go", "src/header.h"}
	corpus := &navigationCorpus{files: files, contents: map[string]string{}}
	for _, file := range files {
		corpus.contents[file] = ""
	}
	indexes, _ := newLanguageNavigationIndexes(corpus, nil)
	js := indexes["javascript"].(*ecmaNavigationIndex)
	ts := indexes["typescript"].(*ecmaNavigationIndex)
	if js.paths != ts.paths || js.paths != corpus.pathIndex() {
		t.Fatal("JS, TS, and the corpus built separate path indexes")
	}
	goIndex := indexes["go"].(*goNavigationIndex)
	for _, directory := range []string{"src", "src/sub", "src/missing"} {
		var want []string
		for _, file := range files {
			if filepath.Clean(filepath.Dir(file)) == directory {
				want = append(want, file)
			}
		}
		got := goIndex.filesInDirectory(directory)
		if !reflect.DeepEqual(got, want) && !(len(got) == 0 && len(want) == 0) {
			t.Errorf("directory %q: got %v want %v", directory, got, want)
		}
	}
	cIndex := indexes["c"].(*cFamilyNavigationIndex)
	for _, test := range []struct{ path, kind, want string }{
		{"header.h", "include-quoted", "src/header.h"},
		{"header.h", "include-angle", ""},
		{"../etc/passwd", "include-quoted", ""},
	} {
		got := cIndex.importTargets(navigationImportRequest{sourceFile: "src/main.c", importPath: test.path, kind: test.kind}).files
		if test.want == "" && len(got) == 0 {
			continue
		}
		if !reflect.DeepEqual(got, []string{test.want}) {
			t.Errorf("C include %q %s: got %v want %s", test.path, test.kind, got, test.want)
		}
	}
}

func TestIndexedPythonImportTargetsMatchCorpusScan(t *testing.T) {
	root := t.TempDir()
	files := []string{
		filepath.Join(root, "pkg", "models.py"),
		filepath.Join(root, "two", "pkg", "models.pyw"),
		filepath.Join(root, "two", "pkg", "helpers", "__init__.py"),
		filepath.Join(root, "two", "app.py"),
		filepath.Join(root, "other", "pkg", "models.pyi"),
		filepath.Join(root, "README.md"),
	}
	sort.Strings(files)
	index := &pythonNavigationIndex{baseLanguageNavigationIndex: baseLanguageNavigationIndex{corpus: &navigationCorpus{files: files}}}
	for _, test := range []struct{ source, imported string }{
		{filepath.Join(root, "two", "app.py"), "pkg.models"},
		{filepath.Join(root, "two", "app.py"), "pkg.helpers"},
		{filepath.Join(root, "two", "app.py"), ".pkg.models"},
		{filepath.Join(root, "two", "app.py"), "..pkg.models"},
		{filepath.Join(root, "app.py"), "pkg.models"},
		{filepath.Join(root, "two", "app.py"), "not.present"},
		{filepath.Join(root, "two", "app.py"), "  pkg.models  "},
	} {
		got := index.importTargets(navigationImportRequest{sourceFile: test.source, importPath: test.imported}).files
		want := pythonImportTargetFiles(files, test.source, test.imported)
		if !reflect.DeepEqual(got, want) && !(len(got) == 0 && len(want) == 0) {
			t.Errorf("Python %s importing %q: got %v want %v", test.source, test.imported, got, want)
		}
	}
}

func TestIndexedTypeScriptAliasTargetsMatchCorpusScan(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "tsconfig.json"), []byte(`{"compilerOptions":{"baseUrl":"src","paths":{"@lib/*":["lib/*"],"@fallback/*":["missing/*","lib/*"]}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	files := []string{
		filepath.Join(root, "src", "lib", "thing.ts"),
		filepath.Join(root, "src", "lib", "thing", "index.js"),
		filepath.Join(root, "src", "lib", "thing.tsx"),
		filepath.Join(root, "src", "other.ts"),
	}
	sort.Strings(files)
	paths := (&navigationCorpus{files: files}).pathIndex()
	source := filepath.Join(root, "src", "main.ts")
	for _, imported := range []string{"@lib/thing", "@fallback/thing", "lib/thing", "@lib/missing"} {
		got := typeScriptAliasImportTargets(paths, source, imported)
		want := scanTypeScriptAliasTargets(files, source, imported)
		if !reflect.DeepEqual(got, want) && !(len(got) == 0 && len(want) == 0) {
			t.Errorf("TS alias %q: got %v want %v", imported, got, want)
		}
	}
}

// Keep the previous full-file scan as a test oracle for config aliases.
func scanTypeScriptAliasTargets(files []string, sourceFile, importPath string) []string {
	configPath := nearestTypeScriptConfig(sourceFile)
	config, ok := readTypeScriptConfig(configPath)
	if !ok {
		return nil
	}
	base := filepath.Dir(configPath)
	if config.CompilerOptions.BaseURL != "" {
		base = filepath.Join(base, filepath.FromSlash(config.CompilerOptions.BaseURL))
	}
	patterns := make([]string, 0, len(config.CompilerOptions.Paths))
	for pattern := range config.CompilerOptions.Paths {
		patterns = append(patterns, pattern)
	}
	sort.Strings(patterns)
	var targets []string
	for _, pattern := range patterns {
		wildcard, matches := typeScriptPathPatternMatch(pattern, importPath)
		if !matches {
			continue
		}
		for _, replacement := range config.CompilerOptions.Paths[pattern] {
			target := strings.Replace(filepath.FromSlash(replacement), "*", filepath.FromSlash(wildcard), 1)
			targets = append(targets, scanTypeScriptModuleFiles(files, filepath.Join(base, target))...)
		}
	}
	if len(targets) == 0 && config.CompilerOptions.BaseURL != "" {
		targets = scanTypeScriptModuleFiles(files, filepath.Join(base, filepath.FromSlash(importPath)))
	}
	sort.Strings(targets)
	return compactSortedStrings(targets)
}

func scanTypeScriptModuleFiles(files []string, target string) []string {
	target = strings.TrimSuffix(filepath.Clean(target), filepath.Ext(target))
	var matches []string
	for _, file := range files {
		module := strings.TrimSuffix(filepath.Clean(file), filepath.Ext(file))
		if module == target || filepath.Base(module) == "index" && filepath.Dir(module) == target {
			matches = append(matches, file)
		}
	}
	return matches
}

func TestIndexedExportTargetsMatchScan(t *testing.T) {
	corpus := &navigationCorpus{graph: parser.NavigationGraph{Exports: []parser.NavigationExport{
		{Language: "java", ImportPath: "pkg", Name: "One", Path: "one.java"},
		{Language: "kotlin", ImportPath: "pkg", Name: "One", Path: "one.kt"},
		{Language: "csharp", ImportPath: "Ns", Name: "Type", Path: "a.cs"},
		{Language: "csharp", ImportPath: "Ns.Type", Name: "Inner", Path: "b.cs"},
		{Language: "csharp", ImportPath: "Ns", Name: "Type", Path: "a.cs"},
		{Language: "java", ImportPath: "pkg", Name: "Two", Path: "two.java"},
	}}}
	for _, test := range []struct{ family, path, imported string }{
		{"java", "pkg", "*"}, {"java", "pkg.One", ""}, {"kotlin", "pkg.One", ""},
		{"csharp", "Ns", "*"}, {"csharp", "Ns.Type", ""}, {"csharp", "Ns.Type", "Type"},
		{"csharp", "missing", "*"},
	} {
		base := baseLanguageNavigationIndex{family: test.family, corpus: corpus}
		var got []string
		if test.family == "csharp" {
			got = (&cSharpNavigationIndex{baseLanguageNavigationIndex: base}).importTargets(navigationImportRequest{importPath: test.path, imported: test.imported}).files
		} else {
			got = (&qualifiedExportNavigationIndex{baseLanguageNavigationIndex: base}).importTargets(navigationImportRequest{importPath: test.path, imported: test.imported}).files
		}
		var want []string
		for _, item := range corpus.graph.Exports {
			if navigationLanguageFamily(item.Language) != test.family {
				continue
			}
			if test.family == "csharp" {
				if cSharpExportMatches(item.ImportPath, item.Name, test.path, test.imported) {
					want = append(want, item.Path)
				}
			} else if test.imported == "*" && item.ImportPath == test.path || test.imported != "*" && navigationQualifiedExportName(item.ImportPath, item.Name) == test.path {
				want = append(want, item.Path)
			}
		}
		want = compactSortedStrings(want)
		if !reflect.DeepEqual(got, want) && !(len(got) == 0 && len(want) == 0) {
			t.Errorf("%s %s imported=%q: got %v want %v", test.family, test.path, test.imported, got, want)
		}
	}
}

func TestGoNavigationIndexCachesModuleIdentityPerSource(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, config := range []struct{ file, content string }{
		{filepath.Join(root, "go.mod"), "module example.org/root\n"},
		{filepath.Join(nested, "go.mod"), "module example.org/nested\n"},
	} {
		if err := os.WriteFile(config.file, []byte(config.content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	index := &goNavigationIndex{}
	for _, test := range []struct{ source, imported string }{
		{filepath.Join(root, "a.go"), "example.org/root"},
		{filepath.Join(root, "a.go"), "example.org/root/pkg"},
		{filepath.Join(root, "a.go"), "example.org/nested"},
		{filepath.Join(nested, "b.go"), "example.org/nested/pkg"},
		{filepath.Join(nested, "b.go"), "example.org/root/pkg"},
		{filepath.Join(t.TempDir(), "missing.go"), "example.org/root"},
	} {
		for range 2 {
			got, known := index.localImportDirectory(test.source, test.imported)
			want, wantKnown := localGoImportDirectory(test.source, test.imported)
			if got != want || known != wantKnown {
				t.Errorf("Go %s importing %q: got (%q, %v) want (%q, %v)", test.source, test.imported, got, known, want, wantKnown)
			}
		}
	}
	if len(index.modules) != 3 {
		t.Fatalf("cached %d modules for three distinct source files", len(index.modules))
	}
}
