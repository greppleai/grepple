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
	}{{[]string{"--help"}, "Build a deterministic"}, {[]string{"diff", "--help"}, "Compare semantic"}, {[]string{"callers", "--help"}, "Query a deterministic"}, {[]string{"resolve", "--help"}, "Preview every declaration"}} {
		var output bytes.Buffer
		command := New(cliruntime.Environment{Output: &output})
		if err := command.Run(test.args); err != nil {
			t.Fatalf("Run(%v): %v", test.args, err)
		}
		if !strings.Contains(output.String(), test.want) {
			t.Fatalf("Run(%v) output=%q", test.args, output.String())
		}
	}
}
