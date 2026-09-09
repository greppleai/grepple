package cli

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

func TestRunSearchPipedStdinInvertMatch(t *testing.T) {
	chdirTemp(t)
	out := captureStdout(t, func() {
		withStdin(t, "keep\nskip\nalso keep\n", func() {
			if err := runSearch([]string{"--line-only", "-v", "skip"}); err != nil {
				t.Fatal(err)
			}
		})
	})
	want := "<stdin>:1:keep\n<stdin>:3:also keep\n"
	if out != want {
		t.Fatalf("expected %q, got %q", want, out)
	}
}

func TestRunSearchPipedStdinOnlyMatching(t *testing.T) {
	chdirTemp(t)
	out := captureStdout(t, func() {
		withStdin(t, "ids: abc-12 and def-34\nnone\n", func() {
			if err := runSearch([]string{"--only-matching", `[a-z]+-[0-9]+`}); err != nil {
				t.Fatal(err)
			}
		})
	})
	want := "<stdin>:1:abc-12\n<stdin>:1:def-34\n"
	if out != want {
		t.Fatalf("expected %q, got %q", want, out)
	}
}

func TestRunSearchPipedStdinOnlyMatchingFixedIgnoreCase(t *testing.T) {
	chdirTemp(t)
	out := captureStdout(t, func() {
		withStdin(t, "Hi HELLO hello\n", func() {
			if err := runSearch([]string{"-o", "-F", "-i", "hello"}); err != nil {
				t.Fatal(err)
			}
		})
	})
	want := "<stdin>:1:HELLO\n<stdin>:1:hello\n"
	if out != want {
		t.Fatalf("expected %q, got %q", want, out)
	}
}

func TestRunSearchPipedStdinAsymmetricContext(t *testing.T) {
	chdirTemp(t)
	content := "one\ntwo\nneedle\nfour\nfive\nsix\n"
	out := captureStdout(t, func() {
		withStdin(t, content, func() {
			if err := runSearch([]string{"-B", "2", "-A", "1", "needle"}); err != nil {
				t.Fatal(err)
			}
		})
	})
	want := "<stdin>-1-one\n<stdin>-2-two\n<stdin>:3:needle\n<stdin>-4-four\n"
	if out != want {
		t.Fatalf("expected %q, got %q", want, out)
	}
}

func TestContextSetsBothSidesAndAsymmetricFlagsOverride(t *testing.T) {
	options, _, _, err := parseSearchArgs([]string{"-C", "2", "-A", "4", "needle"})
	if err != nil {
		t.Fatal(err)
	}
	if options.Params.BeforeContext != 2 || options.Params.AfterContext != 4 {
		t.Fatalf("unexpected context: before=%d after=%d", options.Params.BeforeContext, options.Params.AfterContext)
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
	if out != "<stdin>\t2\n" {
		t.Fatalf("expected a per-file matching-line count, got %q", out)
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
