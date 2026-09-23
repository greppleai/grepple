package auth

import (
	"testing"

	"github.com/greppleai/grepple/internal/cliruntime"
)

func TestCommandsDispatchOwnedAuthenticationOperations(t *testing.T) {
	called := ""
	application := cliruntime.Environment{}
	for _, test := range []struct {
		command interface{ Run([]string) error }
		want    string
	}{{newCommand(application, operationAIProvider, func([]string) error { called = "provider"; return nil }), "provider"}, {newCommand(application, operationLogin, func([]string) error { called = "login"; return nil }), "login"}, {newCommand(application, operationLogout, func([]string) error { called = "logout"; return nil }), "logout"}} {
		called = ""
		if err := test.command.Run(nil); err != nil {
			t.Fatal(err)
		}
		if called != test.want {
			t.Fatalf("called=%q want=%q", called, test.want)
		}
	}
}
