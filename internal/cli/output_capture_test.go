package cli

import (
	"io"
	"os"
	"testing"
)

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

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	previous := os.Stderr
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = writer
	done := make(chan string, 1)
	go func() { data, _ := io.ReadAll(reader); done <- string(data) }()
	fn()
	_ = writer.Close()
	os.Stderr = previous
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
