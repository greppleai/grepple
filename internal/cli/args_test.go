package cli

import (
	"os"
	"strings"
	"testing"
)

func TestParseSearchArgs(t *testing.T) {
	options, server, remote, err := parseSearchArgs([]string{
		"--line-only",
		"--enclosing",
		"--ignore-case",
		"--invert-match",
		"--max-files", "5",
		"--sort", "matches",
		"--server", "http://search.example",
		"needle",
		"src/*/*.go",
	})
	if err != nil {
		t.Fatal(err)
	}
	if options == nil {
		t.Fatal("expected options")
	}
	// An explicit --server opts into remote.
	if server != "http://search.example" || !remote {
		t.Fatalf("unexpected target options: server=%q remote=%v", server, remote)
	}
	if !options.LineOnly || !options.Params.LineRanges || !options.Params.EnclosingRanges || !options.Params.IgnoreCase || !options.Params.InvertMatch || options.Params.MaxFiles != 5 || options.Params.Sort != "matches" {
		t.Fatalf("unexpected options: %#v", options)
	}
	if options.Params.Query != "needle" || len(options.Params.Globs) != 1 {
		t.Fatalf("unexpected search parameters: %#v", options.Params)
	}
	request := searchRequestFromParams(options.Params)
	if request.InvertMatch == nil || !*request.InvertMatch {
		t.Fatalf("invert-match was not preserved in the remote request: %#v", request)
	}
	if !request.LineRanges || !request.EnclosingRanges || request.Sort != "matches" {
		t.Fatalf("line-only metadata or result sort was not preserved remotely: %#v", request)
	}
}

func TestEnclosingRequiresLineOnlyAndRejectsExplicitAnchors(t *testing.T) {
	if err := validateEnclosingArgs(&searchArgs{Enclosing: true}); err == nil {
		t.Fatal("expected --enclosing without --line-only to fail")
	}
	if err := validateEnclosingArgs(&searchArgs{LineOnly: true, Enclosing: true, Anchors: true}); err == nil {
		t.Fatal("expected --enclosing with anchors to fail")
	}
}

func TestParseSearchArgsAcceptsSafeGrepCompatibilityAliases(t *testing.T) {
	options, _, _, err := parseSearchArgs([]string{"-r", "-E", "needle", "src"})
	if err != nil {
		t.Fatal(err)
	}
	if options.Params.Query != "needle" || !options.Params.Regex {
		t.Fatalf("compatibility aliases produced unexpected options: %#v", options.Params)
	}
	if options.MaxOutputBytes != DefaultTextOutputBytes {
		t.Fatalf("default output cap = %d, want %d", options.MaxOutputBytes, DefaultTextOutputBytes)
	}

	unbounded, _, _, err := parseSearchArgs([]string{"--max-output-bytes", "0", "needle"})
	if err != nil {
		t.Fatal(err)
	}
	if unbounded.MaxOutputBytes != 0 {
		t.Fatalf("--max-output-bytes 0 = %d, want unbounded", unbounded.MaxOutputBytes)
	}
	if _, _, _, err := parseSearchArgs([]string{"--sort", "score", "needle"}); err == nil || !strings.Contains(err.Error(), "path") {
		t.Fatalf("invalid sort error = %v", err)
	}
}

func TestParseSearchArgsEnablesRelatedGoNavigation(t *testing.T) {
	options, _, remote, err := parseSearchArgs([]string{"--related", "needle", "**/*.go"})
	if err != nil {
		t.Fatal(err)
	}
	if remote || options == nil || !options.Params.Related {
		t.Fatalf("related navigation was not enabled: remote=%v options=%#v", remote, options)
	}
	request := searchRequestFromParams(options.Params)
	if !request.Related {
		t.Fatalf("related navigation was not preserved in request: %#v", request)
	}
}

func TestParseSearchArgsEnablesRemoteNavigation(t *testing.T) {
	options, _, remote, err := parseSearchArgs([]string{"--server", "http://search.example", "--follow-related", "1", "needle"})
	if err != nil {
		t.Fatal(err)
	}
	request := searchRequestFromParams(options.Params)
	if !remote || !request.Related || request.FollowRelated != 1 {
		t.Fatalf("remote navigation was not preserved: remote=%v request=%#v", remote, request)
	}
}

func TestAnchorProviderFlagEnablesAnchoredOutput(t *testing.T) {
	options, _, remote, err := parseSearchArgs([]string{"--anchor-provider", "pi", "--line-only", "needle"})
	if err != nil {
		t.Fatal(err)
	}
	if remote || !options.Anchors || options.AnchorProvider != "pi" {
		t.Fatalf("anchor provider was not enabled: remote=%v options=%#v", remote, options)
	}
}

