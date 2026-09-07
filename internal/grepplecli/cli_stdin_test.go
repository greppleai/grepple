package grepplecli

import (
	"os"
	"strings"
	"testing"
)

// withStdin replaces os.Stdin with a pipe carrying content (simulating
// `producer | grepple ...`) while fn runs.
func withStdin(t *testing.T, content string, fn func()) {
	t.Helper()
	previous := os.Stdin
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.WriteString(content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	os.Stdin = reader
	defer func() {
		os.Stdin = previous
		_ = reader.Close()
	}()
	fn()
}

func TestRunSearchPipedStdinLineOnly(t *testing.T) {
	chdirTemp(t)
	out := captureStdout(t, func() {
		withStdin(t, "alpha\nbeta hello\ngamma hello\n", func() {
			if err := runSearch([]string{"--line-only", "hello"}); err != nil {
				t.Fatal(err)
			}
		})
	})
	want := "<stdin>:2:beta hello\n<stdin>:3:gamma hello\n"
	if out != want {
		t.Fatalf("expected %q, got %q", want, out)
	}
}

func TestRunSearchPipedStdinDefaultSegments(t *testing.T) {
	chdirTemp(t)
	out := captureStdout(t, func() {
		withStdin(t, "one\nhello two\nthree\n", func() {
			if err := runSearch([]string{"hello"}); err != nil {
				t.Fatal(err)
			}
		})
	})
	if !strings.HasPrefix(out, "<stdin>\n") {
		t.Fatalf("expected the <stdin> result header, got %q", out)
	}
	if !strings.Contains(out, "hello two") {
		t.Fatalf("expected the matching line in the output, got %q", out)
	}
}

func TestRunSearchPipedStdinFilesWithMatches(t *testing.T) {
	chdirTemp(t)
	out := captureStdout(t, func() {
		withStdin(t, "x\nhello y\n", func() {
			if err := runSearch([]string{"--files-with-matches", "hello"}); err != nil {
				t.Fatal(err)
			}
		})
	})
	if out != "<stdin>\n" {
		t.Fatalf("expected %q, got %q", "<stdin>\n", out)
	}
}

func TestRunSearchPipedStdinCount(t *testing.T) {
	chdirTemp(t)
	out := captureStdout(t, func() {
		withStdin(t, "a hello\nb\nc hello\n", func() {
			if err := runSearch([]string{"--count", "hello"}); err != nil {
				t.Fatal(err)
			}
		})
	})
	if !strings.Contains(out, "<stdin>\t1 files\t2 matches") {
		t.Fatalf("expected a <stdin> count row, got %q", out)
	}
}

func TestRunSearchGlobIgnoresStdin(t *testing.T) {
	dir := chdirTemp(t)
	if err := os.WriteFile(dir+"/note.txt", []byte("hello from file\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := captureStdout(t, func() {
		withStdin(t, "hello from stdin\n", func() {
			if err := runSearch([]string{"--line-only", "hello", "*.txt"}); err != nil {
				t.Fatal(err)
			}
		})
	})
	if strings.Contains(out, "<stdin>") {
		t.Fatalf("expected stdin to be ignored when a glob is given, got %q", out)
	}
	if !strings.Contains(out, "note.txt:1:hello from file") {
		t.Fatalf("expected the filesystem match, got %q", out)
	}
}
