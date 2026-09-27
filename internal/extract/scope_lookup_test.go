package extract

import (
	"path/filepath"
	"testing"
)

func TestPrepareSourcePathsSharedDirectoriesAndMultipleProjects(t *testing.T) {
	root := t.TempDir()
	writeScopeFile(t, filepath.Join(root, "first", "go.mod"), "module example.com/first\n")
	writeScopeFile(t, filepath.Join(root, "second", "go.mod"), "module example.com/second\n")
	paths := []string{
		filepath.Join(root, "first", "pkg", "a.go"),
		filepath.Join(root, "first", "pkg", "b.go"),
		filepath.Join(root, "second", "pkg", "c.go"),
	}
	sources := make([]Source, 0, len(paths))
	for _, path := range paths {
		sources = append(sources, Source{Path: path})
	}
	analysis := newAnalysis()
	prepareSourcePaths(analysis, sources)
	for index, want := range []string{"first/pkg/a.go", "first/pkg/b.go", "second/pkg/c.go"} {
		if got := analysis.SourcePaths[paths[index]]; got != want {
			t.Errorf("source %s: got %q, want %q", paths[index], got, want)
		}
	}
	analysis = newAnalysis()
	prepareSourcePaths(analysis, sources[:2])
	for index, want := range []string{"pkg/a.go", "pkg/b.go"} {
		if got := analysis.SourcePaths[paths[index]]; got != want {
			t.Errorf("single-project source %s: got %q, want %q", paths[index], got, want)
		}
	}
}
