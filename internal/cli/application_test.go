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
	values, help, err := parseApplicationArgs([]string{"--production-only", "--no-spill", "graph", "callers", "--compact", "--symbol", "Run", "."})
	if err != nil || help {
		t.Fatalf("parse: help=%v err=%v", help, err)
	}
	if !values.ProductionOnly || !values.NoSpill || values.Graph == nil || values.Graph.Callers == nil {
		t.Fatalf("parsed arguments = %#v", values)
	}
	query := values.Graph.Callers
	if !query.Compact || query.Symbol != "Run" || len(query.Paths) != 1 || query.Paths[0] != "." {
		t.Fatalf("graph callers = %#v", query)
	}
}
func TestApplicationParserNormalizesLegacyDefaultModesAndDefaults(t *testing.T) {
	graphValues, help, err := parseApplicationArgs([]string{"graph", "--compact", "."})
	if err != nil || help {
		t.Fatalf("graph parse: help=%v err=%v", help, err)
	}
	if graphValues.Graph == nil || graphValues.Graph.Build == nil || !graphValues.Graph.Build.Compact || graphValues.Graph.Build.MaxOutputBytes != 16*1024 {
		t.Fatalf("graph build arguments = %#v", graphValues.Graph)
	}

	gritValues, help, err := parseApplicationArgs([]string{"grit", "language go `func $name() {}`"})
	if err != nil || help {
		t.Fatalf("grit parse: help=%v err=%v", help, err)
	}
	if gritValues.Grit == nil || gritValues.Grit.Run == nil || gritValues.Grit.Run.Limit != 20 || gritValues.Grit.Run.MaxOutputBytes != 16*1024 {
		t.Fatalf("grit run arguments = %#v", gritValues.Grit)
	}
}

func TestApplicationParserAcceptsGlobalsAroundNestedSubcommands(t *testing.T) {
	values, help, err := parseApplicationArgs([]string{"graph", "--no-spill", "callers", "--production-only", "--compact", "--symbol", "Run"})
	if err != nil || help {
		t.Fatalf("parse: help=%v err=%v", help, err)
	}
	if !values.NoSpill || !values.ProductionOnly || values.Graph == nil || values.Graph.Callers == nil || !values.Graph.Callers.Compact {
		t.Fatalf("arguments = %#v", values)
	}
}
