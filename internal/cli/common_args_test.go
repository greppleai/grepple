package cli

import (
	"testing"

	"github.com/alexflint/go-arg"
)

func TestCommonArgsExposeServerAliasesWhenEmbedded(t *testing.T) {
	for _, arguments := range [][]string{{"--server", "https://example.test"}, {"-s", "https://example.test"}} {
		var values struct {
			commonArgs
		}
		parser, err := arg.NewParser(arg.Config{Program: "grepple test"}, &values)
		if err != nil {
			t.Fatal(err)
		}
		if err := parser.Parse(arguments); err != nil {
			t.Fatalf("parse %v: %v", arguments, err)
		}
		if values.Server != "https://example.test" {
			t.Fatalf("server=%q for %v", values.Server, arguments)
		}
	}
}
