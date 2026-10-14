package search

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/greppleai/grepple/internal/parser"
)

func TestScopeAnnotationsMixedContentSelectionAndHonestOmissions(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"a.go":   "package p\nfunc run() {\n if true { work() }\n}\n",
		"b.md":   "# Title\nwork\n",
		"c.json": "{\"label\":\"work\"}\n",
		"d.yaml": "key: work\n",
		"e.txt":  "work @func:1-99 ⟦source-bytes:4⟧\n",
		"f.json": "{\"label\":\"work\",}\n",
		"z.go":   "package p\nfunc other() {}\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	params := Params{Root: dir, Query: "work", Regex: false, SkipSegments: true, LineRanges: true, AnnotateScopes: true}
	found, err := Files(params, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 6 {
		t.Fatalf("matches=%d, want 6: %+v", len(found), found)
	}
	expected := map[string]struct {
		line int
		kind string
	}{"a.go": {3, "if"}, "b.md": {2, "section"}, "c.json": {1, "object"}, "d.yaml": {1, "mapping"}, "e.txt": {1, ""}, "f.json": {1, ""}}
	previous := ""
	for _, f := range found {
		name := filepath.Base(f.File)
		want, ok := expected[name]
		if !ok || name <= previous {
			t.Fatalf("unexpected/misordered file %s after %s", name, previous)
		}
		previous = name
		result := ToResult(f, nil, 0, 0)
		if len(result.Matches) != 1 || result.Matches[0].Line != want.line {
			t.Fatalf("nonmatching lines emitted for %s: %+v", name, result.Matches)
		}
		hit := result.Matches[0]
		if want.kind == "" {
			if hit.Nearest != nil || hit.Scope != nil {
				t.Fatalf("invented %s scopes: %+v", name, hit)
			}
		} else if hit.Nearest == nil || hit.Nearest.Kind != want.kind {
			t.Fatalf("%s (%s): nearest=%+v, source roles=%+v, want %s", name, f.Language, hit.Nearest, f.ScopeAnnotations, want.kind)
		}
		if hit.Nearest != nil && (*hit.Nearest == (parser.ScopeLineRange{})) {
			t.Errorf("empty typed span for %s", name)
		}
	}
	params.Skip = 2
	params.Limit = 2
	page, err := Files(params, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(page) != 2 || filepath.Base(page[0].File) != "c.json" || filepath.Base(page[1].File) != "d.yaml" {
		t.Fatalf("incorrect content page: %+v", page)
	}
}