func TestSettingsEnableAnchorsByDefault(t *testing.T) {
	settingsPath := t.TempDir() + "/settings.json"
	writeJSONFile(t, settingsPath, userSettings{Anchors: anchorSettings{EnabledByDefault: true, DefaultProvider: "pi"}})
	t.Setenv("GREPPLE_SETTINGS", settingsPath)

	options, _, _, err := parseSearchArgs([]string{"--line-only", "needle", "sample.go"})
	if err != nil {
		t.Fatal(err)
	}
	if !options.Anchors || !options.AnchorsDefaulted {
		t.Fatalf("settings did not enable anchors: %#v", options)
	}

	disabled, _, _, err := parseSearchArgs([]string{"--no-anchors", "--line-only", "needle", "sample.go"})
	if err != nil {
		t.Fatal(err)
	}
	if disabled.Anchors || disabled.AnchorsDefaulted {
		t.Fatalf("--no-anchors did not disable the setting: %#v", disabled)
	}

	count, _, _, err := parseSearchArgs([]string{"--count", "needle", "sample.go"})
	if err != nil {
		t.Fatal(err)
	}
	if count.Anchors {
		t.Fatalf("settings enabled anchors for unsupported count output: %#v", count)
	}
}

func TestParseRemoteAtRequiresOneRepository(t *testing.T) {
	options, _, remote, err := parseSearchArgs([]string{"--server", "http://search.example", "--repo", "owner/repo@tag~v1", "--at", "app.go:20"})
	if err != nil {
		t.Fatal(err)
	}
	request := searchRequestFromParams(options.Params)
	if !remote || request.At != "app.go:20" {
		t.Fatalf("remote at was not preserved: remote=%v request=%#v", remote, request)
	}
	if _, _, _, err := parseSearchArgs([]string{"--remote", "--at", "app.go:20"}); err == nil {
		t.Fatal("remote at without exactly one repository succeeded")
	}
}

func TestFollowRelatedImpliesNavigation(t *testing.T) {
	options, _, _, err := parseSearchArgs([]string{"--follow-related", "2", "needle"})
	if err != nil {
		t.Fatal(err)
	}
	if !options.Params.Related || options.Params.FollowRelated != 2 {
		t.Fatalf("follow navigation was not enabled: %#v", options.Params)
	}
	request := searchRequestFromParams(options.Params)
	if !request.Related || request.FollowRelated != 2 {
		t.Fatalf("follow navigation was not preserved in request: %#v", request)
	}
}

func TestParseAtLocationWithoutQuery(t *testing.T) {
	options, _, remote, err := parseSearchArgs([]string{"--at", "search/result.go:47"})
	if err != nil {
		t.Fatal(err)
	}
	if remote || options.Params.At != "search/result.go:47" || options.Params.Query != "" {
		t.Fatalf("unexpected --at options: %#v", options)
	}
}

func TestRelatedGoNavigationRejectsUnsupportedModes(t *testing.T) {
	for _, args := range [][]string{
		{"--related", "--line-only", "needle"},
		{"--related", "-C", "2", "needle"},
		{"--follow-related", "4", "needle"},
		{"--at", "search/result.go:47", "needle"},
		{"--anchors", "--remote", "needle"},
		{"--anchors", "--json", "needle"},
		{"--anchors", "--only-matching", "needle"},
		{"--anchors", "--outline", "sample.go"},
		{"--anchors", "--no-anchors", "needle"},
		{"--anchor-provider", "pi", "--no-anchors", "needle"},
	} {
		if _, _, _, err := parseSearchArgs(args); err == nil {
			t.Fatalf("parseSearchArgs(%q) succeeded, want an error", args)
		}
	}
}

func TestTopLevelHelpListsCommandFamilies(t *testing.T) {
	output := captureStdout(t, func() {
		if err := Run([]string{"--help"}); err != nil {
			t.Fatal(err)
		}
	})
	for _, expected := range []string{"search", "grit", "graph", "anchors", "boundaries", "examples", "extract", "architecture", "sources", "artifacts", "languages", "rules", "get", "tree", "repos", "login", "logout", "version", "--production-only", "--no-repo-config"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("top-level help missing %q:\n%s", expected, output)
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
	if !strings.Contains(searchHelp, "Search the local working directory") {
		t.Fatalf("search help missing description:\n%s", searchHelp)
	}

	directory := t.TempDir()
	path := directory + "/sample.txt"
	if err := os.WriteFile(path, []byte("graph\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	output := captureStdout(t, func() {
		if err := Run([]string{"search", "--line-only", "--no-anchors", "-F", "graph", path}); err != nil {
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
