package tree

import (
	"bytes"
	"testing"

	"github.com/greppleai/grepple/api"
	"github.com/greppleai/grepple/internal/cliruntime"
	sourcedomain "github.com/greppleai/grepple/internal/sources"
)

func TestRunUsesLocalTreeByDefault(t *testing.T) {
	var stdout bytes.Buffer
	calledPath, calledDepth := "", 0
	application := cliruntime.Environment{Output: &stdout}
	command := &command{context: application, local: func(path string, depth int, kind sourcedomain.Kind) (api.TreeResponse, error) {
		calledPath, calledDepth = path, depth
		return api.TreeResponse{Repo: ".", Path: path, Depth: depth, Entries: []api.TreeEntry{{Path: "main.go"}}}, nil
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
