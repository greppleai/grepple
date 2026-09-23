package outputspill

import (
	"reflect"
	"testing"
)

func TestParseKeepsCommandArgumentsAndConsumesProcessFlags(t *testing.T) {
	args, options, err := Parse([]string{"search", "--json", "--spill-threshold-bytes=128", "--artifact-dir", "artifacts", "needle"})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"search", "--json", "needle"}; !reflect.DeepEqual(args, want) {
		t.Fatalf("args=%q want=%q", args, want)
	}
	if options.Disabled || options.Threshold != 128 || options.Directory != "artifacts" {
		t.Fatalf("options=%+v", options)
	}
}

func TestParseHonorsLiteralSeparator(t *testing.T) {
	args, options, err := Parse([]string{"search", "--", "--no-spill", "--spill-threshold-bytes=1"})
	if err != nil {
		t.Fatal(err)
	}
	if options.Disabled || options.Threshold != -1 {
		t.Fatalf("options=%+v", options)
	}
	if want := []string{"search", "--", "--no-spill", "--spill-threshold-bytes=1"}; !reflect.DeepEqual(args, want) {
		t.Fatalf("args=%q want=%q", args, want)
	}
}

func TestParseRejectsInvalidProcessFlags(t *testing.T) {
	for _, args := range [][]string{{"--spill-threshold-bytes=0"}, {"--spill-threshold-bytes"}, {"--artifact-dir="}} {
		if _, _, err := Parse(args); err == nil {
			t.Fatalf("Parse(%q) succeeded", args)
		}
	}
}
