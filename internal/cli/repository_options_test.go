package cli

import (
	"strings"
	"testing"
)

func TestParseRepositoryInvocationOptions(t *testing.T) {
	args, options, err := parseRepositoryInvocationOptions([]string{"--production-only", "search", "needle", "--no-config-ignore", "--no-repo-config", "--", "--production-only"})
	if err != nil {
		t.Fatal(err)
	}
	if !options.productionOnly || !options.ignoreDisabled || !options.disabled {
		t.Fatalf("options=%+v", options)
	}
	if len(args) != 4 || args[0] != "search" || args[3] != "--production-only" {
		t.Fatalf("args=%#v", args)
	}
	previous := activeRepositoryOptions
	activeRepositoryOptions = options
	continuation := graphContinuationCommand("graph", []string{"."}, &navigationGraphTruncation{Reason: "max_files", Limit: 1, Skipped: 1})
	activeRepositoryOptions = previous
	if !strings.Contains(continuation, "--production-only") || !strings.Contains(continuation, "--no-repo-config") {
		t.Fatalf("continuation=%q", continuation)
	}
}
