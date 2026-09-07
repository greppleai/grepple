package grepplecli

import "testing"

func TestParseSearchArgs(t *testing.T) {
	options, server, remote, err := parseSearchArgs([]string{
		"--line-only",
		"--ignore-case",
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
	if !options.LineOnly || !options.Params.IgnoreCase || options.Params.MaxFiles != 5 {
		t.Fatalf("unexpected options: %#v", options)
	}
	if options.Params.Query != "needle" || len(options.Params.Globs) != 1 {
		t.Fatalf("unexpected search parameters: %#v", options.Params)
	}
}
