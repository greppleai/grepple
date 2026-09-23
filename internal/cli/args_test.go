package cli

import (
	"os"
	"strings"
	"testing"
)

func TestTopLevelHelpListsCommandFamilies(t *testing.T) {
	output := captureStdout(t, func() {
		if err := Run([]string{"--help"}); err != nil {
			t.Fatal(err)
		}
	})
	for _, expected := range []string{"search", "write", "grit", "graph", "anchors", "boundaries", "examples", "extract", "architecture", "sources", "artifacts", "languages", "rules", "get", "tree", "repos", "refs", "ask", "ai-provider", "login", "logout", "version", "--production-only", "--no-repo-config"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("top-level help missing %q:\n%s", expected, output)
		}
	}
	if count := strings.Count(output, "  refs         "); count != 1 {
		t.Fatalf("top-level help lists refs %d times:\n%s", count, output)
	}
}

func TestCommandAvailabilityRejectsWrongExecutionUniverse(t *testing.T) {
	for _, test := range []struct {
		args []string
		want string
	}{
		{args: []string{"extract", "--remote"}, want: "extract is local-only"},
		{args: []string{"write", "--repo", "owner/repo"}, want: "write is local-only"},
		{args: []string{"languages", "--server", "https://example.test"}, want: "languages is source-independent"},
		{args: []string{"repos", "--local"}, want: "repos uses the remote service"},
	} {
		err := Run(test.args)
		if err == nil || !strings.Contains(err.Error(), test.want) {
			t.Fatalf("Run(%q) error=%v want %q", test.args, err, test.want)
		}
	}
}

func TestHelpCommandAndExplicitSearch(t *testing.T) {
	help := captureStdout(t, func() {
		if err := Run([]string{"help"}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(help, "Commands:") {
		t.Fatalf("help command did not render top-level help:\n%s", help)
	}

	searchHelp := captureStdout(t, func() {
		if err := Run([]string{"help", "search"}); err != nil {
			t.Fatal(err)
		}
	})
	for _, expected := range []string{
		"Search the local working directory",
		"unlike grep -l, does not search contents",
		"--files-with-matches",
		"grep -l equivalent",
	} {
		if !strings.Contains(searchHelp, expected) {
			t.Fatalf("search help missing %q:\n%s", expected, searchHelp)
		}
	}
	assertSearchHelpOmitsRemovedAnchorFlags(t, searchHelp)

	directory := t.TempDir()
	path := directory + "/sample.txt"
	if err := os.WriteFile(path, []byte("graph\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	output := captureStdout(t, func() {
		if err := Run([]string{"search", "--line-only", "-F", "graph", path}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(output, "graph") || !strings.Contains(output, "sample.txt") {
		t.Fatalf("explicit search did not search a command-name pattern:\n%s", output)
	}

	if err := Run([]string{"help", "not-a-command"}); err == nil {
		t.Fatal("unknown help topic succeeded")
	}
}
func assertSearchHelpOmitsRemovedAnchorFlags(t *testing.T, help string) {
	t.Helper()
	if strings.Contains(help, "--anchors") || strings.Contains(help, "--anchor-provider") || strings.Contains(help, "--no-anchors") {
		t.Fatalf("search help still exposes a removed anchor flag:\n%s", help)
	}
}
