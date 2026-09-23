package grit

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
	if !strings.Contains(output.String(), "Run native GritQL") {
		t.Fatalf("help=%q", output.String())
	}
}
