package architecture

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/greppleai/grepple/search"
)

func testArchitectureDependencies(productionOnly bool, exitCode *int) Dependencies {
	return Dependencies{
		ApplySourceConfig: func(params *search.Params) error {
			working, _ := os.Getwd()
			params.IgnoreRoot = working
			params.ProductionOnly = productionOnly
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
		RequestExit: func(code int) {
			if exitCode != nil {
				*exitCode = code
			}
		},
	}
}

func runArchitecture(args []string) error {
	return runArchitectureInternal(args, testArchitectureDependencies(false, nil))
}
func runArchitectureWithExit(args []string, productionOnly bool) (int, error) {
	code := 0
	err := runArchitectureInternal(args, testArchitectureDependencies(productionOnly, &code))
	return code, err
}
func runArchitectureInternal(args []string, dependencies Dependencies) error {
	if len(args) > 0 {
		switch args[0] {
		case "resolve":
			return runArchitectureResolve(args[1:], dependencies)
		case "why":
			return runArchitectureWhy(args[1:], dependencies)
		case "responsibilities":
			return runArchitectureResponsibilities(args[1:], dependencies)
		case "compare":
			return runArchitectureCompare(args[1:], dependencies)
		}
	}
	return newWithDependencies(dependencies).Run(args)
}
func buildDirectoryArchitecture(paths []string, maxFiles int) (Report, error) {
	return Build(paths, maxFiles, testArchitectureDependencies(false, nil))
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
