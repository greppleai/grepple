package cli

import (
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/cliruntime"
)

func TestRepositoryOptionsAppearInContinuation(t *testing.T) {
	options := cliruntime.RepositoryInvocationOptions{ProductionOnly: true, NoConfigIgnore: true, NoRepositoryConfig: true}
	continuation := graphContinuationCommandWithOptions(options, "graph", []string{"."}, &navigationGraphTruncation{Reason: "max_files", Limit: 1, Skipped: 1})
	if !strings.Contains(continuation, "--production-only") || !strings.Contains(continuation, "--no-repo-config") {
		t.Fatalf("continuation=%q", continuation)
	}
}
