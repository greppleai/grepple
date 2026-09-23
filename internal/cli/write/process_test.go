package write

import (
	"io"
	"os"
	"testing"

	"github.com/greppleai/grepple/internal/cliruntime"
)

// Test-only compatibility keeps legacy package tests focused on write behavior.
type Dependencies struct{}

func Run(args []string, _ Dependencies) error {
	return New(cliruntime.Environment{}).Run(args)
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

func withStdin(t *testing.T, content string, fn func()) {
	t.Helper()
	previous := os.Stdin
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.WriteString(content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	os.Stdin = reader
	defer func() { os.Stdin = previous; _ = reader.Close() }()
	fn()
}
