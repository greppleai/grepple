package navigation

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// Compare the indexed lookup against the previous corpus-order scan, including
// extensions, index modules, normalization, duplicates, and absolute paths.
func TestECMARelativeImportTargetsMatchCorpusScan(t *testing.T) {
	files := []string{
		"src/mod/index.ts", "src/mod.ts", "src/mod.tsx", "src/mod.js",
		"src/mod/index.js", "src/mod.d.ts", "src/nested/entry.ts",
		"src/nested/../mod.ts", "src/mod.ts", "src/index.ts",
		"/tmp/project/src/index.ts", "/tmp/project/src/main.ts",
	}
	index := newECMANavigationIndex(baseLanguageNavigationIndex{corpus: &navigationCorpus{files: files}})
	for _, test := range []struct{ source, imported string }{
		{"src/nested/entry.ts", "../mod"},
		{"src/nested/entry.ts", "../mod.ts"},
		{"src/nested/entry.ts", "../mod/index"},
		{"src/nested/entry.ts", "./entry.ts"},
		{"src/nested/entry.ts", "../missing"},
		{"src/nested/entry.ts", "../../src"},
		{"/tmp/project/src/main.ts", "./"},
	} {
		imported := filepath.Clean(filepath.Join(filepath.Dir(test.source), filepath.FromSlash(test.imported)))
		imported = strings.TrimSuffix(imported, filepath.Ext(imported))
		var want []string
		for _, file := range files {
			candidate := strings.TrimSuffix(filepath.Clean(file), filepath.Ext(file))
			if candidate == imported || filepath.Base(candidate) == "index" && filepath.Dir(candidate) == imported {
				want = append(want, file)
			}
		}
		got := index.importTargets(test.source, "", test.imported, "", "").files
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s importing %s: got %v want %v", test.source, test.imported, got, want)
		}
	}
}
