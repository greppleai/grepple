package grepguard

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestClaudeHook(t *testing.T) {
	for _, test := range []struct {
		payload string
		deny    bool
	}{
		{`{"tool_name":"Grep","tool_input":{"pattern":"x"}}`, true},
		{`{"tool_name":"Glob","tool_input":{"pattern":"*.go"}}`, true},
		{`{"tool_name":"Bash","tool_input":{"command":"echo grep"}}`, false},
		{`{"tool_name":"Bash","tool_input":{"command":"rg -n x ."}}`, true},
		{`{"tool_name":"Bash","tool_input":{"command":"rg x # grep-guard:allow"}}`, false},
		{`{"tool_name":"Read","tool_input":{"command":"rg x"}}`, false},
		{`broken`, false},
	} {
		output := HandleClaudeHook([]byte(test.payload))
		if (len(output) > 0) != test.deny {
			t.Errorf("input %s: deny=%t, want %t", test.payload, len(output) > 0, test.deny)
		}
		if test.deny {
			var result HookOutput
			if err := json.Unmarshal(output, &result); err != nil || result.HookSpecificOutput.PermissionDecision != "deny" || !strings.Contains(result.HookSpecificOutput.PermissionDecisionReason, "grepple") {
				t.Errorf("invalid denial: %s (%v)", output, err)
			}
		}
	}
}
