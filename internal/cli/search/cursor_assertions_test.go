package search

import (
	"strings"
	"testing"
)

func assertCursorCommand(t *testing.T, output, server string) {
	t.Helper()
	if strings.Contains(output, "local.go") || !strings.Contains(output, "--cursor") || !strings.Contains(output, server) || !strings.Contains(output, "-F") {
		t.Fatalf("missing stable remote continuation: %s", output)
	}
}
func assertCursorReplayFiles(t *testing.T, output string) {
	t.Helper()
	if strings.Contains(output, "local.go") || strings.Count(output, `"path":"remote.go"`)+strings.Count(output, `"path": "remote.go"`) != 3 {
		t.Fatalf("cursor page was dropped/windowed or mixed: %s", output)
	}
}
