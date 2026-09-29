package search

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/greppleai/grepple/internal/config"
)

func TestAnchorsEnabledBySettingsUseConfiguredProvider(t *testing.T) {
	directory := t.TempDir()
	t.Chdir(directory)
	if err := os.WriteFile("sample.go", []byte("package sample\r\n// needle\r\nfunc run() {}\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(directory, "settings.json")
	settings := config.UserSettings{Anchors: config.Anchors{EnabledByDefault: true, DefaultProvider: "test", Providers: map[string]config.Provider{"test": {Command: []string{os.Args[0], "-test.run=TestAnchorProviderProcess"}}}}}
	writeJSONFile(t, settingsPath, settings)
	t.Setenv("GREPPLE_SETTINGS", settingsPath)
	t.Setenv("GREPPLE_TEST_ANCHOR_PROVIDER", "1")
	output := captureStdout(t, func() {
		if err := Run([]string{"--line-only", "needle", "sample.go"}); err != nil {
			t.Fatal(err)
		}
	})
	if output != "sample.go\n\nA02│2│// needle\n" {
		t.Fatalf("anchored output = %q", output)
	}
}
