package overview

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/config"
	"github.com/greppleai/grepple/internal/directorymeta"
	"github.com/greppleai/grepple/internal/filedigest"
	sourcedomain "github.com/greppleai/grepple/internal/sources"
)

func source(t *testing.T, root, relative string, file directorymeta.FileV1) directorymeta.FileV1 {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("package demo\n"+relative+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hash, err := filedigest.SHA256Hex(path)
	if err != nil {
		t.Fatal(err)
	}
	file.Name, file.Checksum = filepath.Base(path), hash
	if file.Description == "" {
		file.Description = "Reviewed source."
	}
	if file.Purpose == "" {
		file.Purpose = "Implementation"
	}
	return file
}

func contextFor(t *testing.T, root string, ignored ...string) (cliruntime.Environment, *bytes.Buffer) {
	t.Helper()
	t.Chdir(root)
	var output bytes.Buffer
	application := cliruntime.Environment{
		Output: &output,
		RepositoryContext: cliruntime.RepositoryServices{
			WorkingDirectoryFunc: func() string { return root },
			ScopeOptionsFunc: func() (sourcedomain.Options, error) {
				return sourcedomain.Options{WorkingDirectory: root, IgnoreRoot: root, IgnorePaths: ignored}, nil
			},
		},
		Config: cliruntime.ConfigurationServices{LoadAskPreferences: func() (config.AskPreferences, error) {
			t.Fatal("overview read model preferences")
			return config.AskPreferences{}, nil
		}},
	}
	return application, &output
}

func TestOverviewSelectedCurrentLabelsAndDeterministicOutput(t *testing.T) {
	root := t.TempDir()
	a := source(t, root, "svc/a.go", directorymeta.FileV1{Purpose: "Implementation", DomainAreas: []string{"Payments"}, FunctionalAreas: []string{"HTTP"}, Keywords: []string{"c++"}})
	b := source(t, root, "svc/b.go", directorymeta.FileV1{Purpose: "Testing", DomainAreas: []string{"Payments"}, FunctionalAreas: []string{"HTTP"}, Keywords: []string{"c++"}})
	guide := source(t, root, "docs/guide.md", directorymeta.FileV1{Purpose: "Documentation", DomainAreas: []string{"Payments"}, Keywords: []string{"Manual"}})
	stale := source(t, root, "stale.go", directorymeta.FileV1{Purpose: "Stale poison", DomainAreas: []string{"Fake area"}, Keywords: []string{"broken"}})
	_ = source(t, root, "new.go", directorymeta.FileV1{})
	ignored := source(t, root, "hidden.go", directorymeta.FileV1{Purpose: "Hidden poison", DomainAreas: []string{"Fake area"}})
	gitignored := source(t, root, "gitignored.go", directorymeta.FileV1{Purpose: "Gitignored poison"})
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("gitignored.go\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := directorymeta.WriteDirectoryV1(root, filepath.Join(root, "svc"), directorymeta.DirectoryV1{Files: []directorymeta.FileV1{a, b}}); err != nil {
		t.Fatal(err)
	}
	if err := directorymeta.WriteDirectoryV1(root, filepath.Join(root, "docs"), directorymeta.DirectoryV1{Files: []directorymeta.FileV1{guide}}); err != nil {
		t.Fatal(err)
	}
	if err := directorymeta.WriteDirectoryV1(root, root, directorymeta.DirectoryV1{Files: []directorymeta.FileV1{stale, ignored, gitignored}}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "stale.go"), []byte("changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	application, output := contextFor(t, root, "hidden.go", ".gitignore")
	first, err := Build(application)
	if err != nil {
		t.Fatal(err)
	}
	if first.Files != (Coverage{Selected: 5, Current: 3, Stale: 1, Missing: 1}) {
		t.Fatalf("file coverage=%+v", first.Files)
	}
	if first.Directories != (Coverage{Selected: 3, Current: 2, Stale: 1}) {
		t.Fatalf("directory coverage=%+v", first.Directories)
	}
	if first.DomainAreas.Total != 1 || first.DomainAreas.Shown[0].Name != "payments" || first.DomainAreas.Shown[0].Files != 3 || first.DomainAreas.Shown[0].OmittedPaths != 1 || !reflect.DeepEqual(first.DomainAreas.Shown[0].Paths, []string{"docs/guide.md", "svc/a.go"}) {
		t.Fatalf("domain=%+v", first.DomainAreas)
	}
	if first.Purposes.Total != 3 || first.Keywords.Total != 2 || !strings.Contains(strings.Join(first.Examples, "\n"), "--domain-area 'payments'") {
		t.Fatalf("categories/examples=%+v", first)
	}
	second, err := Build(application)
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatalf("nondeterministic: %+v %+v err=%v", first, second, err)
	}
	text := first.Text()
	if strings.Contains(text, "poison") || strings.Contains(text, "Fake area") || strings.Contains(text, "broken") || strings.Contains(text, "hidden.go") || !strings.Contains(text, "grepple verify") {
		t.Fatalf("unsafe overview:\n%s", text)
	}
	if lines := strings.Count(text, "\n"); lines > 35 {
		t.Fatalf("overview too long (%d lines):\n%s", lines, text)
	}
	if err := Execute(application, &Args{JSON: true}); err != nil {
		t.Fatal(err)
	}
	var decoded Report
	if err := json.Unmarshal(output.Bytes(), &decoded); err != nil || !reflect.DeepEqual(decoded, first) {
		t.Fatalf("JSON parity: %v %+v", err, decoded)
	}
}

func TestOverviewAbsentInvalidAndEmptyCoverage(t *testing.T) {
	root := t.TempDir()
	application, _ := contextFor(t, root)
	empty, err := Build(application)
	if err != nil || empty.Files.Selected != 0 || len(empty.Guidance) == 0 || empty.Purposes.Total != 0 {
		t.Fatalf("empty=%+v err=%v", empty, err)
	}
	source(t, root, "main.go", directorymeta.FileV1{})
	missing, err := Build(application)
	if err != nil || missing.Files.Missing != 1 || missing.Files.Current != 0 || missing.Purposes.Total != 0 {
		t.Fatalf("missing=%+v err=%v", missing, err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".grepple"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(directorymeta.RepositoryPath(root), []byte("version: 1\ndirectories: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	invalid, err := Build(application)
	if err != nil || invalid.Files.Invalid != 1 || invalid.Files.Current != 0 || invalid.Purposes.Total != 0 || len(invalid.Guidance) == 0 {
		t.Fatalf("invalid=%+v err=%v", invalid, err)
	}
}

func TestOverviewCapsEveryCategoryAndExamples(t *testing.T) {
	root := t.TempDir()
	var files []directorymeta.FileV1
	for i := 0; i < 17; i++ {
		label := fmt.Sprintf("Label %02d", i)
		files = append(files, source(t, root, fmt.Sprintf("f%02d.go", i), directorymeta.FileV1{Purpose: label, DomainAreas: []string{label}, FunctionalAreas: []string{label}, Keywords: []string{label}}))
	}
	if err := directorymeta.WriteDirectoryV1(root, root, directorymeta.DirectoryV1{Files: files}); err != nil {
		t.Fatal(err)
	}
	application, _ := contextFor(t, root)
	report, err := Build(application)
	if err != nil {
		t.Fatal(err)
	}
	for _, group := range []Category{report.Purposes, report.DomainAreas, report.FunctionalAreas, report.Keywords} {
		if group.Total != 17 || len(group.Shown) != maxLabels || group.Truncated != 9 || group.Shown[0].Name != "label-00" {
			t.Fatalf("unbounded category=%+v", group)
		}
		for _, label := range group.Shown {
			if len(label.Paths) > maxExamples {
				t.Fatalf("unbounded examples=%+v", label)
			}
		}
	}
	if lines := strings.Count(report.Text(), "\n"); lines > 45 {
		t.Fatalf("unbounded text %d lines", lines)
	}
}

func TestOverviewExamplesQuoteShellMetacharacters(t *testing.T) {
	if got, want := quoteLabel("it's;$(whoami)"), "'it'\"'\"'s;$(whoami)'"; got != want {
		t.Fatalf("unsafe example argument %q, want %q", got, want)
	}
}
