package write

import (
	"bytes"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/cliruntime"
)

func TestCommandUsesCommandContextStreamsAndExit(t *testing.T) {
	var output bytes.Buffer
	exitCode := 0
	application := cliruntime.Environment{
		Input:  strings.NewReader("{"),
		Output: &output,
		Exit:   func(code int) { exitCode = code },
	}
	if err := New(application).Run([]string{"--json"}); err != nil {
		t.Fatal(err)
	}
	if exitCode != 1 {
		t.Fatalf("exit=%d", exitCode)
	}
	if !strings.Contains(output.String(), "invalid_request") {
		t.Fatalf("output=%q", output.String())
	}
}
