package cli

import "testing"

func isolateCLIAuth(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GREPPLE_TOKEN", "")
	t.Setenv("GREPPLE_SERVER", "")
}
