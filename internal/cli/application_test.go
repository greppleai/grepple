package cli

import (
	"reflect"
	"testing"

	cliruntime "github.com/greppleai/grepple/internal/cli/runtime"
)

func TestApplicationDispatchesNamedAndDefaultCommands(t *testing.T) {
	var namedArgs, defaultArgs []string
	app := &application{commands: make(map[string]commandSpec), defaultCommand: cliruntime.CommandFunc(func(args []string) error {
		defaultArgs = append([]string(nil), args...)
		return nil
	})}
	app.register("named", cliruntime.CommandFunc(func(args []string) error {
		namedArgs = append([]string(nil), args...)
		return nil
	}))
	if err := app.run([]string{"named", "--flag"}); err != nil {
		t.Fatal(err)
	}
	if want := []string{"--flag"}; !reflect.DeepEqual(namedArgs, want) {
		t.Fatalf("named args = %v, want %v", namedArgs, want)
	}
	if err := app.run([]string{"pattern", "path"}); err != nil {
		t.Fatal(err)
	}
	if want := []string{"pattern", "path"}; !reflect.DeepEqual(defaultArgs, want) {
		t.Fatalf("default args = %v, want %v", defaultArgs, want)
	}
}

func TestNewApplicationRegistersCommandSurface(t *testing.T) {
	app := newApplication()
	for _, name := range []string{"search", "version", "write", "graph", "anchors", "boundaries", "examples", "artifacts", "context", "languages", "get", "tree", "repos", "refs", "ask", "ai-provider", "login", "logout", "rules", "grit", "extract", "architecture", "sources"} {
		if _, ok := app.commands[name]; !ok {
			t.Errorf("command %q is not registered", name)
		}
	}
	if app.defaultCommand == nil {
		t.Fatal("default command is nil")
	}
}
