package auth

import "testing"

func TestCommandsShareAuthenticationDependencies(t *testing.T) {
	called := ""
	dependencies := Dependencies{AIProvider: func([]string) error { called = "provider"; return nil }, Login: func([]string) error { called = "login"; return nil }, Logout: func([]string) error { called = "logout"; return nil }}
	for _, test := range []struct {
		command interface{ Run([]string) error }
		want    string
	}{{NewAIProvider(dependencies), "provider"}, {NewLogin(dependencies), "login"}, {NewLogout(dependencies), "logout"}} {
		called = ""
		if err := test.command.Run(nil); err != nil {
			t.Fatal(err)
		}
		if called != test.want {
			t.Fatalf("called=%q want=%q", called, test.want)
		}
	}
}
