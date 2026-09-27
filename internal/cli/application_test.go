package cli

import "testing"

func TestApplicationParserSelectsExplicitAndDefaultSearch(t *testing.T) {
	explicit, help, err := parseApplicationArgs([]string{"search", "needle", "path"})
	if err != nil || help {
		t.Fatalf("explicit parse: help=%v err=%v", help, err)
	}
	implicit, help, err := parseApplicationArgs([]string{"needle", "path"})
	if err != nil || help {
		t.Fatalf("implicit parse: help=%v err=%v", help, err)
	}
	for name, values := range map[string]*Arguments{"explicit": explicit, "implicit": implicit} {
		if values.Search == nil || values.Search.Query != "needle" || len(values.Search.Globs) != 1 || values.Search.Globs[0] != "path" {
			t.Fatalf("%s search arguments = %#v", name, values.Search)
		}
	}
}

func TestApplicationParserOwnsGlobalAndNestedArguments(t *testing.T) {
	values, help, err := parseApplicationArgs([]string{"--production-only", "--no-spill", "graph", "callers", "--symbol", "Run", "."})
	if err != nil || help {
		t.Fatalf("parse: help=%v err=%v", help, err)
	}
	if !values.ProductionOnly || !values.NoSpill || values.Graph == nil || values.Graph.Callers == nil {
		t.Fatalf("parsed arguments = %#v", values)
	}
	query := values.Graph.Callers
	if query.JSON || query.Symbol != "Run" || len(query.Paths) != 1 || query.Paths[0] != "." {
		t.Fatalf("graph callers = %#v", query)
	}
}
func TestApplicationParserRequiresFocusedGraphSubcommand(t *testing.T) {
	if _, _, err := parseApplicationArgs([]string{"graph", "."}); err == nil {
		t.Fatal("graph without a focused subcommand unexpectedly parsed")
	}

	gritValues, help, err := parseApplicationArgs([]string{"grit", "language go `func $name() {}`"})
	if err != nil || help {
		t.Fatalf("grit parse: help=%v err=%v", help, err)
	}
	if gritValues.Grit == nil || gritValues.Grit.Run == nil || gritValues.Grit.Run.Limit != 20 || gritValues.Grit.Run.MaxOutputBytes != 16*1024 {
		t.Fatalf("grit run arguments = %#v", gritValues.Grit)
	}
}

func TestApplicationParserHookFlags(t *testing.T) {
	values, help, err := parseApplicationArgs([]string{"hook", "--id", "go-empty-if", "--id", "go-direct-dot-import", "--all", "--json"})
	if err != nil || help || values.Hook == nil || !values.Hook.All || !values.Hook.JSON || len(values.Hook.IDs) != 2 || values.Hook.IDs[0] != "go-empty-if" || values.Hook.IDs[1] != "go-direct-dot-import" {
		t.Fatalf("hook parse: values=%+v help=%v err=%v", values, help, err)
	}
}

func TestApplicationParserAcceptsGlobalsAroundNestedSubcommands(t *testing.T) {
	values, help, err := parseApplicationArgs([]string{"graph", "--no-spill", "callers", "--production-only", "--symbol", "Run"})
	if err != nil || help {
		t.Fatalf("parse: help=%v err=%v", help, err)
	}
	if !values.NoSpill || !values.ProductionOnly || values.Graph == nil || values.Graph.Callers == nil || values.Graph.Callers.JSON {
		t.Fatalf("arguments = %#v", values)
	}
}

func TestApplicationParserInitConcurrencyDefaultsAndOverrides(t *testing.T) {
	for _, test := range []struct {
		args []string
		want int
	}{
		{args: []string{"init"}, want: 1},
		{args: []string{"init", "--concurrency", "4", "--only-directory", "internal/cli/init"}, want: 4},
	} {
		values, help, err := parseApplicationArgs(test.args)
		if err != nil || help || values.Init == nil || values.Init.Concurrency != test.want {
			t.Fatalf("parse %v: values=%+v help=%v err=%v", test.args, values, help, err)
		}
	}
}

func TestApplicationParserDaemonGlobalAroundArchitecture(t *testing.T) {
	for _, args := range [][]string{
		{"--daemon", "architecture", "directory", "--json", "."},
		{"architecture", "--daemon", "directory", "--json", "."},
		{"architecture", "directory", "--daemon", "--json", "."},
	} {
		values, help, err := parseApplicationArgs(args)
		if err != nil || help || !values.Daemon || values.Architecture == nil || values.Architecture.Directory == nil {
			t.Fatalf("parse %v: %+v help=%v err=%v", args, values, help, err)
		}
	}
}
