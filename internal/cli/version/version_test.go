package version

import (
	"strings"
	"testing"
)

func TestStringIncludesReproducibleMetadata(t *testing.T) {
	oldVersion, oldCommit, oldBuildDate := Version, Commit, BuildDate
	Version, Commit, BuildDate = "v1.2.3", "0123456789abcdef", "2026-09-10T12:00:00Z"
	t.Cleanup(func() { Version, Commit, BuildDate = oldVersion, oldCommit, oldBuildDate })
	output := String()
	for _, expected := range []string{"grepple v1.2.3", "commit 0123456789abcdef", "source-date 2026-09-10T12:00:00Z"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("output %q does not contain %q", output, expected)
		}
	}
}
