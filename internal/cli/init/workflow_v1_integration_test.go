package initcommand

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/cli/overview"
	searchcommand "github.com/greppleai/grepple/internal/cli/search"
	treecommand "github.com/greppleai/grepple/internal/cli/tree"
	"github.com/greppleai/grepple/internal/cli/verify"
	"github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/directorymeta"
)

func TestV1WorkflowFromInitThroughLocalCommandsAndMetadataEdits(t *testing.T) {
	root, app, initOutput := initTestContext(t)
	writeInitSource(t, root, "src/engine.go", "package src\n// selected\nfunc Engine() {}\n")
	writeInitSource(t, root, "src/view.ts", "// selected\nexport function View() { return 1 }\n")
	writeInitSource(t, root, "config/settings.json", "{\"selected\":true}\n")
	writeInitSource(t, root, ".gitignore", "src/excluded.go\n")
	writeInitSource(t, root, "src/excluded.go", "package src\n// selected\nfunc Excluded() {}\n")
	ctx := context.Background()
	plan, err := planGenerationV1(ctx, app, nil, false, "")
	if err != nil {
		t.Fatal(err)
	}
	err = runGenerationV1(ctx, app, root, plan, 1, func(_ context.Context, job generationJobV1) (directorymeta.DirectoryV1, error) {
		result := generatedV1(job)
		for i := range result.Files {
			file := &result.Files[i]
			switch file.Name {
			case "engine.go":
				file.Purpose = "Implementation"
				file.DomainAreas = []string{"Payments"}
				file.Keywords = []string{"c++"}
			case "view.ts":
				file.Purpose = "Testing"
				file.DomainAreas = []string{"Payments"}
				file.Keywords = []string{"c++"}
			case "settings.json":
				file.Purpose = "Documentation"
				file.DomainAreas = []string{"Configuration"}
			case ".gitignore":
				file.Purpose = "Documentation"
				file.DomainAreas = []string{"Configuration"}
			default:
				return directorymeta.DirectoryV1{}, fmt.Errorf("unselected file in generation: %s", file.Name)
			}
		}
		return result, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(initOutput.String(), "write .grepple/grepple.yaml") {
		t.Fatalf("init output: %s", initOutput.String())
	}
	yaml, err := os.ReadFile(filepath.Join(root, ".grepple", "grepple.yaml"))
	if err != nil || !bytes.Contains(yaml, []byte("schema: v1")) || !bytes.Contains(yaml, []byte("directory:")) || bytes.Contains(yaml, []byte("directories:")) || bytes.Contains(yaml, []byte("excluded.go")) {
		t.Fatalf("persisted YAML: %s (%v)", yaml, err)
	}
	run := func(fn func(cliruntime.Context) error) string {
		t.Helper()
		var out, errs bytes.Buffer
		runtime := cliruntime.NewContext(cliruntime.ContextOptions{Output: &out, ErrorOutput: &errs})
		if err := fn(runtime); err != nil {
			t.Fatalf("command: %v; stderr: %s", err, errs.String())
		}
		return out.String()
	}
	valid := run(func(c cliruntime.Context) error { return verify.Execute(c, &verify.Args{JSON: true}) })
	if !strings.Contains(valid, `"valid":`) || strings.Contains(valid, `"stale":`) {
		t.Fatalf("verify JSON: %s", valid)
	}
	if text := run(func(c cliruntime.Context) error { return verify.Execute(c, &verify.Args{}) }); !strings.Contains(text, "stale=0 invalid=0") {
		t.Fatalf("verify text: %s", text)
	}
	report, err := overview.Build(app)
	if err != nil || report.Files.Current != 4 || report.Files.Selected != 4 || report.DomainAreas.Total != 2 {
		t.Fatalf("overview: %+v %v", report, err)
	}
	if text := run(func(c cliruntime.Context) error { return overview.Execute(c, &overview.Args{}) }); !strings.Contains(text, "payments") {
		t.Fatalf("overview text: %s", text)
	}
	if out := run(func(c cliruntime.Context) error { return overview.Execute(c, &overview.Args{JSON: true}) }); !json.Valid([]byte(out)) || !strings.Contains(out, `"current": 4`) {
		t.Fatalf("overview JSON: %s", out)
	}
	selectedTree := func(jsonMode bool) string {
		return run(func(c cliruntime.Context) error {
			return treecommand.Execute(c, &treecommand.Request{Depth: 3, JSON: jsonMode, Purposes: []string{"Implementation"}, DomainAreas: []string{"Payments"}, Keywords: []string{"c++"}})
		})
	}
	for _, jsonMode := range []bool{false, true} {
		out := selectedTree(jsonMode)
		if jsonMode && !json.Valid([]byte(out)) {
			t.Fatalf("invalid tree JSON: %s", out)
		}
		if !strings.Contains(out, "engine.go") || strings.Contains(out, "view.ts") || strings.Contains(out, "settings.json") || strings.Contains(out, "excluded.go") {
			t.Fatalf("tree json=%v: %s", jsonMode, out)
		}
	}
	filtered := []string{"--purpose", "Implementation", "--domain-area", "Payments", "--keyword", "c++"}
	search := func(args ...string) string {
		return run(func(c cliruntime.Context) error {
			return searchcommand.New(c).Run(append(append([]string(nil), filtered...), args...))
		})
	}
	for _, args := range [][]string{
		{"--line-only", "selected", "src"}, {"--json", "selected", "src"},
		{"--files", "src"}, {"--files", "--json", "src"},
		{"--outline", "src"}, {"--outline", "--json", "src"},
		{"--at", "src/engine.go:3"},
	} {
		out := search(args...)
		if !strings.Contains(out, "engine.go") && !strings.Contains(out, "Engine") || strings.Contains(out, "view.ts") || strings.Contains(out, "View") || strings.Contains(out, "excluded.go") {
			t.Fatalf("filtered search %q: %s", args, out)
		}
	}
	baseline := run(func(c cliruntime.Context) error { return searchcommand.New(c).Run([]string{"--files", "src"}) })
	narrowed := search("--files", "src")
	if !strings.Contains(baseline, "engine.go") || !strings.Contains(baseline, "view.ts") || !strings.Contains(baseline, "excluded.go") || len(narrowed) >= len(baseline) {
		t.Fatalf("expected bounded output vs selected sources: baseline=%q narrowed=%q", baseline, narrowed)
	}
	t.Logf("search files in explicit src directory: 3 -> 1 (explicit path bypasses ignore; metadata still excludes it); --files output: %d -> %d bytes", len(baseline), len(narrowed))
	writeInitSource(t, root, "src/engine.go", "package src\n// selected but changed\nfunc Engine() {}\n")
	report, err = overview.Build(app)
	if err != nil || report.Files.Stale != 1 || report.Files.Current != 3 {
		t.Fatalf("freshness: %+v %v", report, err)
	}
	if out := search("--files", "src"); strings.Contains(out, "engine.go") || strings.Contains(out, "view.ts") || strings.Contains(out, "excluded.go") {
		t.Fatalf("stale selected file escaped metadata scope: %s", out)
	}
	if out := selectedTree(true); !json.Valid([]byte(out)) || strings.Contains(out, `"path": "src/engine.go"`) || strings.Contains(out, `"path": "src/view.ts"`) {
		t.Fatalf("stale tree widened: %s", out)
	}
	stale, err := verify.Build(app, nil)
	if err != nil || len(stale.Stale) == 0 {
		t.Fatalf("stale verification: %+v %v", stale, err)
	}
	document, err := directorymeta.ReadDocumentV1(root)
	if err != nil {
		t.Fatal(err)
	}
	src := document.Directory["src"]
	for i := range src.Files {
		if src.Files[i].Name == "view.ts" {
			src.Files[i].Purpose = "Implementation"
		}
	}
	if err := directorymeta.WriteDirectoryV1(root, filepath.Join(root, "src"), src); err != nil {
		t.Fatal(err)
	}
	for _, jsonMode := range []bool{false, true} {
		out := selectedTree(jsonMode)
		if !strings.Contains(out, "view.ts") || strings.Contains(out, "src/engine.go") {
			t.Fatalf("metadata edit not reflected in tree json=%v: %s", jsonMode, out)
		}
	}
	if out := search("--files", "src"); !strings.Contains(out, "view.ts") || strings.Contains(out, "engine.go") {
		t.Fatalf("metadata edit not reflected in search: %s", out)
	}
	report, err = overview.Build(app)
	if err != nil || report.Files.Current != 3 || report.Files.Stale != 1 {
		t.Fatalf("post-edit overview: %+v %v", report, err)
	}
}
