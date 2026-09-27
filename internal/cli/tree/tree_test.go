package tree

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/wire"
	"github.com/greppleai/grepple/internal/cliruntime"
	sourcedomain "github.com/greppleai/grepple/internal/sources"
)

func TestRunUsesLocalTreeByDefault(t *testing.T) {
	var stdout bytes.Buffer
	calledPath, calledDepth := "", 0
	application := cliruntime.Environment{Output: &stdout}
	command := &command{context: application, local: func(path string, depth int, kind sourcedomain.Kind, areas []string) (wire.TreeResponse, error) {
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
	cmd := &command{context: cliruntime.Environment{Output: &stdout}, local: func(path string, depth int, kind sourcedomain.Kind, areas []string) (wire.TreeResponse, error) {
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

func TestTreeAreaFlagsAreRepeatableAndLocalOnly(t *testing.T) {
	var stdout bytes.Buffer
	var selected []string
	cmd := &command{context: cliruntime.Environment{Output: &stdout}, local: func(path string, depth int, kind sourcedomain.Kind, areas []string) (wire.TreeResponse, error) {
		selected = append([]string(nil), areas...)
		return wire.TreeResponse{Repo: ".", Entries: []wire.TreeEntry{{Path: "pkg", Dir: true}}}, nil
	}}
	if err := cmd.Run([]string{"--area", "backend", "--area", "tests", "."}); err != nil || !reflect.DeepEqual(selected, []string{"backend", "tests"}) {
		t.Fatalf("repeatable areas=%v err=%v", selected, err)
	}
	for _, args := range [][]string{
		{"--area", "Bad", "."},
		{"--area", "", "."},
		{"--area", "two--words", "."},
		{"--area", "backend", "--repo", "owner/repo"},
		{"--area", "backend", "owner/repo", "src"},
	} {
		if err := cmd.Run(args); err == nil || !strings.Contains(err.Error(), "--area") {
			t.Fatalf("area args=%q err=%v", args, err)
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
