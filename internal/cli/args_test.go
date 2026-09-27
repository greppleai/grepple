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
	for _, expected := range []string{"search", "write", "grit", "hook", "graph", "ask", "anchors", "examples", "architecture", "sources", "artifacts", "languages", "rules", "get", "tree", "repos", "refs", "ai-provider", "login", "logout", "version", "--production-only", "--no-repo-config"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("top-level help missing %q:\n%s", expected, output)
		}
	}
	for _, removed := range []string{"boundaries", "extract", "area", "start"} {
		if strings.Contains(output, "  "+removed+" ") {
			t.Fatalf("top-level help exposes removed command %q:\n%s", removed, output)
		}
	}
	if count := strings.Count(output, "  refs         "); count != 1 {
		t.Fatalf("top-level help lists refs %d times:\n%s", count, output)
	}
}

func TestAskCommandIsReachableWithoutContactingProvider(t *testing.T) {
	help := captureStdout(t, func() {
		if err := Run([]string{"ask", "--help"}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(help, "Usage: grepple ask") || !strings.Contains(help, "--timeout-seconds") {
		t.Fatalf("ask help was not dispatched:\n%s", help)
	}
	if err := Run([]string{"ask"}); err == nil || !strings.Contains(err.Error(), "ask requires a question") {
		t.Fatalf("empty ask argument error = %v", err)
	}
}

func TestCommandAvailabilityRejectsWrongExecutionUniverse(t *testing.T) {
	for _, test := range []struct {
		args []string
		want string
	}{
		{args: []string{"graph", "impact", "--symbol", "Run"}, want: "graph impact has been removed"},
		{args: []string{"help", "graph", "impact"}, want: "graph impact has been removed"},
		{args: []string{"graph", "dependencies", "--root-path", "pkg"}, want: "graph dependencies has been removed"},
		{args: []string{"graph", "dependents", "--at", "file.go:1"}, want: "graph dependents has been removed"},
		{args: []string{"area", "list", "--remote"}, want: "area has been removed"},
		{args: []string{"start", "--remote"}, want: "start has been removed"},
		{args: []string{"extract", "--remote"}, want: "extract has been removed"},
		{args: []string{"boundaries"}, want: "boundaries has been removed"},
		{args: []string{"graph", "build"}, want: "graph build has been removed"},
		{args: []string{"graph", "diff"}, want: "graph diff has been removed"},
		{args: []string{"architecture", "why", "a", "b"}, want: "architecture why has been removed"},
		{args: []string{"architecture", "resolve", "--symbol", "Run"}, want: "architecture resolve has been removed"},
		{args: []string{"architecture", "responsibilities"}, want: "architecture responsibilities has been removed"},
		{args: []string{"architecture", "compare"}, want: "architecture compare has been removed"},
		{args: []string{"write", "--repo", "owner/repo"}, want: "write is local-only"},
		{args: []string{"hook", "--repo", "owner/repo"}, want: "hook is local-only"},
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
