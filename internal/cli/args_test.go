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

func TestEnclosingRequiresLineOnly(t *testing.T) {
	if err := validateEnclosingArgs(&searchArgs{Enclosing: true}); err == nil {
		t.Fatal("expected --enclosing without --line-only to fail")
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

func TestParseSearchArgsDefaultsToOneLevelRelatedNavigation(t *testing.T) {
	options, _, _, err := parseSearchArgs([]string{"needle", "sample.go"})
	if err != nil {
		t.Fatal(err)
	}
	if !options.Params.Related || options.Params.FollowRelated != 1 {
		t.Fatalf("automatic navigation=%#v", options.Params)
	}

	lineOnly, _, _, err := parseSearchArgs([]string{"--line-only", "needle", "sample.go"})
	if err != nil {
		t.Fatal(err)
	}
	if lineOnly.Params.Related || lineOnly.Params.FollowRelated != 0 {
		t.Fatalf("line-only unexpectedly enabled navigation: %#v", lineOnly.Params)
	}

	disabled, _, _, err := parseSearchArgs([]string{"--no-related", "needle", "sample.go"})
	if err != nil {
		t.Fatal(err)
	}
	if disabled.Params.Related || disabled.Params.FollowRelated != 0 {
		t.Fatalf("--no-related did not disable navigation: %#v", disabled.Params)
	}
	if _, _, _, err := parseSearchArgs([]string{"--no-related", "--related", "needle"}); err == nil {
		t.Fatal("expected contradictory related flags to fail")
	}
}

func TestParseSearchArgsEnablesRelatedGoNavigation(t *testing.T) {
	options, _, remote, err := parseSearchArgs([]string{"--related", "needle", "**/*.go"})
	if err != nil {
		t.Fatal(err)
	}
	if remote || options == nil || !options.Params.Related || options.Params.FollowRelated != 1 {
		t.Fatalf("related navigation was not enabled: remote=%v options=%#v", remote, options)
	}
	request := searchRequestFromParams(options.Params)
	if !request.Related || request.FollowRelated != 1 {
		t.Fatalf("related navigation was not preserved in request: %#v", request)
	}
}

func TestStructuralSearchDefaultsToCallerAndCalleePreviews(t *testing.T) {
	directory := chdirTemp(t)
	writeGraphSource(t, directory, "caller.go", "package sample\nfunc caller() string { return target() }\n")
	writeGraphSource(t, directory, "target.go", "package sample\nfunc target() string { return callee() + \"DEFAULT_RELATED_NEEDLE\" }\n")
	writeGraphSource(t, directory, "callee.go", "package sample\nfunc callee() string { return \"done\" }\n")
	output := captureStdout(t, func() {
		if err := Run([]string{"-F", "DEFAULT_RELATED_NEEDLE", "target.go"}); err != nil {
			t.Fatal(err)
		}
	})
	for _, expected := range []string{"← caller", "→ callee", "func caller() string", "func callee() string"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("automatic related output missing %q:\n%s", expected, output)
		}
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

func TestRemovedAnchorSelectionFlagsAreRejected(t *testing.T) {
	for _, args := range [][]string{
		{"--anchors", "--line-only", "needle"},
		{"--anchor-provider", "pi", "--line-only", "needle"},
	} {
		if _, _, _, err := parseSearchArgs(args); err == nil {
			t.Fatalf("removed anchor selection args %v succeeded", args)
		}
	}
}

func TestAnchorsDefaultToNativeAndCanBeDisabled(t *testing.T) {
	settingsPath := t.TempDir() + "/settings.json"
	t.Setenv("GREPPLE_SETTINGS", settingsPath)

	options, _, _, err := parseSearchArgs([]string{"--line-only", "needle", "sample.go"})
	if err != nil {
		t.Fatal(err)
	}
	if !options.Anchors {
		t.Fatalf("anchors were not enabled by default: %#v", options)
	}
	native, err := useNativeAnchorProvider()
	if err != nil || !native {
		t.Fatalf("default provider was not native: native=%v err=%v", native, err)
	}

	writeJSONFile(t, settingsPath, userSettings{Anchors: anchorSettings{EnabledByDefault: true, DefaultProvider: "pi"}})
	configured, _, _, err := parseSearchArgs([]string{"--line-only", "needle", "sample.go"})
	if err != nil {
		t.Fatal(err)
	}
	if !configured.Anchors {
		t.Fatalf("configured provider did not enable anchors: %#v", configured)
	}
	native, err = useNativeAnchorProvider()
	if err != nil || native {
		t.Fatalf("configured default provider was ignored: native=%v err=%v", native, err)
	}

	if _, _, _, err := parseSearchArgs([]string{"--no-anchors", "needle", "sample.go"}); err == nil {
		t.Fatal("removed --no-anchors flag succeeded")
	}
	count, _, _, err := parseSearchArgs([]string{"--count", "needle", "sample.go"})
	if err != nil {
		t.Fatal(err)
	}
	if count.Anchors {
		t.Fatalf("default anchors were enabled for unsupported count output: %#v", count)
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

func TestParseAtSupportsExactAnchoredAndContextOutput(t *testing.T) {
	for _, args := range [][]string{
		{"--at", "search/result.go:40-45", "--line-only"},
		{"--at", "search/result.go:40-45", "-C", "2"},
	} {
		options, _, _, err := parseSearchArgs(args)
		if err != nil {
			t.Fatalf("parseSearchArgs(%q): %v", args, err)
		}
		if !options.Anchors || options.Params.At != "search/result.go:40-45" {
			t.Fatalf("options=%+v", options)
		}
	}
	if _, _, _, err := parseSearchArgs([]string{"--at", "search/result.go:40", "--files"}); err == nil {
		t.Fatal("--at with --files succeeded")
	}
}

func TestRelatedGoNavigationRejectsUnsupportedModes(t *testing.T) {
	for _, args := range [][]string{
		{"--related", "--line-only", "needle"},
		{"--related", "-C", "2", "needle"},
		{"--follow-related", "4", "needle"},
		{"--at", "search/result.go:47", "needle"},
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
	for _, expected := range []string{"search", "write", "grit", "graph", "anchors", "boundaries", "examples", "extract", "architecture", "sources", "artifacts", "languages", "rules", "get", "tree", "repos", "refs", "ask", "ai-provider", "login", "logout", "version", "--production-only", "--no-repo-config"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("top-level help missing %q:\n%s", expected, output)
		}
	}
	if count := strings.Count(output, "  refs         "); count != 1 {
		t.Fatalf("top-level help lists refs %d times:\n%s", count, output)
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
