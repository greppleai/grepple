package search

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/parser"
)

func TestOutlineKindArgsValidation(t *testing.T) {
	options, _, _, err := parseTestSearchArgs([]string{"--outline", "--kind", "types,functions", "--kind", "variables", "file.go"})
	if err != nil || len(options.Kinds) != 3 || options.Params.Globs[0] != "file.go" {
		t.Fatalf("options=%+v err=%v", options, err)
	}
	for _, args := range [][]string{
		{"--kind", "types", "file.go"},
		{"--outline", "--kind", "invalid", "file.go"},
		{"--outline", "--kind", "", "file.go"},
	} {
		if _, _, _, err := parseTestSearchArgs(args); err == nil {
			t.Fatalf("expected invalid --kind for %v", args)
		}
	}
}

func TestOutlineKindsFilterHumanAndJSON(t *testing.T) {
	dir := chdirTemp(t)
	src := "package p\n\ntype Widget interface { Run() }\nvar Count int\nconst Limit = 5\nfunc Build() {}\n"
	if err := os.WriteFile(filepath.Join(dir, "widget.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ kind, want, excluded string }{
		{"types", "interface\tWidget", "method\tRun"},
		{"functions", "method\tRun", "interface\tWidget"},
		{"variables", "var\tCount", "func\tBuild"},
	} {
		t.Run(test.kind, func(t *testing.T) {
			out := captureStdout(t, func() {
				if err := runTestSearch([]string{"--outline", "--kind", test.kind, "widget.go"}); err != nil {
					t.Fatal(err)
				}
			})
			if !strings.Contains(out, test.want) || strings.Contains(out, test.excluded) || strings.Contains(out, "package p") {
				t.Fatalf("filtered %s outline=%q", test.kind, out)
			}
		})
	}
	out := captureStdout(t, func() {
		if err := runTestSearch([]string{"--outline", "--kind", "types,functions", "--json", "widget.go"}); err != nil {
			t.Fatal(err)
		}
	})
	var payload struct {
		Files []parser.FileOutline `json:"files"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil || len(payload.Files) != 1 {
		t.Fatalf("invalid JSON %q: %v", out, err)
	}
	if len(payload.Files[0].Symbols) != 2 || payload.Files[0].Symbols[0].Kind != "interface" || len(payload.Files[0].Symbols[0].Children) != 1 || payload.Files[0].Symbols[1].Kind != "func" {
		t.Fatalf("filtered JSON=%#v", payload)
	}
}
