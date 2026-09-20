package runtime

import "testing"

func TestExitStateAndError(t *testing.T) {
	var state ExitState
	state.Request(1)
	if state.Requested() != 1 {
		t.Fatalf("requested=%d", state.Requested())
	}
	state.Reset()
	if state.Requested() != 0 {
		t.Fatalf("requested after reset=%d", state.Requested())
	}
	if code, ok := ExitCode(NewExitError(3)); !ok || code != 3 {
		t.Fatalf("exit code=%d ok=%v", code, ok)
	}
}
