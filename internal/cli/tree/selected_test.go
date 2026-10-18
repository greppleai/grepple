package tree

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/directorymeta"
	"github.com/greppleai/grepple/internal/filedigest"
	sourcedomain "github.com/greppleai/grepple/internal/sources"
	"github.com/greppleai/grepple/internal/wire"
)

func treeSelectedSource(t *testing.T, root, relative string, file directorymeta.FileV1) directorymeta.FileV1 {
	t.Helper()
	name := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(name), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(relative), 0600); err != nil {
		t.Fatal(err)
	}
	digest, err := filedigest.SHA256Hex(name)
	if err != nil {
		t.Fatal(err)
	}
	file.Name = filepath.Base(name)
	file.Checksum = digest
	file.Description = "Source " + relative
	return file
}

func TestLocalTreeSelectedFilesScopedBeforeDepthAndLabels(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	a := treeSelectedSource(t, root, "pkg/a.go", directorymeta.FileV1{Purpose: "Implementation", DomainAreas: []string{"Billing"}, Keywords: []string{"c++"}})
	b := treeSelectedSource(t, root, "pkg/deep/b.go", directorymeta.FileV1{Purpose: "Testing", DomainAreas: []string{"Payments"}, FunctionalAreas: []string{"API"}, Keywords: []string{"c++"}})
	c := treeSelectedSource(t, root, "pkg/deep/c.go", directorymeta.FileV1{Purpose: "Testing", DomainAreas: []string{"Billing"}})
	stale := treeSelectedSource(t, root, "pkg/deep/stale.go", directorymeta.FileV1{Purpose: "Testing", DomainAreas: []string{"Poison"}})
	stale.Checksum = strings.Repeat("0", 64)
	ignored := treeSelectedSource(t, root, "pkg/deep/ignored.go", directorymeta.FileV1{Purpose: "Testing", DomainAreas: []string{"Poison"}})
	if err := directorymeta.WriteDirectoryV1(root, filepath.Join(root, "pkg"), directorymeta.DirectoryV1{Files: []directorymeta.FileV1{a}}); err != nil {
		t.Fatal(err)
	}
	if err := directorymeta.WriteDirectoryV1(root, filepath.Join(root, "pkg/deep"), directorymeta.DirectoryV1{Files: []directorymeta.FileV1{b, c, stale, ignored}}); err != nil {
		t.Fatal(err)
	}
	repository := cliruntime.RepositoryServices{WorkingDirectoryFunc: func() string { return root }, ScopeOptionsFunc: func() (sourcedomain.Options, error) {
		return sourcedomain.Options{WorkingDirectory: root, IgnoreRoot: root, IgnorePaths: []string{"pkg/deep/ignored.go"}}, nil
	}}
	for _, test := range []struct {
		selection directorymeta.SelectorsV1
		want      string
	}{
		{directorymeta.SelectorsV1{Purposes: []string{"testing"}, DomainAreas: []string{"payments"}, Keywords: []string{"c++"}}, "pkg/deep/b.go"},
		{directorymeta.SelectorsV1{Purposes: []string{"Implementation", "Testing"}, DomainAreas: []string{"BILLING"}, Keywords: []string{"c++"}}, "pkg/a.go"},
		{directorymeta.SelectorsV1{Purposes: []string{"implementation"}, DomainAreas: []string{"Payments"}}, ""},
		{directorymeta.SelectorsV1{DomainAreas: []string{"Poison"}}, ""},
	} {
		selector, err := directorymeta.CompileSelectorsV1(test.selection)
		if err != nil {
			t.Fatal(err)
		}
		response, err := buildLocalSelected(root, 1, selector, repository)
		if err != nil {
			t.Fatal(err)
		}
		if test.want == "" {
			if len(response.Entries) != 0 || response.Labels != nil {
				t.Fatalf("no match leaked: %+v", response)
			}
			continue
		}
		if len(response.Entries) != 1 || response.Entries[0].Path != "pkg" || response.Entries[0].Labels == nil || response.Labels == nil {
			t.Fatalf("shallow match=%+v", response)
		}
		deep, err := buildLocalSelected(root, 3, selector, repository)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, entry := range deep.Entries {
			if entry.Path == test.want {
				found = entry.Labels != nil
			}
			if strings.Contains(entry.Path, "stale") || strings.Contains(entry.Path, "ignored") || entry.Path == "pkg/deep/c.go" && test.want != "pkg/deep/c.go" {
				t.Fatalf("unmatched entry %+v", entry)
			}
		}
		if !found {
			t.Fatalf("depth-three match=%+v", deep)
		}
		scoped, err := buildLocalSelected(filepath.Join(root, "pkg/deep"), 1, selector, repository)
		if err != nil {
			t.Fatal(err)
		}
		if test.want == "pkg/a.go" && len(scoped.Entries) > 0 || test.want == "pkg/deep/b.go" && (len(scoped.Entries) != 1 || scoped.Entries[0].Path != "b.go") {
			t.Fatalf("scoped=%+v", scoped)
		}
	}
	selector, err := directorymeta.CompileSelectorsV1(directorymeta.SelectorsV1{Purposes: []string{"Implementation", "Testing", "IMPLEMENTATION"}, DomainAreas: []string{"Billing"}})
	if err != nil {
		t.Fatal(err)
	}
	shallow, err := buildLocalSelected(root, 1, selector, repository)
	if err != nil || len(shallow.Entries) != 1 || shallow.Labels == nil || shallow.Labels.PurposeTotal != 2 || shallow.Labels.DomainAreaTotal != 1 {
		t.Fatalf("union of matching files=%+v err=%v", shallow, err)
	}
	expanded, err := buildLocalSelected(root, 2, selector, repository)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range expanded.Entries {
		if entry.Path == "pkg/deep" {
			found = entry.Labels != nil && entry.Labels.PurposeTotal == 1 && entry.Labels.Purposes[0] == "testing"
		}
	}
	if !found {
		t.Fatalf("matched-descendant union=%+v", expanded)
	}
}

