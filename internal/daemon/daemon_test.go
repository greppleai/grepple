package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/greppleai/grepple/internal/analysis"
)

func daemonTestRoot(t *testing.T) string {
	t.Helper()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })
	cache := filepath.Join(t.TempDir(), "cache")
	if err := os.MkdirAll(cache, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GREPPLE_CACHE_DIR", cache)
	t.Setenv("GREPPLE_NAVIGATION_CACHE_DIR", "")
	return root
}

func daemonTestReport(t *testing.T, paths []string) analysis.ArchitectureReport {
	t.Helper()
	universe, err := analysis.NewUniverse(analysis.ReadSources(paths), 0)
	if err != nil {
		t.Fatal(err)
	}
	defer universe.Close()
	return analysis.BuildArchitecture(universe)
}

func daemonTestStore(t *testing.T, paths []string) analysis.ArchitectureReport {
	t.Helper()
	sources := analysis.ReadSources(paths)
	key, ok := Key(paths, 0, sources)
	if !ok {
		t.Fatal("source fingerprint unavailable")
	}
	report := daemonTestReport(t, paths)
	if !Store(paths, 0, key, report) {
		t.Fatal("report was not accepted")
	}
	return report
}

//revive:disable-next-line:cognitive-complexity
func TestSnapshotTracksSourcesContextAndRoot(t *testing.T) {
	root := daemonTestRoot(t)
	if err := os.WriteFile("main.go", []byte("package p\nfunc First() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	paths := []string{"main.go"}
	sources, key, ok, err := architectureSnapshot(root, paths, 0)
	if err != nil || !ok {
		t.Fatalf("initial snapshot: %v %v", err, ok)
	}
	if clientKey, valid := architectureFingerprint(root, paths, 0, sources); !valid || clientKey != key {
		t.Fatal("client/server fingerprints differ")
	}
	if err := os.MkdirAll(filepath.Join(root, ".grepple"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"go.mod", "go.work", "tsconfig.json", filepath.Join(".grepple", "grepple.yaml"), filepath.Join(".grepple", "grepple.json"), "main.go"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(name+" changed"), 0o600); err != nil {
			t.Fatal(err)
		}
		_, next, valid, err := architectureSnapshot(root, paths, 0)
		if err != nil || !valid || next == key {
			t.Fatalf("%s did not invalidate snapshot: %v %v", name, valid, err)
		}
		key = next
	}
	if _, next, valid, err := architectureSnapshot(root, paths, 1); err != nil || !valid || next == key {
		t.Fatalf("max-files did not invalidate snapshot: %v %v", valid, err)
	}
	if _, _, _, err := architectureSnapshot(root, []string{"../outside.go"}, 0); err == nil {
		t.Fatal("outside source accepted")
	}
	if runtime.GOOS != "windows" {
		outside := filepath.Join(t.TempDir(), "outside.go")
		if err := os.WriteFile(outside, []byte("package p"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, "link.go"); err == nil {
			if _, _, _, err := architectureSnapshot(root, []string{"link.go"}, 0); err == nil {
				t.Fatal("symlink escape accepted")
			}
		}
	}
}

func TestGlobalDescriptorRequiresPrivateDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows ACLs are inherited from the user profile")
	}
	_ = daemonTestRoot(t)
	details := descriptor{Protocol: protocol, Version: sourceIdentity(), Address: "127.0.0.1:12345", Token: strings.Repeat("0", 64)}
	if err := publishDescriptor(details); err != nil {
		t.Fatal(err)
	}
	if _, ok := readDescriptor(); !ok {
		t.Fatal("private descriptor was rejected")
	}
	cache := os.Getenv("GREPPLE_CACHE_DIR")
	if err := os.Chmod(cache, 0o777); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(cache, 0o700) })
	if _, ok := readDescriptor(); ok {
		t.Fatal("shared cache directory was trusted")
	}
	t.Setenv("GREPPLE_CACHE_DIR", "relative/cache")
	if _, ok := readDescriptor(); ok {
		t.Fatal("relative shared cache path was trusted")
	}
}

