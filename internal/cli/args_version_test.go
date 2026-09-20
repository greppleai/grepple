package cli

import (
	"strings"
	"testing"
)

func TestRunDispatchesVersionAliases(t *testing.T) {
	for _, argument := range []string{"--version", "version"} {
		output := captureStdout(t, func() {
			if err := Run([]string{argument}); err != nil {
				t.Fatal(err)
			}
		})
		if !strings.Contains(output, "grepple ") || !strings.Contains(output, "commit ") {
			t.Fatalf("%s output=%q", argument, output)
		}
	}
}
