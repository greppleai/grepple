package cli

import (
	"strings"
	"testing"
)

func TestRunDispatchesLanguagesCommand(t *testing.T) {
	output := captureStdout(t, func() {
		if err := Run([]string{"languages", "--json"}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(output, `"language"`) || !strings.Contains(output, `"navigationFacts"`) {
		t.Fatalf("output=%q", output)
	}
}
