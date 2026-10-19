package grepguard

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestHandleCommandGuard(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name    string
		input   string
		blocked bool
	}{
		{name: "blocked", input: `{"command":"rg needle ."}`, blocked: true},
		{name: "allowed", input: `{"command":"echo rg"}`},
		{name: "bypass", input: `{"command":"rg needle . # grep-guard:allow"}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			output := HandleCommandGuard([]byte(test.input))
			var response CommandGuardResponse
			if err := json.Unmarshal(output, &response); err != nil {
				t.Fatalf("invalid response %q: %v", output, err)
			}
			if response.Blocked != test.blocked {
				t.Fatalf("blocked = %t, want %t", response.Blocked, test.blocked)
			}
			if test.blocked && !strings.Contains(response.Reason, "grepple") {
				t.Fatalf("blocked response omitted remediation: %q", response.Reason)
			}
		})
	}
}

func TestHandleCommandGuardFailsOpen(t *testing.T) {
	t.Parallel()
	for _, input := range [][]byte{[]byte(`not json`), []byte(`{}`), []byte(`{"command":42}`)} {
		if output := HandleCommandGuard(input); len(output) != 0 {
			t.Errorf("HandleCommandGuard(%q) = %q, want empty response", input, output)
		}
	}
}
