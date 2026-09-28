package architecture

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/greppleai/grepple/internal/search"
)

func testArchitectureDependencies() Dependencies {
	return Dependencies{
		ApplySourceConfig: func(params *search.Params) error {
			working, _ := os.Getwd()
			params.IgnoreRoot = working
			content, err := os.ReadFile(filepath.Join(working, "grepple.json"))
			if os.IsNotExist(err) {
				return nil
			}
			if err != nil {
				return err
			}
			var config struct {
				Ignore struct {
					Paths []string `json:"paths"`
				} `json:"ignore"`
			}
			if err := json.Unmarshal(content, &config); err != nil {
				return err
			}
			params.IgnorePaths = config.Ignore.Paths
			return nil
		},
	}
}

func runArchitecture(args []string) error {
	return newWithDependencies(testArchitectureDependencies()).Run(args)
}
func buildDirectoryArchitecture(paths []string, maxFiles int) (Report, error) {
	return Build(paths, maxFiles, testArchitectureDependencies())
}

func chdirForConfigTest(t testing.TB, directory string) {
	t.Helper()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(directory); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })
}

func captureStdout(t testing.TB, run func()) string {
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
