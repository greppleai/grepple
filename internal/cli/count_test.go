package cli

import (
	"os"
	"testing"
)

func TestCountRendersMatchingLinesPerFile(t *testing.T) {
	dir := chdirTemp(t)
	if err := os.WriteFile(dir+"/a.txt", []byte("needle needle\nno\nneedle\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir+"/b.txt", []byte("needle\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() {
		if err := runSearch([]string{"--count", "needle", "a.txt", "b.txt"}); err != nil {
			t.Fatal(err)
		}
	})
	if want := "a.txt\t2\nb.txt\t1\n"; out != want {
		t.Fatalf("expected %q, got %q", want, out)
	}
}

func TestCountAndCountByRepoAreMutuallyExclusive(t *testing.T) {
	if _, _, _, err := parseSearchArgs([]string{"--count", "--count-by-repo", "needle"}); err == nil {
		t.Fatal("expected conflicting count modes to fail")
	}
}
