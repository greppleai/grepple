package context

import "testing"

func TestRunParsesInvalidationReason(t *testing.T) {
	got := ""
	if err := Run([]string{"invalidate", "--reason", "compact"}, Dependencies{Invalidate: func(reason string) error { got = reason; return nil }}); err != nil {
		t.Fatal(err)
	}
	if got != "compact" {
		t.Fatalf("reason=%q", got)
	}
	if err := Run([]string{"invalidate", "--unknown"}, Dependencies{Invalidate: func(string) error { return nil }}); err == nil {
		t.Fatal("expected unknown argument error")
	}
}
