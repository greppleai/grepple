package sources

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/greppleai/grepple/internal/directorymeta"
	"github.com/greppleai/grepple/internal/filedigest"
)

func TestClassifyUsesFreshDirectoryMetadataOnly(t *testing.T) {
	root := t.TempDir()
	productionPath := filepath.Join(root, "service_test.go")
	testPath := filepath.Join(root, "service.go")
	for _, path := range []string{productionPath, testPath} {
		if err := os.WriteFile(path, []byte("package sample\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeKinds(t, root, root, map[string]Kind{"service_test.go": Production, "service.go": Test})
	classifier := NewClassifier(root)
	if got := classifier.Classify(productionPath); got != Production {
		t.Fatalf("metadata production kind=%q", got)
	}
	if got := classifier.Classify(testPath); got != Test {
		t.Fatalf("metadata test kind=%q", got)
	}
	if got := Classify(filepath.Join(root, "missing.go"), root); got != Unknown {
		t.Fatalf("missing metadata kind=%q", got)
	}
	if err := os.WriteFile(testPath, []byte("package changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := Classify(testPath, root); got != Unknown {
		t.Fatalf("stale metadata kind=%q", got)
	}
}

func TestClassifyMissingKindInCurrentMetadataIsUnknown(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "ordinary.go")
	if err := os.WriteFile(path, []byte("package ordinary\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	digest, err := filedigest.SHA256Hex(path)
	if err != nil {
		t.Fatal(err)
	}
	metadata := directorymeta.Metadata{Description: "Legacy metadata.", Responsibilities: []string{"Describe source."}, Files: []directorymeta.File{{Path: "ordinary.go", Description: "Ordinary source.", Checksum: digest}}}
	if err := directorymeta.Write(root, root, metadata); err != nil {
		t.Fatal(err)
	}
	if kind := Classify(path, root); kind != Unknown {
		t.Fatalf("unclassified file kind=%q, want unknown", kind)
	}
}
func TestProductionOnlyWalksUnknownParentDirectories(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "nested", "deep")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(nested, "service.go")
	if err := os.WriteFile(path, []byte("package deep\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeKinds(t, root, nested, map[string]Kind{"service.go": Production})
	files, err := Listing(nil, []string{root}, DiscoveryOptions{Root: root, IgnoreRoot: root, ProductionOnly: true})
	if err != nil || len(files) != 1 || files[0] != path {
		t.Fatalf("selected=%q err=%v; want %q", files, err, path)
	}
}

func TestParseKindDefaultsMissingToUnknownAndRejectsInvalid(t *testing.T) {
	if kind, valid := ParseKind(""); !valid || kind != Unknown {
		t.Fatalf("missing kind=%q valid=%v", kind, valid)
	}
	if kind, valid := ParseKind("application"); valid || kind != Unknown {
		t.Fatalf("invalid kind=%q valid=%v", kind, valid)
	}
}

func writeKinds(t *testing.T, root, directory string, kinds map[string]Kind) {
	t.Helper()
	files := make([]directorymeta.File, 0, len(kinds))
	for name, kind := range kinds {
		digest, err := filedigest.SHA256Hex(filepath.Join(directory, name))
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, directorymeta.File{Path: name, Description: "Test fixture.", Kind: string(kind), Checksum: digest})
	}
	if err := directorymeta.Write(root, directory, directorymeta.Metadata{Description: "Test sources.", Responsibilities: []string{"Support tests."}, Files: files}); err != nil {
		t.Fatal(err)
	}
}
