package cliruntime

import "testing"

func TestParseRepositoryInvocationOptions(t *testing.T) {
	args, options, err := ParseRepositoryInvocationOptions([]string{"--production-only", "search", "needle", "--no-config-ignore", "--no-repo-config", "--", "--production-only"})
	if err != nil {
		t.Fatal(err)
	}
	if !options.ProductionOnly || !options.NoConfigIgnore || !options.NoRepositoryConfig {
		t.Fatalf("options=%+v", options)
	}
	if len(args) != 4 || args[0] != "search" || args[3] != "--production-only" {
		t.Fatalf("args=%#v", args)
	}
}
