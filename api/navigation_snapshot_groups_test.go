package api

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func navigationGroupsFixture(t *testing.T) (string, []string, NavigationSnapshot, []NavigationSnapshotGroup) {
	t.Helper()
	root := t.TempDir()
	var paths []string
	for name, content := range map[string]string{"sample.go": "package sample\ntype Widget struct{}\nfunc Root(w Widget){Leaf(w)}\nfunc Leaf(w Widget){}\nfunc Caller(){Root(Widget{})}\n", "sample.py": "def Root():\n    Leaf()\ndef Leaf():\n    pass\ndef Caller():\n    Root()\n"} {
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
	}
	artifact, _ := BuildNavigationArtifact(paths)
	parts, err := PartitionNavigationArtifact(artifact)
	if err != nil || len(parts) != 2 {
		t.Fatal("partitions", err)
	}
	store := NewNavigationSnapshotStore()
	key := strings.Repeat("a", 64)
	path := filepath.Join(t.TempDir(), "whole.gnav")
	if err := store.Save(path, key, artifact, nil); err != nil {
		t.Fatal(err)
	}
	whole, _, err := store.Open(path, key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { whole.Close() })
	var groups []NavigationSnapshotGroup
	for _, part := range parts {
		path := filepath.Join(t.TempDir(), "part.gnav")
		if err := store.SavePaged(path, key, part.Artifact, nil); err != nil {
			t.Fatal(err)
		}
		lease, _, err := store.OpenPaged(path, key)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { lease.Close() })
		groups = append(groups, NavigationSnapshotGroup{Languages: part.Languages, Snapshot: lease})
	}
	return root, paths, whole, groups
}
func TestNavigationGroupsMixedLanguageParity(t *testing.T) {
	root, paths, whole, groups := navigationGroupsFixture(t)

	for _, query := range []string{"Root", "Leaf", "Caller"} {
		for _, depth := range []int{0, 1, 2, 3} {
			assertNavigationGroupsParity(t, root, paths, whole, groups, query, depth)
		}
	}
	if _, err := AttachNavigationGroups(append(groups, groups[0]), NewSearchPlan(SearchPlanOptions{}), searchBatch{}, root, root, nil); err == nil {
		t.Fatal("overlapping groups accepted")
	}
}
func assertNavigationGroupsParity(t *testing.T, root string, paths []string, whole NavigationSnapshot, groups []NavigationSnapshotGroup, query string, depth int) {
	t.Helper()
	plan := NewSearchPlan(SearchPlanOptions{Root: root, Query: query, Related: true, FollowRelated: depth})
	batch, err := SearchFiles(NewSearchPlan(SearchPlanOptions{Root: root, Query: query}), paths)
	if err != nil {
		t.Fatal(err)
	}
	expected, err := whole.Attach(plan, batch, root, root)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := AttachNavigationGroups(groups, plan, batch, root, root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(expected.Results(0, 0, true), actual.Results(0, 0, true)) {
		t.Fatalf("mixed groups differ query=%s depth=%d", query, depth)
	}
	if _, err := AttachNavigationGroups(groups[:1], plan, batch, root, root, nil); err == nil {
		t.Fatal("missing selected group accepted")
	}
}
