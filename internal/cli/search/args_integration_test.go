package search

import (
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/anchor"
)

func TestParseSearchArgs(t *testing.T) {
	options, server, remote, err := parseTestSearchArgs([]string{
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
	if _, _, _, err := parseTestSearchArgs([]string{"--enclosing", "needle"}); err == nil {
		t.Fatal("expected --enclosing without --line-only to fail")
	}
}

func TestParseSearchArgsAcceptsSafeGrepCompatibilityAliases(t *testing.T) {
	options, _, _, err := parseTestSearchArgs([]string{"-r", "-E", "needle", "src"})
	if err != nil {
		t.Fatal(err)
	}
	if options.Params.Query != "needle" || !options.Params.Regex {
		t.Fatalf("compatibility aliases produced unexpected options: %#v", options.Params)
	}
	if options.MaxOutputBytes != DefaultTextOutputBytes {
		t.Fatalf("default output cap = %d, want %d", options.MaxOutputBytes, DefaultTextOutputBytes)
	}

	unbounded, _, _, err := parseTestSearchArgs([]string{"--max-output-bytes", "0", "needle"})
	if err != nil {
		t.Fatal(err)
	}
	if unbounded.MaxOutputBytes != 0 {
		t.Fatalf("--max-output-bytes 0 = %d, want unbounded", unbounded.MaxOutputBytes)
	}
	if _, _, _, err := parseTestSearchArgs([]string{"--sort", "score", "needle"}); err == nil || !strings.Contains(err.Error(), "path") {
		t.Fatalf("invalid sort error = %v", err)
	}
}

func TestParseSearchArgsDefaultsToOneLevelRelatedNavigation(t *testing.T) {
	options, _, _, err := parseTestSearchArgs([]string{"needle", "sample.go"})
	if err != nil {
		t.Fatal(err)
	}
	if !options.Params.Related || options.Params.FollowRelated != 1 {
		t.Fatalf("automatic navigation=%#v", options.Params)
	}

	lineOnly, _, _, err := parseTestSearchArgs([]string{"--line-only", "needle", "sample.go"})
	if err != nil {
		t.Fatal(err)
	}
	if lineOnly.Params.Related || lineOnly.Params.FollowRelated != 0 {
		t.Fatalf("line-only unexpectedly enabled navigation: %#v", lineOnly.Params)
	}

	disabled, _, _, err := parseTestSearchArgs([]string{"--no-related", "needle", "sample.go"})
	if err != nil {
		t.Fatal(err)
	}
	if disabled.Params.Related || disabled.Params.FollowRelated != 0 {
		t.Fatalf("--no-related did not disable navigation: %#v", disabled.Params)
	}
	if _, _, _, err := parseTestSearchArgs([]string{"--no-related", "--related", "needle"}); err == nil {
		t.Fatal("expected contradictory related flags to fail")
	}
}

func TestParseSearchArgsSupportsRepeatSource(t *testing.T) {
	options, _, _, err := parseTestSearchArgs([]string{"--repeat-source", "needle"})
	if err != nil {
		t.Fatal(err)
	}
	if !options.RepeatSource {
		t.Fatal("--repeat-source was not retained")
	}
	for _, args := range [][]string{{"--repeat-source", "--line-only", "needle"}, {"--repeat-source", "--json", "needle"}, {"--repeat-source", "-C", "2", "needle"}} {
		if _, _, _, err := parseTestSearchArgs(args); err == nil {
			t.Fatalf("--repeat-source accepted unsupported output: %v", args)
		}
	}
	if _, _, _, err := parseTestSearchArgs([]string{"--repeat-source", "--at", "sample.go:1-3", "--line-only"}); err != nil {
		t.Fatalf("--repeat-source rejected focused line output: %v", err)
	}
}

func TestParseSearchArgsEnablesRelatedGoNavigation(t *testing.T) {
	options, _, remote, err := parseTestSearchArgs([]string{"--related", "needle", "**/*.go"})
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

func TestStructuralSearchDefaultsToCallerAndCalleeLocations(t *testing.T) {
	directory := chdirTemp(t)
	writeGraphSource(t, directory, "caller.go", "package sample\nfunc caller() string { return target() }\n")
	writeGraphSource(t, directory, "target.go", "package sample\nfunc target() string { return callee() + \"DEFAULT_RELATED_NEEDLE\" }\n")
	writeGraphSource(t, directory, "callee.go", "package sample\nfunc callee() string { return \"done\" }\n")
	output := captureStdout(t, func() {
		if err := Run([]string{"-F", "DEFAULT_RELATED_NEEDLE", "target.go"}); err != nil {
			t.Fatal(err)
		}
	})
	for _, expected := range []string{"← caller  caller.go:2-2", "→ callee  callee.go:2-2"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("automatic related output missing %q:\n%s", expected, output)
		}
	}
	for _, body := range []string{"func caller() string", "func callee() string"} {
		if strings.Contains(output, body) {
			t.Fatalf("automatic related output inlined %q:\n%s", body, output)
		}
	}
}

func TestParseSearchArgsEnablesRemoteNavigation(t *testing.T) {
	options, _, remote, err := parseTestSearchArgs([]string{"--server", "http://search.example", "--follow-related", "1", "needle"})
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
		if _, _, _, err := parseTestSearchArgs(args); err == nil {
			t.Fatalf("removed anchor selection args %v succeeded", args)
		}
	}
}

func TestAnchorsDefaultToNativeAndCanBeDisabled(t *testing.T) {
	settingsPath := t.TempDir() + "/settings.json"
	t.Setenv("GREPPLE_SETTINGS", settingsPath)

	options, _, _, err := parseTestSearchArgs([]string{"--line-only", "needle", "sample.go"})
	if err != nil {
		t.Fatal(err)
	}
	if !options.Anchors {
		t.Fatalf("anchors were not enabled by default: %#v", options)
	}
	native, err := anchor.UseNativeProvider()
	if err != nil || !native {
		t.Fatalf("default provider was not native: native=%v err=%v", native, err)
	}

	writeJSONFile(t, settingsPath, userSettings{Anchors: anchorSettings{EnabledByDefault: true, DefaultProvider: "pi"}})
	configured, _, _, err := parseTestSearchArgs([]string{"--line-only", "needle", "sample.go"})
	if err != nil {
		t.Fatal(err)
	}
	if !configured.Anchors {
		t.Fatalf("configured provider did not enable anchors: %#v", configured)
	}
	native, err = anchor.UseNativeProvider()
	if err != nil || native {
		t.Fatalf("configured default provider was ignored: native=%v err=%v", native, err)
	}

	if _, _, _, err := parseTestSearchArgs([]string{"--no-anchors", "needle", "sample.go"}); err == nil {
		t.Fatal("removed --no-anchors flag succeeded")
	}
	count, _, _, err := parseTestSearchArgs([]string{"--count", "needle", "sample.go"})
	if err != nil {
		t.Fatal(err)
	}
	if count.Anchors {
		t.Fatalf("default anchors were enabled for unsupported count output: %#v", count)
	}
}

func TestParseRemoteAtRequiresOneRepository(t *testing.T) {
	options, _, remote, err := parseTestSearchArgs([]string{"--server", "http://search.example", "--repo", "owner/repo@tag~v1", "--at", "app.go:20"})
	if err != nil {
		t.Fatal(err)
	}
	request := searchRequestFromParams(options.Params)
	if !remote || request.At != "app.go:20" {
		t.Fatalf("remote at was not preserved: remote=%v request=%#v", remote, request)
	}
	if _, _, _, err := parseTestSearchArgs([]string{"--remote", "--at", "app.go:20"}); err == nil {
		t.Fatal("remote at without exactly one repository succeeded")
	}
}

func TestFollowRelatedImpliesNavigation(t *testing.T) {
	options, _, _, err := parseTestSearchArgs([]string{"--follow-related", "2", "needle"})
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
	options, _, remote, err := parseTestSearchArgs([]string{"--at", "search/result.go:47"})
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
		options, _, _, err := parseTestSearchArgs(args)
		if err != nil {
			t.Fatalf("parseTestSearchArgs(%q): %v", args, err)
		}
		if !options.Anchors || options.Params.At != "search/result.go:40-45" {
			t.Fatalf("options=%+v", options)
		}
	}
	if _, _, _, err := parseTestSearchArgs([]string{"--at", "search/result.go:40", "--files"}); err == nil {
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
		if _, _, _, err := parseTestSearchArgs(args); err == nil {
			t.Fatalf("parseTestSearchArgs(%q) succeeded, want an error", args)
		}
	}
}

func TestParseAtAllowsExplicitServerForLocalDependencyResolution(t *testing.T) {
	options, server, remote, err := parseTestSearchArgs([]string{"--local", "--server", "http://localhost:8080", "--at", "sample.go:3"})
	if err != nil {
		t.Fatal(err)
	}
	if options == nil || server != "http://localhost:8080" || remote {
		t.Fatalf("options=%#v server=%q remote=%v", options, server, remote)
	}
}
