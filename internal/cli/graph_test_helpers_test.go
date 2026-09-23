package cli

import (
	"os"
	"path/filepath"
	"testing"

	graphcommand "github.com/greppleai/grepple/internal/cli/graph"
	"github.com/greppleai/grepple/internal/cliruntime"
)

type navigationGraphOutput = graphcommand.Output

type navigationGraphTruncation = graphcommand.Truncation

func buildNavigationGraphOutputFromPaths(paths []string, maxFiles int) navigationGraphOutput {
	return graphcommand.BuildFromPaths(paths, maxFiles)
}
func graphContinuationCommand(mode string, paths []string, truncation *navigationGraphTruncation) string {
	return graphContinuationCommandWithOptions(cliruntime.RepositoryInvocationOptions{}, mode, paths, truncation)
}

func graphContinuationCommandWithOptions(options cliruntime.RepositoryInvocationOptions, mode string, paths []string, truncation *navigationGraphTruncation) string {
	return graphcommand.ContinuationCommand(newCommandContextWith(options, 0, nil), mode, paths, truncation)
}
func shortGraphID(id string) string { return graphcommand.ShortID(id) }

func writeGraphSource(t *testing.T, root, path, content string) string {
	t.Helper()
	fullPath := filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return fullPath
}
