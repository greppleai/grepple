package search

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRelatedNavigationDiscoversSiblingGoModulesFromRepositoryRoot(t *testing.T) {
	root := t.TempDir()
	mustCreateNavigationDirectory(t, filepath.Join(root, ".git"))
	appRoot := filepath.Join(root, "app")
	modelRoot := filepath.Join(root, "model")
	mustCreateNavigationDirectory(t, appRoot)
	mustCreateNavigationDirectory(t, filepath.Join(modelRoot, "types"))
	mustWriteNavigationFile(t, filepath.Join(appRoot, "go.mod"), "module example.com/app\n")
	mustWriteNavigationFile(t, filepath.Join(modelRoot, "go.mod"), "module example.com/model\n")
	mustWriteNavigationFile(t, filepath.Join(modelRoot, "types", "request.go"), "package types\ntype Request struct { Name string }\n")
	mustWriteNavigationFile(t, filepath.Join(appRoot, "run.go"), "package app\nimport \"example.com/model/types\"\nfunc Run(request types.Request) { _ = request /* SIBLING_MODULE_NEEDLE */ }\n")
	t.Chdir(appRoot)
	matches, err := Files(Params{Query: "SIBLING_MODULE_NEEDLE", Globs: []string{"run.go"}, Related: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("matches=%#v", matches)
	}
	point := findRelatedTypePoint(matches[0].Related, "Request")
	if point == nil || point.Confidence != "import-resolved" || !strings.HasSuffix(filepath.ToSlash(point.Path), "model/types/request.go") {
		t.Fatalf("sibling module type=%#v related=%#v", point, matches[0].Related)
	}
}

func findRelatedTypePoint(points []RelatedPoint, name string) *RelatedPoint {
	for index := range points {
		if points[index].Direction == "type" && points[index].Name == name {
			return &points[index]
		}
	}
	return nil
}

func TestRelatedNavigationFallsBackToWorkingDirectory(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "package", "nested")
	mustCreateNavigationDirectory(t, child)
	t.Chdir(root)
	roots, err := relatedNavigationRoots("", []FileMatch{{File: filepath.Join(child, "source.go")}})
	if err != nil {
		t.Fatal(err)
	}
	if len(roots) != 1 || roots[0] != root {
		t.Fatalf("navigation roots=%v want %s", roots, root)
	}
}

func mustCreateNavigationDirectory(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWriteNavigationFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
