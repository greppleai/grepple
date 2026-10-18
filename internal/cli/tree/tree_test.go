package tree

import (
	"bytes"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/directorymeta"
	"github.com/greppleai/grepple/internal/wire"
)

func TestRunUsesLocalTreeByDefault(t *testing.T) {
	var stdout bytes.Buffer
	calledPath, calledDepth := "", 0
	application := cliruntime.Environment{Output: &stdout}
	command := &command{context: application, local: func(path string, depth int, selector directorymeta.SelectorV1) (wire.TreeResponse, error) {
		calledPath, calledDepth = path, depth
		return wire.TreeResponse{Repo: ".", Path: path, Depth: depth, Entries: []wire.TreeEntry{{Path: "main.go"}}}, nil
	}}
	err := command.Run([]string{"src", "--depth", "3"})
	if err != nil {
		t.Fatal(err)
	}
	if calledPath != "src" || calledDepth != 3 {
		t.Fatalf("local request path=%q depth=%d", calledPath, calledDepth)
	}
	if stdout.String() != "./src\n└── main.go\n" {
		t.Fatalf("output=%q", stdout.String())
	}
}

func TestTreeDefaultsToOneLevelAndAllowsExplicitExpansion(t *testing.T) {
	var stdout bytes.Buffer
	calledDepth := 0
	cmd := &command{context: cliruntime.Environment{Output: &stdout}, local: func(path string, depth int, selector directorymeta.SelectorV1) (wire.TreeResponse, error) {
		calledDepth = depth
		return wire.TreeResponse{Repo: ".", Depth: depth, Entries: []wire.TreeEntry{{Path: "pkg", Dir: true}}}, nil
	}}
	if err := cmd.Run([]string{"."}); err != nil || calledDepth != 1 {
		t.Fatalf("default tree depth=%d err=%v", calledDepth, err)
	}
	if err := cmd.Run([]string{"--depth", "3", "."}); err != nil || calledDepth != 3 {
		t.Fatalf("expanded tree depth=%d err=%v", calledDepth, err)
	}
}

func TestTreeSelectorsRepeatAndRejectRemote(t *testing.T) {
	var stdout bytes.Buffer
	var selected directorymeta.SelectorV1
	cmd := &command{context: cliruntime.Environment{Output: &stdout}, local: func(path string, depth int, selector directorymeta.SelectorV1) (wire.TreeResponse, error) {
		selected = selector
		return wire.TreeResponse{Repo: ".", Entries: []wire.TreeEntry{{Path: "pkg", Dir: true}}}, nil
	}}
	if err := cmd.Run([]string{"--purpose", "Testing", "--purpose", "Implementation", "--domain-area", "API", "--functional-area", "Review", "--keyword", "c++", "."}); err != nil {
		t.Fatal(err)
	}
	if !selected.Requested() || !selected.Matches(directorymeta.FileStateV1{Status: directorymeta.StatusCurrent, File: directorymeta.FileV1{Purpose: "implementation", DomainAreas: []string{"api"}, FunctionalAreas: []string{"review"}, Keywords: []string{"c++"}}}) || selected.Matches(directorymeta.FileStateV1{Status: directorymeta.StatusCurrent, File: directorymeta.FileV1{Purpose: "Other", DomainAreas: []string{"api"}, FunctionalAreas: []string{"review"}, Keywords: []string{"c++"}}}) {
		t.Fatal("repeated categories lost or cross-category AND ignored")
	}
	if err := cmd.Run([]string{"--keyword=c++", "."}); err != nil {
		t.Fatal(err)
	}
	if !selected.Matches(directorymeta.FileStateV1{Status: directorymeta.StatusCurrent, File: directorymeta.FileV1{Keywords: []string{"c++"}}}) {
		t.Fatal("equals syntax lost keyword punctuation")
	}
	for _, flag := range []string{"--purpose", "--domain-area", "--functional-area", "--keyword"} {
		for _, args := range [][]string{{flag, "c++", "--repo", "owner/repo"}, {flag, "c++", "owner/repo", "src"}} {
			if err := cmd.Run(args); err == nil || !strings.Contains(err.Error(), flag) {
				t.Fatalf("args=%v err=%v", args, err)
			}
		}
	}
	for _, args := range [][]string{{"--purpose", "", "."}, {"--keyword", "\x00", "."}, {"--domain-area", strings.Repeat("a", 81), "."}} {
		if err := cmd.Run(args); err == nil {
			t.Fatalf("invalid args accepted: %q", args)
		}
	}
	for _, flag := range []string{"--area", "--kind"} {
		if err := cmd.Run([]string{flag, "legacy", "."}); err == nil {
			t.Fatalf("obsolete tree flag %q accepted", flag)
		}
	}
}

func TestNormalizeRequestSupportsExplicitAndLegacyRemoteRepositories(t *testing.T) {
	explicit := Request{Repository: "owner/repo", Targets: []string{"src"}}
	if remote, err := normalizeRequest(&explicit); err != nil || !remote || explicit.Repo != "owner/repo" || explicit.Path != "src" {
		t.Fatalf("explicit=%+v remote=%t err=%v", explicit, remote, err)
	}
	legacy := Request{Targets: []string{"owner/repo", "src"}}
	if remote, err := normalizeRequest(&legacy); err != nil || !remote || legacy.Repo != "owner/repo" || legacy.Path != "src" {
		t.Fatalf("legacy=%+v remote=%t err=%v", legacy, remote, err)
	}
}
