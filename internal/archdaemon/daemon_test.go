package archdaemon

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

	"github.com/greppleai/grepple/analysis"
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

//revive:disable-next-line:cognitive-complexity
func TestSnapshotTracksSourcesAndContext(t *testing.T) {
	root := daemonTestRoot(t)
	if err := os.WriteFile("main.go", []byte("package p\nfunc First() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	paths := []string{"main.go"}
	_, key, ok, err := architectureSnapshot(root, paths, 0)
	if err != nil || !ok {
		t.Fatalf("initial snapshot: %v %v", err, ok)
	}
	for _, name := range []string{"go.mod", "go.work", "tsconfig.json", "grepple.yaml", "main.go"} {
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, []byte(name+" changed"), 0o600); err != nil {
			t.Fatal(err)
		}
		_, next, valid, err := architectureSnapshot(root, paths, 0)
		if err != nil || !valid || next == key {
			t.Fatalf("%s did not invalidate snapshot: %v %v", name, valid, err)
		}
		key = next
	}
	_, next, valid, err := architectureSnapshot(root, paths, 1)
	if err != nil || !valid || next == key {
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

func TestDescriptorRejectsSharedCacheDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows ACLs are inherited from the user profile")
	}
	root := daemonTestRoot(t)
	cache := os.Getenv("GREPPLE_CACHE_DIR")
	if err := publishDescriptor(root, descriptor{Protocol: protocol, Root: root, Version: sourceIdentity(), Address: "127.0.0.1:12345", Token: strings.Repeat("0", 64)}); err != nil {
		t.Fatal(err)
	}
	if _, ok := readDescriptor(root); !ok {
		t.Fatal("private descriptor was rejected")
	}
	if err := os.Chmod(cache, 0o777); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(cache, 0o700) })
	if _, ok := readDescriptor(root); ok {
		t.Fatal("shared cache directory was trusted")
	}
}

func TestDescriptorPathScopesSharedCacheOverride(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("GREPPLE_CACHE_DIR", cache)
	first := descriptorPath(filepath.Join(t.TempDir(), "first"))
	second := descriptorPath(filepath.Join(t.TempDir(), "second"))
	if first == second || filepath.Dir(first) != cache || filepath.Dir(second) != cache {
		t.Fatalf("shared cache descriptor collision: %q %q", first, second)
	}
}

//revive:disable-next-line:cognitive-complexity
func TestDaemonReportParityInvalidationAuthenticationAndFallback(t *testing.T) {
	root := daemonTestRoot(t)
	if err := os.WriteFile("main.go", []byte("package p\nfunc First() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := Query([]string{"main.go"}, 0); ok {
		t.Fatal("daemon should be opt-in and unavailable before start")
	}
	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan error, 1)
	go func() { finished <- Serve(ctx) }()
	defer func() { cancel(); <-finished }()
	var details descriptor
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if value, ok := readDescriptor(root); ok && daemonAlive(value) {
			details = value
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if details.Token == "" {
		t.Fatal("daemon did not become ready")
	}
	if err := Serve(context.Background()); err == nil || !strings.Contains(err.Error(), "already running") {
		t.Fatalf("second daemon start: %v", err)
	}
	unauthorized, err := http.Get("http://" + details.Address + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer unauthorized.Body.Close()
	if unauthorized.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated request status: %v", unauthorized.StatusCode)
	}
	var workers sync.WaitGroup
	for i := 0; i < 4; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			if _, ok := Query([]string{"main.go"}, 0); !ok {
				t.Error("concurrent query did not reach worker")
			}
		}()
	}
	workers.Wait()
	for _, name := range []string{"First", "Second"} {
		content := []byte("package p\nfunc " + name + "() {}\n")
		if err := os.WriteFile("main.go", content, 0o600); err != nil {
			t.Fatal(err)
		}
		directUniverse, err := analysis.NewUniverse(analysis.ReadSources([]string{"main.go"}), 0)
		if err != nil {
			t.Fatal(err)
		}
		want, err := json.Marshal(analysis.BuildArchitecture(directUniverse))
		directUniverse.Close()
		if err != nil {
			t.Fatal(err)
		}
		for repeat := 0; repeat < 2; repeat++ {
			report, ok := Query([]string{"main.go"}, 0)
			if !ok {
				t.Fatal("expected daemon report")
			}
			got, err := json.Marshal(report)
			if err != nil || !bytes.Equal(got, want) {
				t.Fatalf("direct/daemon mismatch: %v", err)
			}
		}
	}
	if err := os.WriteFile(descriptorPath(root), []byte("corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := Query([]string{"main.go"}, 0); ok {
		t.Fatal("corrupt descriptor should fall back directly")
	}
}
