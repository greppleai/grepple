package ask

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/greppleai/grepple/internal/cliruntime"
	sourcedomain "github.com/greppleai/grepple/internal/sources"
)

func TestBackendSearchUsesCommandRepositoryPolicy(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "sample.go"), []byte("package sample\nfunc Parse() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	configured := false
	application := cliruntime.Environment{RepositoryContext: cliruntime.RepositoryServices{ScopeOptionsFunc: func() (sourcedomain.Options, error) {
		configured = true
		return sourcedomain.Options{WorkingDirectory: root, IgnoreRoot: root}, nil
	}}}
	result, err := runAskSearch(context.Background(), application, root, "", SearchInput{Query: "Parse", Mode: "snippets"})
	if err != nil {
		t.Fatal(err)
	}
	if !configured || result == nil {
		t.Fatalf("configured=%v result=%#v", configured, result)
	}
}
