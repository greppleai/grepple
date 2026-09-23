package graph

import (
	"io"
	"os"
	"testing"

	"github.com/greppleai/grepple/internal/cliruntime"
)

type graphDiffOutput = DiffOutput
type graphResolveOutput = ResolveOutput

const navigationResolveSchema = NavigationResolveSchema

func Run(args []string) error {
	if len(args) > 0 && args[0] == "graph" {
		args = args[1:]
	}
	if len(args) >= 2 && args[0] == "help" && args[1] == "graph" {
		args = append(append([]string(nil), args[2:]...), "--help")
	}
	return New(cliruntime.Environment{}).Run(args)
}

func runGraph(args []string) error { return New(cliruntime.Environment{}).Run(args) }
func buildNavigationGraphOutputFromPaths(paths []string, maxFiles int) navigationGraphOutput {
	return BuildFromPaths(paths, maxFiles)
}

func captureStdout(t testing.TB, fn func()) string {
	t.Helper()
	previous := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	done := make(chan string, 1)
	go func() { data, _ := io.ReadAll(reader); done <- string(data) }()
	fn()
	_ = writer.Close()
	os.Stdout = previous
	return <-done
}

func chdirTemp(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(directory); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })
	return directory
}