func TestTreeFilteredJSONNoMatchGuidanceAndBoundedLabels(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	a := treeSelectedSource(t, root, "a.go", directorymeta.FileV1{Purpose: "Testing", Keywords: []string{"c++", "build", "syntax"}})
	if err := directorymeta.WriteDirectoryV1(root, root, directorymeta.DirectoryV1{Files: []directorymeta.FileV1{a}}); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	exit := 0
	context := cliruntime.Environment{Output: &stdout, ErrorOutput: &stderr, Exit: func(code int) { exit = code }}
	if err := Execute(context, &Request{Depth: 1, JSON: true, Keywords: selectorFlags{"c++"}}); err != nil {
		t.Fatal(err)
	}
	var data wire.TreeResponse
	if err := json.Unmarshal(stdout.Bytes(), &data); err != nil {
		t.Fatal(err)
	}
	if len(data.Entries) != 1 || data.Labels == nil || data.Labels.KeywordTotal != 3 || len(data.Labels.Keywords) != 2 || data.Entries[0].Labels == nil || data.Entries[0].Labels.KeywordTotal != 3 || exit != 0 {
		t.Fatalf("JSON=%+v exit=%d", data, exit)
	}
	stdout.Reset()
	if err := Execute(context, &Request{Depth: 1, JSON: true, Keywords: selectorFlags{"missing"}}); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(stdout.Bytes(), &data); err != nil {
		t.Fatal(err)
	}
	if len(data.Entries) != 0 || exit != 1 || !strings.Contains(stderr.String(), "grepple verify") {
		t.Fatalf("empty JSON=%+v exit=%d guidance=%q", data, exit, stderr.String())
	}
	stdout.Reset()
	if err := Execute(context, &Request{Depth: 1, Keywords: selectorFlags{"c++"}}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "keyword: build, c++ +1") {
		t.Fatalf("text summary=%q", stdout.String())
	}
	stdout.Reset()
	stderr.Reset()
	exit = 0
	if err := Execute(context, &Request{Depth: 1, Keywords: selectorFlags{"missing"}}); err != nil {
		t.Fatal(err)
	}
	if exit != 1 || !strings.Contains(stderr.String(), "grepple overview") || strings.Contains(stdout.String(), "a.go") {
		t.Fatalf("empty text=%q exit=%d guidance=%q", stdout.String(), exit, stderr.String())
	}
}
