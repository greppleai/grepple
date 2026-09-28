package parser

import (
	"errors"
	"slices"
	"sync"
	"testing"
	"time"
)

func TestDocumentReadViewIsStableAndCallbackScoped(t *testing.T) {
	document, err := NewParser().Parse("go", "package p\nfunc run() { call() }\n")
	if err != nil {
		t.Fatal(err)
	}
	defer document.Close()
	var retained DocumentView
	var retainedRoot ViewNode
	var kinds []string
	if err := document.Read(func(view DocumentView) error {
		retained = view
		retainedRoot = view.Root()
		if view.Language() != "go" || view.Source() == "" || !retainedRoot.Valid() {
			t.Fatalf("active view is invalid: %#v", view)
		}
		retainedRoot.WalkNamed(func(node ViewNode) { kinds = append(kinds, node.Kind()) })
		snapshot, ok := retainedRoot.Snapshot()
		if !ok || !snapshot.Valid() {
			t.Fatal("root snapshot failed")
		}
		cached := len(view.state.snapshots)
		if _, ok := retainedRoot.Snapshot(); !ok || len(view.state.snapshots) != cached {
			t.Fatal("repeated snapshot rebuilt cached nodes")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(kinds) < 4 || kinds[0] != "source_file" || kinds[1] != "package_clause" {
		t.Fatalf("walk order=%v", kinds)
	}
	if retained.Language() != "" || retained.Source() != "" || retainedRoot.Valid() || retainedRoot.Text() != "" {
		t.Fatal("view or node remained usable after callback")
	}
}

func TestDocumentWalkNamedViewOwnsReadScope(t *testing.T) {
	document, err := NewParser().Parse("go", "package p\nfunc run() { call() }\n")
	if err != nil {
		t.Fatal(err)
	}
	var nodes []string
	document.WalkNamed(func(node Node) { nodes = append(nodes, node.Kind()) })
	var views []string
	var retained ViewNode
	if err := document.WalkNamedView(func(node ViewNode) {
		views = append(views, node.Kind())
		retained = node
	}); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(nodes, views) || len(views) < 4 || retained.Valid() {
		t.Fatalf("node walk=%v view walk=%v retained=%v", nodes, views, retained.Valid())
	}
	document.Close()
	if err := document.WalkNamedView(func(ViewNode) {}); !errors.Is(err, ErrDocumentClosed) {
		t.Fatalf("closed document walk error=%v", err)
	}
	document.WalkNamed(func(Node) { t.Fatal("closed document yielded a node") })
}

func TestWalkNamedViewBoundedReportsConfiguredLimits(t *testing.T) {
	document, err := NewParser().Parse("go", "package p\nfunc run() { call() }\n")
	if err != nil {
		t.Fatal(err)
	}
	defer document.Close()
	if err := document.Read(func(view DocumentView) error {
		byNodes := walkNamedViewBounded(view.Root(), walkOptions{MaxNodes: 2}, func(ViewNode, int) bool { return true })
		if byNodes.Visited != 2 || !byNodes.Truncated {
			t.Fatalf("node-bounded walk=%#v", byNodes)
		}
		byDepth := walkNamedViewBounded(view.Root(), walkOptions{MaxDepth: 1}, func(ViewNode, int) bool { return true })
		if byDepth.Visited != 1 || !byDepth.Truncated {
			t.Fatalf("depth-bounded walk=%#v", byDepth)
		}
		pruned := walkNamedViewBounded(view.Root(), walkOptions{}, func(ViewNode, int) bool { return false })
		if pruned.Visited != 1 || pruned.Truncated {
			t.Fatalf("pruned walk=%#v", pruned)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestDocumentReadPropagatesErrorAndRejectsClosedDocument(t *testing.T) {
	document, err := NewParser().Parse("go", "package p\n")
	if err != nil {
		t.Fatal(err)
	}
	want := errors.New("stop")
	if got := document.Read(func(DocumentView) error { return want }); !errors.Is(got, want) {
		t.Fatalf("Read error=%v, want %v", got, want)
	}
	document.Close()
	if err := document.Read(func(DocumentView) error { return nil }); !errors.Is(err, ErrDocumentClosed) {
		t.Fatalf("closed Read error=%v", err)
	}
}

func TestDocumentCloseWaitsForReadView(t *testing.T) {
	document, err := NewParser().Parse("go", "package p\n")
	if err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	readDone := make(chan error, 1)
	go func() {
		readDone <- document.Read(func(view DocumentView) error {
			close(entered)
			<-release
			if !view.Root().Valid() {
				t.Error("view became invalid while callback was active")
			}
			return nil
		})
	}()
	<-entered
	var wait sync.WaitGroup
	wait.Add(1)
	closed := make(chan struct{})
	go func() {
		defer wait.Done()
		document.Close()
		close(closed)
	}()
	select {
	case <-closed:
		t.Fatal("Close returned while read callback was active")
	case <-time.After(10 * time.Millisecond):
	}
	close(release)
	if err := <-readDone; err != nil {
		t.Fatal(err)
	}
	wait.Wait()
	if document.Root().Valid() {
		t.Fatal("document remained open after Close")
	}
}
