package pihooks

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFindBlockedInvocation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		command string
		want    string
	}{
		{name: "direct", command: "grep needle file", want: "grep"},
		{name: "absolute path", command: "/usr/bin/rg needle .", want: "rg"},
		{name: "all blocked names", command: "find .; grep x; egrep x; fgrep x; rg x; ag x; ack x; fd x; fdfind x; locate x", want: "find"},
		{name: "pipeline", command: "go test ./... | rg FAIL", want: "rg"},
		{name: "and list", command: "printf ok && fd test", want: "fd"},
		{name: "or list", command: "false || ack TODO", want: "ack"},
		{name: "newline", command: "echo ok\nlocate passwd", want: "locate"},
		{name: "command substitution", command: `value=$(grep x file)`, want: "grep"},
		{name: "backtick substitution", command: "value=`fgrep x file`", want: "fgrep"},
		{name: "nested substitution", command: `echo "$(printf '%s' "$(ag x .)")"`, want: "ag"},
		{name: "process substitution", command: "diff <(sort a) <(find . -type f)", want: "find"},
		{name: "subshell", command: "(cd src; rg package)", want: "rg"},
		{name: "if condition", command: "if grep -q x file; then echo yes; fi", want: "grep"},
		{name: "quoted command name", command: `'grep' x`, want: "grep"},
		{name: "concatenated command name", command: `gr""ep x`, want: "grep"},
		{name: "escaped command name", command: `gr\ep x`, want: "grep"},
		{name: "assignment prefix", command: "LC_ALL=C grep x file", want: "grep"},
		{name: "shell command string", command: `bash -lc 'rg needle .'`, want: "rg"},
		{name: "wrapped shell command string", command: `sudo env X=1 sh -c 'find . -type f'`, want: "find"},
		{name: "eval string", command: `eval 'fd needle .'`, want: "fd"},
		{name: "env split string", command: `env -S 'grep needle file'`, want: "grep"},
		{name: "interpreter text as ordinary argument", command: `echo env -S 'grep needle file'`},
		{name: "shell option without command mode", command: `bash --norc 'grep needle file'`},
		{name: "allow marker comment", command: "grep x file # grep-guard:allow"},
		{name: "allow marker preserved anywhere", command: `printf '%s' grep-guard:allow; grep x`},
		{name: "argument mention", command: "echo grep rg find"},
		{name: "quoted script data", command: `printf '%s\n' 'grep x | find .'`},
		{name: "comment", command: "echo ok # grep x"},
		{name: "variable", command: "grep_program=grep; echo $grep_program"},
		{name: "dynamic command", command: `$program needle`},
		{name: "function name is not invocation", command: "grep() { echo replacement; }"},
		{name: "heredoc body", command: "cat <<'EOF'\ngrep x\nEOF"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := FindBlockedInvocation(test.command); got != test.want {
				t.Fatalf("FindBlockedInvocation(%q) = %q, want %q", test.command, got, test.want)
			}
		})
	}
}

func TestFindBlockedInvocationWrappers(t *testing.T) {
	t.Parallel()
	tests := []struct {
		command string
		want    string
	}{
		{"sudo grep x", "grep"},
		{"command -- /bin/fgrep x", "fgrep"},
		{"builtin grep x", "grep"},
		{"exec -a search /usr/bin/rg x", "rg"},
		{"printf '%s\\0' . | xargs -0 find", "find"},
		{"xargs -n 1 grep pattern", "grep"},
		{"time -f '%e' ag x", "ag"},
		{"nice -n 10 ack x", "ack"},
		{"env LC_ALL=C fd pattern", "fd"},
		{"env -u HOME fdfind pattern", "fdfind"},
		{"noglob locate pattern", "locate"},
		{"sudo -u root -- env LC_ALL=C nice -n 2 rg x", "rg"},
		{"sudo --user=root /usr/bin/find .", "find"},
		{"echo sudo grep x", ""},
		{"env LC_ALL=C printf grep", ""},
		{"nice -n 5 echo rg", ""},
	}
	for _, test := range tests {
		test := test
		t.Run(test.command, func(t *testing.T) {
			t.Parallel()
			if got := FindBlockedInvocation(test.command); got != test.want {
				t.Errorf("got %q, want %q", got, test.want)
			}
		})
	}
}

func TestFindBlockedInvocationUsesSourceOrder(t *testing.T) {
	t.Parallel()
	if got := FindBlockedInvocation("echo ok | rg x; find ."); got != "rg" {
		t.Fatalf("got %q, want first invocation rg", got)
	}
}

func TestHandleHookDeniesWithProtocolOutput(t *testing.T) {
	t.Parallel()
	input := []byte(`{"tool_name":"Bash","tool_input":{"command":"go test ./... | grep FAIL"}}`)
	output := HandleHook(input)
	if len(output) == 0 {
		t.Fatal("HandleHook returned allow for blocked command")
	}

	var got HookOutput
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}
	specific := got.HookSpecificOutput
	if specific.HookEventName != "PreToolUse" || specific.PermissionDecision != "deny" {
		t.Fatalf("unexpected protocol output: %+v", specific)
	}
	for _, text := range []string{"Blocked: 'grep'", "grepple", AllowMarker} {
		if !strings.Contains(specific.PermissionDecisionReason, text) {
			t.Errorf("reason %q does not contain %q", specific.PermissionDecisionReason, text)
		}
	}
}

func TestHandleHookFailsOpen(t *testing.T) {
	t.Parallel()
	for _, input := range [][]byte{
		[]byte(`not json`),
		[]byte(`{}`),
		[]byte(`{"tool_input":{"command":42}}`),
		[]byte(`{"tool_input":{"command":"echo grep"}}`),
		[]byte(`{"tool_input":{"command":"grep x # grep-guard:allow"}}`),
	} {
		if output := HandleHook(input); len(output) != 0 {
			t.Errorf("HandleHook(%q) = %q, want empty allow output", input, output)
		}
	}
}

func TestDenialReason(t *testing.T) {
	t.Parallel()
	reason := DenialReason("fd")
	if !strings.HasPrefix(reason, "Blocked: 'fd'") {
		t.Fatalf("unexpected reason: %q", reason)
	}
	if !strings.Contains(reason, "grepple --outline") {
		t.Fatalf("reason omits grepple guidance: %q", reason)
	}
}
