package extract

import (
	"io"
	"os"
	"testing"

	"github.com/greppleai/grepple/internal/cliruntime"
)

func runExtract(args []string) error {
	return New(cliruntime.Environment{Output: os.Stdout, RepositoryContext: cliruntime.RepositoryServices{}}).Run(args)
}

func captureStdout(t *testing.T, run func()) string {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	previous := os.Stdout
	os.Stdout = writer
	run()
	_ = writer.Close()
	os.Stdout = previous
	content, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}
