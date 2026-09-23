package get

import (
	"bytes"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/cliruntime"
)

func TestCommandUsesCommandContextOutput(t *testing.T) {
	var output bytes.Buffer
	if err := New(cliruntime.Environment{Output: &output}).Run([]string{"--help"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Fetch a file from an indexed repository") {
		t.Fatalf("help=%q", output.String())
	}
}

func TestOutlineRenderingUsesCommandOutputAndExit(t *testing.T) {
	var output bytes.Buffer
	exitCode := 0
	application := cliruntime.Environment{Output: &output, Exit: func(code int) { exitCode = code }}
	if err := render(application, request{Path: "plain.txt", Outline: true}, []byte("plain text")); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "plain.txt") || exitCode != 1 {
		t.Fatalf("output=%q exit=%d", output.String(), exitCode)
	}
}
