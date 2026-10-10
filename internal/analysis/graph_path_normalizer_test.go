package analysis

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGraphPathNormalizerMatchesExistingSemantics(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	normalize := graphPathNormalizer()
	for _, path := range []string{"", ".", "./foo.go", "folder/../foo.go", "../outside.go", filepath.Join(cwd, "foo.go"), filepath.Join(filepath.Dir(cwd), "outside.go")} {
		if actual, want := normalize(path), cleanPath(path); actual != want {
			t.Fatalf("%q: %q != %q", path, actual, want)
		}
	}
	directory := t.TempDir()
	if err = os.Chdir(directory); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(cwd)
	changed := graphPathNormalizer()
	if actual, want := changed(filepath.Join(directory, "foo.go")), cleanPath(filepath.Join(directory, "foo.go")); actual != want {
		t.Fatalf("cwd change: %q != %q", actual, want)
	}
}
