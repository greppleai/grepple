package architecture

import (
	"bytes"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/cliruntime"
)

func TestCommandUsesCommandContextOutput(t *testing.T) {
	var output bytes.Buffer
	if err := New(cliruntime.Environment{Output: &output}).Run(nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Inspect language-neutral directory architecture") {
		t.Fatalf("help=%q", output.String())
	}
}
