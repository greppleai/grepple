package memory

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func fixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("package a\nfunc Run() {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return root
}
func TestCRUDAndStaleAttachment(t *testing.T) {
	root := fixture(t)
	_, digest, err := Source(root, "a.go")
	if err != nil {
		t.Fatal(err)
	}
	h := Header{Schema: "grepple-memory-v1", Name: "sample", Description: "A short note", DomainAreas: []string{"Source Parsing"}, Files: []Attachment{{Path: "a.go", Checksum: digest, Symbols: []string{"@func:2-2"}}}}
	if _, _, err := Recall(root, "sample"); err == nil {
		t.Fatal("missing directory should not read")
	}
	if _, err := os.Stat(filepath.Join(root, ".grepple")); !os.IsNotExist(err) {
		t.Fatalf("read created directory: %v", err)
	}
	if err := VerifyAttachment(root, h.Files[0]); err != nil {
		t.Fatal(err)
	}
	if err := Save(root, h, []byte("secret body\n"), false); err != nil {
		t.Fatal(err)
	}
	if err := Save(root, h, []byte("overwrite"), false); err == nil {
		t.Fatal("store overwrote")
	}
	data, got, err := Recall(root, "sample")
	if err != nil {
		t.Fatal(err)
	}
	if got.DomainAreas[0] != "source-parsing" || !bytes.Contains(data, []byte("secret body")) {
		t.Fatalf("recall: %q %v", data, got)
	}
	header, err := HeaderOnly(root, "sample")
	if err != nil || header.Description != "A short note" {
		t.Fatalf("header %v %v", header, err)
	}
	h.Description = "Another note"
	if err := Save(root, h, []byte("revised"), true); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("package a\nfunc Changed() {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if status := AttachmentStatuses(root, h); !strings.Contains(status[0], "stale checksum") {
		t.Fatalf("status %q", status)
	}
	if err := Delete(root, "sample"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Recall(root, "sample"); err == nil {
		t.Fatal("deleted memory was recalled")
	}
}
func TestValidationAndSymlinks(t *testing.T) {
	root := fixture(t)
	for _, name := range []string{"../escape", "A", "", "a.md", "with space"} {
		if ValidName(name) {
			t.Fatalf("accepted %q", name)
		}
	}
	for _, path := range []string{"../escape", "/etc/passwd", ".grepple/memories/x.md", "a\\b.go", "a:b.go"} {
		if ValidPath(path) {
			t.Fatalf("accepted path %q", path)
		}
	}
	h := Header{Schema: "grepple-memory-v1", Name: "one", Description: "Hint", Keywords: []string{"source"}}
	if _, err := Encode(h, []byte("  \n")); err == nil {
		t.Fatal("accepted empty body")
	}
	if _, err := Encode(h, []byte("body")); err != nil {
		t.Fatal(err)
	}
	for _, yaml := range []string{
		"schema: grepple-memory-v1\nname: one\nname: two\ndescription: Hint\nkeywords: [source]",
		"schema: grepple-memory-v1\nname: one\ndescription: Hint\nunknown: value\nkeywords: [source]",
		"schema: grepple-memory-v1\nname: one\ndescription: Hint\nkeywords: [&x source, *x]",
		"schema: grepple-memory-v2\nname: one\ndescription: Hint\nkeywords: [source]",
	} {
		if _, _, err := ParseHeader([]byte("---\n"+yaml+"\n---\nbody"), "one"); err == nil {
			t.Fatalf("accepted malformed %q", yaml)
		}
	}
	if err := os.Mkdir(filepath.Join(root, ".grepple"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(root, ".grepple", "memories")); err != nil {
		t.Fatal(err)
	}
	if err := Save(root, h, []byte("body"), false); err == nil {
		t.Fatal("followed memory directory symlink")
	}
}
func TestConcurrentStoresNoOverwrite(t *testing.T) {
	root := fixture(t)
	h := Header{Schema: "grepple-memory-v1", Name: "same", Description: "Short", Keywords: []string{"source"}}
	var wg sync.WaitGroup
	results := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); results <- Save(root, h, []byte("private"), false) }()
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("successful creates = %d", success)
	}
	data, _, err := Recall(root, "same")
	if err != nil || !bytes.HasSuffix(data, []byte("private")) {
		t.Fatalf("invalid final memory %q %v", data, err)
	}
}

func TestMalformedAndSymlinkedEntry(t *testing.T) {
	root := fixture(t)
	h := Header{Schema: "grepple-memory-v1", Name: "one", Description: "Brief hint", Keywords: []string{"source"}}
	if err := Save(root, h, []byte("private"), false); err != nil {
		t.Fatal(err)
	}
	name := filepath.Join(root, ".grepple", "memories", "one.md")
	if err := os.Remove(name); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "a.go"), name); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Recall(root, "one"); err == nil {
		t.Fatal("recalled symlink")
	}
	if err := Save(root, h, []byte("private"), true); err == nil {
		t.Fatal("updated symlink")
	}
	if err := os.Remove(name); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte("---\nschema: bad\n---\nprivate"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Recall(root, "one"); err == nil {
		t.Fatal("accepted malformed entry")
	}
	if err := Save(root, h, []byte("private"), true); err == nil {
		t.Fatal("overwrote malformed entry")
	}
}

func TestConcurrentUpdatesRemainComplete(t *testing.T) {
	root := fixture(t)
	h := Header{Schema: "grepple-memory-v1", Name: "one", Description: "Brief hint", Keywords: []string{"source"}}
	if err := Save(root, h, []byte("initial"), false); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errors := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); errors <- Save(root, h, []byte("replacement"), true) }()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatalf("cooperating update: %v", err)
		}
	}
	data, _, err := Recall(root, "one")
	if err != nil || !bytes.HasSuffix(data, []byte("replacement")) {
		t.Fatalf("partial update: %q %v", data, err)
	}
}
