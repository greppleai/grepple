package api

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

type phaseRecorder struct{ started, ended []string }

func (r *phaseRecorder) Start(name string) func() {
	r.started = append(r.started, name)
	return func() { r.ended = append(r.ended, name) }
}
func TestObservedSnapshotsPreserveResultsAndReportPhases(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "sample.go")
	if err := os.WriteFile(source, []byte("package sample\nfunc Root(){Leaf()}\nfunc Leaf(){}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	artifact, _ := BuildNavigationArtifact([]string{source})
	query := "Root"
	plan := NewSearchPlan(SearchPlanOptions{Root: root, Query: query, Related: true, FollowRelated: 1})
	batch, err := SearchFiles(NewSearchPlan(SearchPlanOptions{Root: root, Query: query}), []string{source})
	if err != nil {
		t.Fatal(err)
	}
	for _, paged := range []bool{false, true} {
		t.Run(map[bool]string{false: "complete", true: "paged"}[paged], func(t *testing.T) {
			recorder := &phaseRecorder{}
			store := NewObservedNavigationSnapshotStore(recorder)
			path := filepath.Join(t.TempDir(), "snapshot.gnav")
			key := "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
			if paged {
				err = store.SavePaged(path, key, artifact, nil)
			} else {
				err = store.Save(path, key, artifact, nil)
			}
			if err != nil {
				t.Fatal(err)
			}
			var snapshot NavigationSnapshot
			if paged {
				snapshot, _, err = store.OpenPaged(path, key)
			} else {
				snapshot, _, err = store.Open(path, key)
			}
			if err != nil {
				t.Fatal(err)
			}
			defer snapshot.Close()
			stats := ReadNavigationSnapshotStats(snapshot)
			if stats.Paged != paged || stats.CorePagesRead != 0 || stats.CoreBytesRead != 0 {
				t.Fatal("open must not read paged numeric core")
			}
			plain, err := snapshot.Attach(plan, batch, root, root)
			if err != nil {
				t.Fatal(err)
			}
			observed, err := AttachNavigationSnapshotObserved(snapshot, plan, batch, root, root, recorder)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(plain.Results(0, 0, true), observed.Results(0, 0, true)) {
				t.Fatal("timing changed navigation output")
			}
			if !reflect.DeepEqual(recorder.started, recorder.ended) {
				t.Fatal("phase completion mismatch")
			}
			for _, name := range []string{"navigation.detail_directory", "navigation.select_matches", "navigation.graph_projection", "navigation.result_index", "navigation.context_preview"} {
				found := false
				for _, got := range recorder.started {
					found = found || got == name
				}
				if !found {
					t.Fatal("missing phase", name)
				}
			}
		})
	}
}
