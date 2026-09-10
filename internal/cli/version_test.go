package cli

import (
	"strings"
	"testing"
)

func TestRunPrintsReproducibleVersionMetadata(t *testing.T) {
	oldVersion, oldCommit, oldBuildDate := Version, Commit, BuildDate
	Version, Commit, BuildDate = "v1.2.3", "0123456789abcdef", "2026-09-10T12:00:00Z"
	t.Cleanup(func() {
		Version, Commit, BuildDate = oldVersion, oldCommit, oldBuildDate
	})

	for _, argument := range []string{"--version", "version"} {
		output := captureStdout(t, func() {
			if err := Run([]string{argument}); err != nil {
				t.Fatal(err)
			}
		})
		for _, expected := range []string{"grepple v1.2.3", "commit 0123456789abcdef", "source-date 2026-09-10T12:00:00Z"} {
			if !strings.Contains(output, expected) {
				t.Fatalf("%s output %q does not contain %q", argument, output, expected)
			}
		}
	}
}
