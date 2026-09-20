package refs

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
	go func() { content, _ := io.ReadAll(reader); done <- string(content) }()
	fn()
	_ = writer.Close()
	os.Stdout = previous
	return <-done
}
