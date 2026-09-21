package extract

import (
	"io"
	"os"
	"testing"

	codeextract "github.com/greppleai/grepple/extract"
)

func runExtract(args []string) error {
	return Run(args, Dependencies{LoadSources: func(roots []string) ([]codeextract.Source, error) { return codeextract.LoadSources(roots) }})
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
