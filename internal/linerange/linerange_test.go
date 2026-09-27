package linerange

import (
	"errors"
	"testing"
)

func TestResolveClampsOnlyRangesThatStartInsideFile(t *testing.T) {
	for _, test := range []struct {
		start, end     int
		wantStart      int
		wantEnd        int
		wantOutcome    Outcome
		wantOutsideErr bool
	}{
		{start: 1, end: 30, wantStart: 1, wantEnd: 28, wantOutcome: OutcomePartialMiss},
		{start: 20, end: 40, wantStart: 20, wantEnd: 28, wantOutcome: OutcomePartialMiss},
		{start: 28, end: 40, wantStart: 28, wantEnd: 28, wantOutcome: OutcomePartialMiss},
		{start: 29, end: 40, wantOutcome: OutcomeFullMiss, wantOutsideErr: true},
		{start: 50, end: 60, wantOutcome: OutcomeFullMiss, wantOutsideErr: true},
	} {
		result, err := Resolve(test.start, test.end, 28)
		var outside *OutsideError
		if errors.As(err, &outside) != test.wantOutsideErr {
			t.Fatalf("Resolve(%d, %d) error=%v", test.start, test.end, err)
		}
		if result.Outcome != test.wantOutcome || result.ReturnedStart != test.wantStart || result.ReturnedEnd != test.wantEnd {
			t.Fatalf("Resolve(%d, %d)=%#v", test.start, test.end, result)
		}
		if test.wantOutcome == OutcomePartialMiss && result.Warning == "" {
			t.Fatalf("Resolve(%d, %d) omitted EOF warning", test.start, test.end)
		}
	}
}

func TestSplitLinesUsesEditorLineCounts(t *testing.T) {
	if got := len(SplitLines("one\ntwo\n")); got != 2 {
		t.Fatalf("line count=%d, want 2", got)
	}
	if got := len(SplitLines("")); got != 1 {
		t.Fatalf("empty line count=%d, want 1", got)
	}
}
