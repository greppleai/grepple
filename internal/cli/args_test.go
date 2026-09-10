package cli

import "testing"

func TestParseSearchArgs(t *testing.T) {
	options, server, remote, err := parseSearchArgs([]string{
		"--line-only",
		"--ignore-case",
		"--invert-match",
		"--max-files", "5",
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
	if !options.LineOnly || !options.Params.IgnoreCase || !options.Params.InvertMatch || options.Params.MaxFiles != 5 {
		t.Fatalf("unexpected options: %#v", options)
	}
	if options.Params.Query != "needle" || len(options.Params.Globs) != 1 {
		t.Fatalf("unexpected search parameters: %#v", options.Params)
	}
	request := searchRequestFromParams(options.Params)
	if request.InvertMatch == nil || !*request.InvertMatch {
		t.Fatalf("invert-match was not preserved in the remote request: %#v", request)
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

func TestAnchorProviderFlagEnablesAnchoredOutput(t *testing.T) {
	options, _, remote, err := parseSearchArgs([]string{"--anchor-provider", "pi", "--line-only", "needle"})
	if err != nil {
		t.Fatal(err)
	}
	if remote || !options.Anchors || options.AnchorProvider != "pi" {
		t.Fatalf("anchor provider was not enabled: remote=%v options=%#v", remote, options)
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
		{"--related", "--remote", "needle"},
		{"--related", "--line-only", "needle"},
		{"--related", "-C", "2", "needle"},
		{"--follow-related", "4", "needle"},
		{"--at", "search/result.go:47", "needle"},
		{"--anchors", "--remote", "needle"},
		{"--anchors", "--json", "needle"},
		{"--anchors", "--only-matching", "needle"},
		{"--anchors", "--outline", "sample.go"},
	} {
		if _, _, _, err := parseSearchArgs(args); err == nil {
			t.Fatalf("parseSearchArgs(%q) succeeded, want an error", args)
		}
	}
}
