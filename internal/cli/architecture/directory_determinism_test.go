package architecture

import (
	"bytes"
	"testing"
)

func TestDirectoryArchitectureColdWarmOutputIsByteIdentical(t *testing.T) {
	root := t.TempDir()
	writeArchitectureFixture(t, root, "go.mod", "module example.com/project\n")
	writeArchitectureFixture(t, root, "cmd/server/main.go", "package main\nfunc main() { run() }\nfunc run() {}\n")
	chdirForConfigTest(t, root)
	build := func() string {
		return captureStdout(t, func() {
			if err := runArchitecture([]string{"directory", "--json", "."}); err != nil {
				t.Fatal(err)
			}
		})
	}
	if before, after := []byte(build()), []byte(build()); !bytes.Equal(before, after) {
		t.Fatalf("cold/warm directory output differs:\nbefore=%s\nafter=%s", before, after)
	}
}
