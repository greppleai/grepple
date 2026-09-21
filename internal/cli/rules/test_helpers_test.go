package rules

import (
	"io"
	"net/http"
	"os"
	"testing"
)

func runRules(args []string) error {
	return Run(args, Dependencies{ServerDefault: func(value string) string { return value }, NewRequest: func(method, target, contentType string, body io.Reader) (*http.Request, error) {
		request, err := http.NewRequest(method, target, body)
		if err == nil && contentType != "" {
			request.Header.Set("Content-Type", contentType)
		}
		return request, err
	}})
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