//revive:disable-next-line:cognitive-complexity
func TestGlobalDaemonMultiRootParityInvalidationAndEviction(t *testing.T) {
	first := daemonTestRoot(t)
	if err := os.WriteFile("main.go", []byte("package p\nfunc First() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	paths := []string{"main.go"}
	if _, hit := Query(paths, 0); hit {
		t.Fatal("worker should be unavailable before start")
	}
	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan error, 1)
	go func() { finished <- Serve(ctx) }()
	defer func() {
		cancel()
		if err := <-finished; err != nil {
			t.Error(err)
		}
	}()
	var details descriptor
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if value, ok := readDescriptor(); ok && daemonAlive(value) {
			details = value
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if details.Token == "" {
		t.Fatal("worker did not become ready")
	}
	if err := Serve(context.Background()); err == nil || !strings.Contains(err.Error(), "already running") {
		t.Fatalf("duplicate worker start: %v", err)
	}
	unauthorized, err := http.Get("http://" + details.Address + "/health")
	if err != nil {
		t.Fatal(err)
	}
	_ = unauthorized.Body.Close()
	if unauthorized.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthorized request: %d", unauthorized.StatusCode)
	}
	if _, hit := Query(paths, 0); hit {
		t.Fatal("worker built report on cache miss")
	}
	want := daemonTestStore(t, paths)
	got, hit := Query(paths, 0)
	if !hit {
		t.Fatal("published report missing")
	}
	original, _ := json.Marshal(want)
	cached, _ := json.Marshal(got)
	if !bytes.Equal(original, cached) {
		t.Fatal("published report changed JSON")
	}
	var workers sync.WaitGroup
	for range 4 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			if _, ok := Query(paths, 0); !ok {
				t.Error("concurrent query missed")
			}
		}()
	}
	workers.Wait()
	if err := os.WriteFile("main.go", []byte("package p\nfunc Changed() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, hit := Query(paths, 0); hit {
		t.Fatal("source mutation reused report")
	}
	_ = daemonTestStore(t, paths)
	roots := []string{first}
	for i := 1; i <= maxCachedRoots; i++ {
		root := t.TempDir()
		roots = append(roots, root)
		if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package p\nfunc Root() {}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chdir(root); err != nil {
			t.Fatal(err)
		}
		if _, hit := Query(paths, 0); hit {
			t.Fatal("unseen root reused another report")
		}
		_ = daemonTestStore(t, paths)
		if _, hit := Query(paths, 0); !hit {
			t.Fatal("new root report missing")
		}
	}
	if err := os.Chdir(roots[0]); err != nil {
		t.Fatal(err)
	}
	if _, hit := Query(paths, 0); hit {
		t.Fatal("least-recently-used root was not evicted")
	}
	if err := os.Chdir(roots[len(roots)-1]); err != nil {
		t.Fatal(err)
	}
	if _, hit := Query(paths, 0); !hit {
		t.Fatal("most-recent root was evicted")
	}
	if err := os.WriteFile(descriptorPath(), []byte("corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, hit := Query(paths, 0); hit {
		t.Fatal("invalid descriptor did not fall back")
	}
}

func TestStoreRejectsStaleSourceAndBoundsMemory(t *testing.T) {
	root := daemonTestRoot(t)
	paths := []string{"main.go"}
	if err := os.WriteFile("main.go", []byte("package p\nfunc First() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, key, valid, err := architectureSnapshot(root, paths, 0)
	if err != nil || !valid {
		t.Fatalf("snapshot: %v %v", err, valid)
	}
	if err := os.WriteFile("main.go", []byte("package p\nfunc Second() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	worker := &service{cache: newReportCache()}
	report := daemonTestReport(t, paths)
	if err := worker.store(request{Root: root, Paths: paths, Key: key, Report: &report}); err == nil {
		t.Fatal("stale report was accepted")
	}
	if err := worker.cache.put(root, key, report, maxCachedReportBytes+1); err == nil {
		t.Fatal("oversized report was accepted")
	}
	if err := worker.cache.put(root, key, report, maxCachedReportBytes/2+1); err != nil {
		t.Fatal(err)
	}
	if err := worker.cache.put(root+"-other", key, report, maxCachedReportBytes/2+1); err != nil {
		t.Fatal(err)
	}
	if _, hit := worker.cache.get(root, key); hit || worker.cache.bytes > maxCachedReportBytes {
		t.Fatal("memory budget did not evict least-recently-used report")
	}
}
