package runtime

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestBoundedOutputWritesCompleteLinesAndMarker(t *testing.T) {
	var destination bytes.Buffer
	output := NewBoundedOutput(&destination, 12)
	err := output.WriteString("one\ntwo\nthree\n")
	if !errors.Is(err, ErrOutputTruncated) {
		t.Fatalf("error=%v", err)
	}
	if got := destination.String(); !strings.HasSuffix(got, "…\n") || strings.Contains(got, "thr") {
		t.Fatalf("output=%q", got)
	}
	if output.Written() != destination.Len() {
		t.Fatalf("written=%d len=%d", output.Written(), destination.Len())
	}
}

func TestOutputWritesJSONWithoutEscapingHTML(t *testing.T) {
	var destination bytes.Buffer
	if err := NewOutput(&destination).WriteJSON(map[string]string{"value": "<tag>"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(destination.String(), `"<tag>"`) {
		t.Fatalf("output=%q", destination.String())
	}
}
