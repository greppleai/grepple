package api

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestNavigationSnapshotRelatedMatchesFreshSearch(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"go.mod":    "module example.com/sample\n\ngo 1.27\n",
		"root.go":   "package sample\nimport \"fmt\"\ntype Widget struct{}\nfunc Root(w Widget){ Middle(w); fmt.Println(w) }\n",
		"middle.go": "package sample\nfunc Middle(w Widget){Leaf(w)}\nfunc Leaf(w Widget){}\nfunc Caller(){Root(Widget{})}\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	paths := []string{filepath.Join(root, "root.go"), filepath.Join(root, "middle.go")}
	artifact, stats := BuildNavigationArtifact(paths)
	if stats.Failed != 0 {
		t.Fatal(stats)
	}
	path := filepath.Join(t.TempDir(), "query.gnav")
	key := strings.Repeat("a", 64)
	store := NewNavigationSnapshotStore()
	if err := store.Save(path, key, artifact, nil); err != nil {
		t.Fatal(err)
	}
	snapshot, _, err := store.Open(path, key)
	if err != nil {
		t.Fatal(err)
	}
	defer snapshot.Close()
	for _, depth := range []int{0, 1, 2, 3} {
		assertSnapshotSearchParity(t, snapshot, root, depth)
	}
}
func assertSnapshotSearchParity(t *testing.T, snapshot NavigationSnapshot, root string, depth int) {
	t.Helper()
	options := SearchPlanOptions{Root: root, Query: "Root", Related: true, RelatedRepositoryContext: true, FollowRelated: depth}
	plan := NewSearchPlan(options)
	reference, err := SearchFiles(plan, nil)
	if err != nil {
		t.Fatal(err)
	}
	scan := options
	scan.Related = false
	batch, err := SearchFiles(NewSearchPlan(scan), nil)
	if err != nil {
		t.Fatal(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	actual, err := snapshot.Attach(plan, batch, root, cwd)
	if err != nil {
		t.Fatal(err)
	}
	left, right := reference.Results(0, 0, true), actual.Results(0, 0, true)
	if !reflect.DeepEqual(left, right) {
		t.Fatalf("depth=%d persisted related output changed\nreference=%+v\nactual=%+v", depth, left, right)
	}
}
