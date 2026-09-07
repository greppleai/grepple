package search

import (
	"os"
	"testing"
)

// TestDisplayPathFromMatchesGetwd ensures the cwd-injected variant is identical
// to the os.Getwd()-based one for the current directory.
func TestDisplayPathFromMatchesGetwd(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{
		cwd + "/a/b/c.go",
		cwd,
		"/elsewhere/x.go",
	} {
		if got, want := displayPathFrom(file, cwd), displayPath(file); got != want {
			t.Errorf("displayPathFrom(%q) = %q, displayPath = %q", file, got, want)
		}
	}
}

func BenchmarkDisplayPathGetwd(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = displayPath("/srv/repos/acme/some-service/src/App.kt")
	}
}

func BenchmarkDisplayPathFrom(b *testing.B) {
	cwd, _ := os.Getwd()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = displayPathFrom("/srv/repos/acme/some-service/src/App.kt", cwd)
	}
}
