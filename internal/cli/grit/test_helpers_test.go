package grit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/greppleai/grepple/api"
)

func runGrit(args []string) error { return Run(args, testGritDependencies()) }
func testGritDependencies() Dependencies {
	return Dependencies{CurrentRepository: testCurrentRepository, ServerDefault: func(value string) string { return value }, RequestRemote: testRequestGritRemote, Metadata: func(values Arguments, response api.GritResponse, _ bool) *api.ResultMetadata {
		total := response.Total
		return &api.ResultMetadata{Page: api.ResultPage{Returned: len(response.Findings), Total: &total, Complete: true}, Limits: api.ResultLimits{JSONByteUncapped: values.JSON}}
	}}
}
func testCurrentRepository() string {
	output, err := exec.Command("git", "remote", "get-url", "origin").Output()
	if err != nil {
		return ""
	}
	value := strings.TrimSpace(string(output))
	value = strings.TrimSuffix(value, ".git")
	value = strings.TrimPrefix(value, "https://github.com/")
	value = strings.TrimPrefix(value, "git@github.com:")
	if strings.Count(value, "/") != 1 {
		return ""
	}
	return value
}
func testRequestGritRemote(ctx context.Context, request api.GritRequest, server string) (api.GritResponse, error) {
	body, err := json.Marshal(request)
	if err != nil {
		return api.GritResponse{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(server, "/")+"/public/grit", bytes.NewReader(body))
	if err != nil {
		return api.GritResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if token := os.Getenv("GREPPLE_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		return api.GritResponse{}, err
	}
	defer response.Body.Close()
	if response.StatusCode >= 300 {
		return api.GritResponse{}, fmt.Errorf("server returned %d", response.StatusCode)
	}
	var result api.GritResponse
	err = json.NewDecoder(response.Body).Decode(&result)
	return result, err
}
func withStdin(t *testing.T, content string, run func()) {
	t.Helper()
	file, err := os.CreateTemp(t.TempDir(), "stdin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(content); err != nil {
		t.Fatal(err)
	}
	if _, err := file.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	previous := os.Stdin
	os.Stdin = file
	run()
	os.Stdin = previous
	_ = file.Close()
}
func runGitForTest(t *testing.T, directory string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", directory}, args...)...)
	command.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com", "GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
}
func chdirTemp(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	previous, _ := os.Getwd()
	if err := os.Chdir(directory); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })
	return directory
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
