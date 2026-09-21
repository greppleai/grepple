package graph

import "testing"

func TestRunDispatchesGraphOperations(t *testing.T) {
	called := ""
	dependencies := Dependencies{Build: func([]string) error { called = "build"; return nil }, Diff: func([]string) error { called = "diff"; return nil }, Query: func([]string) error { called = "query"; return nil }}
	for _, test := range []struct {
		args []string
		want string
	}{{nil, "build"}, {[]string{"diff"}, "diff"}, {[]string{"callers"}, "query"}} {
		called = ""
		if err := Run(test.args, dependencies); err != nil {
			t.Fatal(err)
		}
		if called != test.want {
			t.Fatalf("Run(%v) called %q, want %q", test.args, called, test.want)
		}
	}
}
