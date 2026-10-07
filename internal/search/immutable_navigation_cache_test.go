package search

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func testImmutableCache() *immutableNavigationCache {
	return &immutableNavigationCache{entries: map[string]*immutableNavigationEntry{}, maxEntries: 2, maxBytes: 1 << 20, ttl: time.Minute}
}
func TestImmutableNavigationCacheReuseIdentityAndBounds(t *testing.T) {
	cache := testImmutableCache()
	builds := 0
	build := func() (*navigationIndex, error) { builds++; return newNavigationIndex(), nil }
	a, _ := cache.resolve(context.Background(), "a", build)
	b, _ := cache.resolve(context.Background(), "a", build)
	if a != b || builds != 1 {
		t.Fatal("lookup rebuilt")
	}
	_, _ = cache.resolve(context.Background(), "b", build)
	_, _ = cache.resolve(context.Background(), "c", build)
	if len(cache.entries) != 2 || cache.entries["a"] != nil {
		t.Fatal("LRU bound not enforced")
	}
	oversized := testImmutableCache()
	oversized.maxBytes = 1
	_, _ = oversized.resolve(context.Background(), "large", build)
	if oversized.entries["large"] != nil || oversized.bytes > oversized.maxBytes {
		t.Fatal("oversized lookup retained")
	}
	_, _ = cache.resolve(context.Background(), "expired", build)
	cache.entries["expired"].expires = time.Now().Add(-time.Second)
	before := builds
	_, _ = cache.resolve(context.Background(), "expired", build)
	if builds != before+1 {
		t.Fatal("expired lookup reused")
	}
	paths := []string{"/root/a.go", "/root/b.go"}
	key := immutableNavigationKey("revision", paths)
	if key != immutableNavigationKey("revision", []string{paths[1], paths[0]}) || key == immutableNavigationKey("new-revision", paths) || key == immutableNavigationKey("revision", paths[:1]) {
		t.Fatal("incorrect lookup identity")
	}
}
func TestImmutableNavigationCacheCoalescesAndCancelsWaiters(t *testing.T) {
	cache := testImmutableCache()
	entered := make(chan struct{})
	release := make(chan struct{})
	var builds atomic.Int32
	build := func() (*navigationIndex, error) {
		builds.Add(1)
		close(entered)
		<-release
		return newNavigationIndex(), nil
	}
	done := make(chan *navigationIndex)
	go func() { index, _ := cache.resolve(context.Background(), "same", build); done <- index }()
	<-entered
	ctx, cancel := context.WithCancel(context.Background())
	canceled := make(chan error, 1)
	go func() { _, err := cache.resolve(ctx, "same", build); canceled <- err }()
	cancel()
	select {
	case err := <-canceled:
		if !errors.Is(err, context.Canceled) {
			t.Fatal("canceled waiter continued")
		}
	case <-time.After(time.Second):
		t.Fatal("waiter did not cancel")
	}
	var workers sync.WaitGroup
	results := make(chan *navigationIndex, 8)
	for i := 0; i < 8; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			index, err := cache.resolve(context.Background(), "same", build)
			if err != nil {
				t.Error(err)
			}
			results <- index
		}()
	}
	close(release)
	first := <-done
	workers.Wait()
	close(results)
	for index := range results {
		if index != first {
			t.Fatal("concurrent lookup differs")
		}
	}
	if builds.Load() != 1 {
		t.Fatal("concurrent builds were not coalesced")
	}
}
func TestNonNavigableMatchesDoNotAddNavigationRepositories(t *testing.T) {
	root := t.TempDir()
	code := filepath.Join(root, "owner", "code")
	docs := filepath.Join(root, "owner", "docs")
	for _, repo := range []string{code, docs} {
		mustCreateNavigationDirectory(t, filepath.Join(repo, ".git"))
	}
	path := filepath.Join(code, "match.go")
	readme := filepath.Join(docs, "README.md")
	mustWriteNavigationFile(t, path, "package code\nfunc Connect() { Helper() }\n")
	mustWriteNavigationFile(t, readme, "Connect documentation\n")
	foreign := filepath.Join(docs, "unmatched.go")
	mustWriteNavigationFile(t, foreign, "package docs\nfunc Helper() {}\n")
	selected := []FileMatch{{File: path, Language: "go"}, {File: readme, Language: "markdown"}}
	files, err := collectRelatedRepositoryFiles(context.Background(), Params{Root: root}, selected)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(files, []string{path}) {
		t.Fatalf("non-navigable match expanded discovery: %v", files)
	}
	matches, err := Files(Params{Root: root, Query: "Connect", Related: true, RelatedRepositoryContext: true, ImmutableNavigationRevision: "mixed"}, []string{readme, path})
	if err != nil || len(matches) != 2 {
		t.Fatalf("text result lost: %v %v", matches, err)
	}
	assertNoForeignNavigation(t, matches, foreign)
	files, err = collectRelatedRepositoryFiles(context.Background(), Params{Root: root}, selected[1:])
	if err != nil || len(files) != 0 {
		t.Fatal("text-only page built navigation")
	}
}
func TestImmutableRelatedLookupConcurrentParityAndMutableFreshness(t *testing.T) {
	root := t.TempDir()
	mustCreateNavigationDirectory(t, filepath.Join(root, ".git"))
	path := filepath.Join(root, "call.go")
	definition := filepath.Join(root, "definition.go")
	mustWriteNavigationFile(t, path, "package code\nfunc Connect(value Item) Item {return helper(value)}\n")
	mustWriteNavigationFile(t, definition, "package code\ntype Item struct {Name string}\nfunc helper(value Item) Item{return value}\n")
	params := Params{Root: root, Query: "Connect", Related: true, FollowRelated: 2, RelatedRepositoryContext: true}
	want, err := Files(params, []string{path})
	if err != nil {
		t.Fatal(err)
	}
	wantJSON, _ := json.Marshal(want)
	params.ImmutableNavigationRevision = "fixed-revision"
	var workers sync.WaitGroup
	for i := 0; i < 12; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			got, err := Files(params, []string{path})
			gotJSON, _ := json.Marshal(got)
			if err != nil || string(gotJSON) != string(wantJSON) {
				t.Errorf("warm lookup changed connections: %v", err)
			}
		}()
	}
	workers.Wait()
	// Mutable searches must never opt into the immutable cache.
	params.ImmutableNavigationRevision = ""
	mustWriteNavigationFile(t, definition, "package code\ntype Item struct {Name string}\n\nfunc helper(value Item) Item{return value}\n")
	got, err := Files(params, []string{path})
	gotJSON, _ := json.Marshal(got)
	if err != nil || string(gotJSON) == string(wantJSON) {
		t.Fatal("mutable lookup returned stale declaration range")
	}
	params.ImmutableNavigationRevision = "new-revision"
	fresh, err := Files(params, []string{path})
	freshJSON, _ := json.Marshal(fresh)
	if err != nil || string(freshJSON) != string(gotJSON) {
		t.Fatal("new revision reused previous graph")
	}
}
