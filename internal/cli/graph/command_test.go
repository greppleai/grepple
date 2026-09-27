package graph

import (
	"bytes"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/cliruntime"
)

func TestCommandOwnsGraphDispatchAndHelp(t *testing.T) {
	for _, test := range []struct {
		args []string
		want string
	}{{[]string{"callers", "--help"}, "Query a deterministic"}, {[]string{"resolve", "--help"}, "Preview every declaration"}} {
		var output bytes.Buffer
		command := New(cliruntime.Environment{Output: &output})
		if err := command.Run(test.args); err != nil {
			t.Fatalf("Run(%v): %v", test.args, err)
		}
		if !strings.Contains(output.String(), test.want) {
			t.Fatalf("Run(%v) output=%q", test.args, output.String())
		}
	}
	for _, removed := range []string{"build", "diff"} {
		if err := New(cliruntime.Environment{}).Run([]string{removed}); err == nil || !strings.Contains(err.Error(), "has been removed") {
			t.Fatalf("graph %s should be rejected: %v", removed, err)
		}
	}
}
