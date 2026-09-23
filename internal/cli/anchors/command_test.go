package anchors

import "testing"

func TestCommandDispatchesOperations(t *testing.T) {
	called := ""
	command := newWithDependencies(dependencies{Help: func() error { called = "help"; return nil }, Doctor: func([]string) error { called = "doctor"; return nil }, Setup: func([]string) error { called = "setup"; return nil }})
	for _, test := range []struct {
		args []string
		want string
	}{{nil, "help"}, {[]string{"doctor"}, "doctor"}, {[]string{"setup"}, "setup"}} {
		called = ""
		if err := command.Run(test.args); err != nil {
			t.Fatal(err)
		}
		if called != test.want {
			t.Fatalf("Run(%v)=%q want %q", test.args, called, test.want)
		}
	}
}
