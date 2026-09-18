package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	directory, err := os.MkdirTemp("", "grepple-cli-tests-")
	if err != nil {
		panic(err)
	}
	if err := os.Setenv("GREPPLE_SETTINGS", filepath.Join(directory, "settings.json")); err != nil {
		panic(err)
	}
	if err := os.Setenv("GREPPLE_CONTEXT_GUARD_DIR", filepath.Join(directory, "context-guard")); err != nil {
		panic(err)
	}
	status := m.Run()
	_ = os.RemoveAll(directory)
	os.Exit(status)
}
