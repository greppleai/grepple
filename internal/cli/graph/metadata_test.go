package graph

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/directorymeta"
	"github.com/greppleai/grepple/internal/parser"
	"github.com/greppleai/grepple/internal/search"
)

func graphMetadataFixture(t *testing.T) string {
	t.Helper()
	root := chdirTemp(t)
	contents := map[string]string{
		"a.go":       "package sample\nfunc Root(){ Work() }\n",
		"b.go":       "package sample\nfunc Work() {}\n",
		"stale.go":   "package sample\nfunc Stale() {}\n",
		"missing.go": "package sample\nfunc Missing() {}\n",
	}
	entries := make([]directorymeta.FileV1, 0, 3)
	for _, name := range []string{"a.go", "b.go", "stale.go", "missing.go"} {
		writeGraphSource(t, root, name, contents[name])
		if name == "missing.go" {
			continue
		}
		sum := sha256.Sum256([]byte(contents[name]))
		checksum := hex.EncodeToString(sum[:])
		if name == "stale.go" {
			checksum = strings.Repeat("0", 64)
		}
		purpose, domain := "implementation", "payments"
		if name == "b.go" {
			purpose, domain = "testing", "billing"
		}
		entries = append(entries, directorymeta.FileV1{Name: name, Checksum: checksum, Description: "Graph fixture.", Purpose: purpose, DomainAreas: []string{domain}, FunctionalAreas: []string{"api"}})
	}
	if err := directorymeta.WriteDirectoryV1(root, root, directorymeta.DirectoryV1{Files: entries}); err != nil {
		t.Fatal(err)
	}
	return root
}

func graphMetadataJSON(t *testing.T, args ...string) navigationGraphOutput {
	t.Helper()
	var text bytes.Buffer
	if err := New(cliruntime.Environment{Output: &text}).Run(args); err != nil {
		t.Fatalf("graph %v: %v", args, err)
	}
	var result navigationGraphOutput
	if err := json.Unmarshal(text.Bytes(), &result); err != nil {
		t.Fatalf("decode %s: %v", text.String(), err)
	}
	return result
}

func TestGraphMetadataQueryFiltersBeforeRootsAndMaxFiles(t *testing.T) {
	root := graphMetadataFixture(t)
	for _, direction := range []string{"callers", "callees"} {
		result := graphMetadataJSON(t, direction, "--symbol", "Root", "--purpose", "testing", "--purpose", "implementation", "--domain-area", "payments", "--functional-area", "API", "--max-files", "1", "--json", ".")
		if names := graphOutputDeclarationNames(result); !reflect.DeepEqual(names, []string{"Root"}) || result.Sources.Parsed != 1 || result.Sources.Discovered != 1 || result.Truncation != nil {
			t.Fatalf("%s selected graph: names=%v sources=%+v truncation=%+v", direction, graphOutputDeclarationNames(result), result.Sources, result.Truncation)
		}
		at := graphMetadataJSON(t, direction, "--at", "a.go:2", "--purpose", "implementation", "--domain-area", "payments", "--functional-area", "api", "--json", ".")
		if names := graphOutputDeclarationNames(at); !reflect.DeepEqual(names, []string{"Root"}) {
			t.Fatalf("%s --at selected names=%v", direction, names)
		}
		absolute := filepath.Join(root, "a.go")
		explicit := graphMetadataJSON(t, direction, "--at", absolute+":2", "--purpose", "implementation", "--domain-area", "payments", "--functional-area", "api", "--json", absolute, filepath.Join(root, "b.go"))
		if names := graphOutputDeclarationNames(explicit); !reflect.DeepEqual(names, []string{"Root"}) || explicit.Sources.Parsed != 1 || explicit.Sources.Discovered != 1 {
			t.Fatalf("%s absolute --at and explicit path intersection: names=%v sources=%+v", direction, names, explicit.Sources)
		}
		both := graphMetadataJSON(t, direction, "--symbol", "Root", "--purpose", "implementation", "--purpose", "testing", "--max-files", "1", "--json", ".")
		if both.Truncation == nil || both.Sources.Discovered <= both.Sources.Parsed {
			t.Fatalf("%s max-files did not limit after metadata selection: %+v %+v", direction, both.Sources, both.Truncation)
		}
		billing := graphMetadataJSON(t, direction, "--symbol", "Work", "--purpose", "testing", "--domain-area", "billing", "--max-files", "1", "--json", ".")
		if names := graphOutputDeclarationNames(billing); !reflect.DeepEqual(names, []string{"Work"}) || billing.Sources.Parsed != 1 {
			t.Fatalf("%s billing scope: names=%v sources=%+v", direction, names, billing.Sources)
		}
		unfiltered := graphMetadataJSON(t, direction, "--symbol", "Root", "--json", ".")
		if unfiltered.Sources.Discovered <= result.Sources.Discovered {
			t.Fatalf("unfiltered sources=%+v filtered=%+v", unfiltered.Sources, result.Sources)
		}
	}
	for _, args := range [][]string{
		{"callees", "--symbol", "Root", "--purpose", "testing", "--domain-area", "payments", "--json"},
		{"callers", "--at", "b.go:2", "--purpose", "implementation", "--json"},
		{"callees", "--symbol", "Stale", "--purpose", "implementation", "--json"},
		{"callees", "--symbol", "Missing", "--purpose", "implementation", "--json"},
	} {
		if err := New(cliruntime.Environment{}).Run(args); err == nil {
			t.Fatalf("out-of-scope graph %v succeeded", args)
		}
	}
}

func TestGraphMetadataResolveExcludesNonCurrentAndMissing(t *testing.T) {
	graphMetadataFixture(t)
	for _, test := range []struct {
		symbol string
		want   int
	}{{"Root", 1}, {"Work", 0}, {"Stale", 0}, {"Missing", 0}} {
		var output bytes.Buffer
		exit := 0
		err := New(cliruntime.Environment{Output: &output, Exit: func(code int) { exit = code }}).Run([]string{"resolve", "--symbol", test.symbol, "--purpose", "implementation", "--domain-area", "payments", "--functional-area", "api", "--json"})
		if err != nil {
			t.Fatal(err)
		}
		var resolved ResolveOutput
		if err := json.Unmarshal(output.Bytes(), &resolved); err != nil {
			t.Fatal(err)
		}
		if len(resolved.Matches) != test.want || resolved.Sources.Parsed != 1 || (exit == 1) != (test.want == 0) {
			t.Fatalf("%s matches=%+v sources=%+v exit=%d", test.symbol, resolved.Matches, resolved.Sources, exit)
		}
	}
	var output bytes.Buffer
	exit := 0
	err := New(cliruntime.Environment{Output: &output, Exit: func(code int) { exit = code }}).Run([]string{"resolve", "--symbol", "Root", "--purpose", "testing", "--domain-area", "payments", "--json"})
	if err != nil || exit != 1 {
		t.Fatalf("empty selected universe err=%v exit=%d output=%s", err, exit, output.String())
	}
}

func TestGraphMetadataRejectsInvalidAndRemoteBeforeRequests(t *testing.T) {
	for _, args := range [][]string{
		{"callers", "--symbol", "Root", "--purpose", "", "--repo", "example/repo"},
		{"callees", "--symbol", "Root", "--purpose", "implementation", "--repo", "example/repo", "--server", "http://127.0.0.1:1"},
		{"callers", "--symbol", "Root", "--purpose", "implementation", "--server", "http://127.0.0.1:1"},
		{"resolve", "--symbol", "Root", "--functional-area", "\n"},
	} {
		err := New(cliruntime.Environment{}).Run(args)
		if err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}

func TestGraphMetadataSelectionScopeAndFreshness(t *testing.T) {
	root := graphMetadataFixture(t)
	selector, err := compileGraphMetadataSelector([]string{"implementation"}, []string{"payments"}, []string{"api"})
	if err != nil {
		t.Fatal(err)
	}
	paths, err := resolveCLIPathsSelected(cliruntime.Environment{}, []string{"."}, selector)
	if err != nil || len(paths) != 1 || filepath.Base(paths[0]) != "a.go" {
		t.Fatalf("initial selected=%v err=%v", paths, err)
	}
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("package sample\nfunc Changed() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	paths, err = resolveCLIPathsSelected(cliruntime.Environment{}, []string{"."}, selector)
	if err != nil || len(paths) != 0 {
		t.Fatalf("stale path selected=%v err=%v", paths, err)
	}
	outside := writeGraphSource(t, t.TempDir(), "outside.go", "package sample\nfunc Outside() {}\n")
	if _, err := resolveCLIPathsSelected(cliruntime.Environment{}, []string{outside}, selector); err == nil || !strings.Contains(err.Error(), "outside local metadata root") {
		t.Fatalf("outside path error=%v", err)
	}
	link := filepath.Join(root, "escaped.go")
	if err := os.Symlink(outside, link); err == nil {
		if _, err := resolveCLIPathsSelected(cliruntime.Environment{}, []string{link}, selector); err == nil || !strings.Contains(err.Error(), "outside local metadata root") {
			t.Fatalf("symlink escaped metadata root: %v", err)
		}
	}
}

func TestGraphMetadataDoesNotBypassRepositoryIgnore(t *testing.T) {
	root := graphMetadataFixture(t)
	writeGraphSource(t, root, ".gitignore", "a.go\n")
	selector, err := compileGraphMetadataSelector([]string{"implementation"}, []string{"payments"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	paths, err := resolveCLIPathsSelected(cliruntime.Environment{}, []string{"."}, selector)
	if err != nil || len(paths) != 0 {
		t.Fatalf("ignored source selected=%v err=%v", paths, err)
	}
}

func TestGraphMetadataMissingAndInvalidSnapshotNeverSelectsSources(t *testing.T) {
	root := chdirTemp(t)
	writeGraphSource(t, root, "a.go", "package sample\nfunc Root() {}\n")
	selector, err := compileGraphMetadataSelector([]string{"implementation"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []bool{false, true} {
		if invalid {
			writeGraphSource(t, root, ".grepple/grepple.yaml", "not: a valid v1 document\n")
		}
		paths, err := resolveCLIPathsSelected(cliruntime.Environment{}, []string{"a.go"}, selector)
		if err != nil || len(paths) != 0 || paths == nil {
			t.Fatalf("invalid=%v selected=%v err=%v", invalid, paths, err)
		}
		if err := New(cliruntime.Environment{}).Run([]string{"callees", "--symbol", "Root", "--purpose", "implementation", "a.go"}); err == nil || !strings.Contains(err.Error(), "grepple verify") {
			t.Fatalf("invalid=%v no-match error=%v", invalid, err)
		}
	}
}

func TestGraphMetadataDaemonCacheRespectsChangedAnnotations(t *testing.T) {
	root := graphMetadataFixture(t)
	for run := 0; run < 2; run++ {
		var output bytes.Buffer
		values := &QueryArgs{Symbol: "Root", Depth: 1, JSON: true, Purposes: []string{"implementation"}, DomainAreas: []string{"payments"}, Paths: []string{"."}}
		if err := executeQueryWithDaemon(cliruntime.Environment{Output: &output}, "callees", values, true); err != nil {
			t.Fatal(err)
		}
		var result navigationGraphOutput
		if err := json.Unmarshal(output.Bytes(), &result); err != nil || result.Sources.Parsed != 1 {
			t.Fatalf("run=%d graph=%+v err=%v", run, result.Sources, err)
		}
	}
	document, err := directorymeta.ReadDocumentV1(root)
	if err != nil {
		t.Fatal(err)
	}
	for index := range document.Directory["."].Files {
		// FileV1 is a value; update the slice in the document's map entry.
		entry := document.Directory["."]
		if entry.Files[index].Name == "a.go" {
			entry.Files[index].Purpose = "testing"
			document.Directory["."] = entry
		}
	}
	if err := directorymeta.WriteDocumentV1(root, document); err != nil {
		t.Fatal(err)
	}
	if err := executeQueryWithDaemon(cliruntime.Environment{}, "callees", &QueryArgs{Symbol: "Root", Depth: 1, Purposes: []string{"implementation"}, DomainAreas: []string{"payments"}, Paths: []string{"."}}, true); err == nil {
		t.Fatal("stale daemon membership leaked after annotation change")
	}
	var output bytes.Buffer
	if err := executeQueryWithDaemon(cliruntime.Environment{Output: &output}, "callees", &QueryArgs{Symbol: "Root", Depth: 1, JSON: true, Purposes: []string{"testing"}, DomainAreas: []string{"payments"}, Paths: []string{"."}}, true); err != nil {
		t.Fatal(err)
	}
	var result navigationGraphOutput
	if err := json.Unmarshal(output.Bytes(), &result); err != nil || !reflect.DeepEqual(graphOutputDeclarationNames(result), []string{"Root"}) {
		t.Fatalf("newly selected graph=%v err=%v", graphOutputDeclarationNames(result), err)
	}
}

// Shell-split generated text without executing Grepple. Values are fixture-controlled.
func graphReplayArgs(t *testing.T, command string) []string {
	t.Helper()
	output, err := exec.Command("sh", "-c", "set -- "+command+"; printf '%s\\n' \"$@\"").Output()
	if err != nil {
		t.Fatalf("split %q: %v", command, err)
	}
	parts := strings.Split(strings.TrimSuffix(string(output), "\n"), "\n")
	if len(parts) < 4 || parts[0] != "grepple" || parts[1] != "graph" {
		t.Fatalf("unexpected replay args: %v", parts)
	}
	return parts[2:]
}

func TestGraphMetadataFollowupsReplayQuotedSelectedScope(t *testing.T) {
	root := chdirTemp(t)
	dir := "one's files"
	contents := map[string]string{"a go.go": "package sample\nfunc Root(){ Work() }\n", "b.go": "package sample\nfunc Work() {}\n", "other.go": "package sample\nfunc Other() {}\n"}
	entries := make([]directorymeta.FileV1, 0, len(contents))
	for _, name := range []string{"a go.go", "b.go", "other.go"} {
		content := contents[name]
		writeGraphSource(t, root, filepath.Join(dir, name), content)
		sum := sha256.Sum256([]byte(content))
		purpose, domain := "Implementation Work", "Feature's Team"
		if name == "b.go" {
			purpose = "Testing Work"
		}
		if name == "other.go" {
			domain = "Other Team"
		}
		entries = append(entries, directorymeta.FileV1{Name: name, Checksum: hex.EncodeToString(sum[:]), Description: "Replay fixture.", Purpose: purpose, DomainAreas: []string{domain}, FunctionalAreas: []string{"api beta"}})
	}
	if err := directorymeta.WriteDirectoryV1(root, filepath.Join(root, dir), directorymeta.DirectoryV1{Files: entries}); err != nil {
		t.Fatal(err)
	}
	flags := []string{"--purpose", "Implementation Work", "--purpose", "Testing Work", "--domain-area", "Feature's Team", "--functional-area", "api beta"}
	queryArgs := append([]string{"callees", "--symbol", "Root", "--max-files", "1", "--json"}, flags...)
	queryArgs = append(queryArgs, dir)
	limited := graphMetadataJSON(t, queryArgs...)
	if limited.Metadata == nil || limited.Metadata.NextCommand == "" || limited.Truncation == nil || limited.Sources.Parsed != 1 {
		t.Fatalf("limited query: %+v", limited)
	}
	replayed := graphMetadataJSON(t, graphReplayArgs(t, limited.Metadata.NextCommand)...)
	if replayed.Truncation != nil || replayed.Sources.Parsed != 2 || replayed.Sources.Discovered != 2 || len(replayed.Metadata.Diagnostics) == 0 {
		t.Fatalf("replayed query: %+v", replayed)
	}
	var text bytes.Buffer
	resolveArgs := append([]string{"resolve", "--symbol", "Root", "--max-files", "1", "--json"}, flags...)
	resolveArgs = append(resolveArgs, dir)
	if err := New(cliruntime.Environment{Output: &text}).Run(resolveArgs); err != nil {
		t.Fatal(err)
	}
	var resolved ResolveOutput
	if err := json.Unmarshal(text.Bytes(), &resolved); err != nil || resolved.Metadata == nil || resolved.Metadata.NextCommand == "" || len(resolved.Matches) != 1 {
		t.Fatalf("limited resolve: %+v: %v", resolved, err)
	}
	text.Reset()
	if err := New(cliruntime.Environment{Output: &text}).Run(graphReplayArgs(t, resolved.Metadata.NextCommand)); err != nil {
		t.Fatal(err)
	}
	var full ResolveOutput
	if err := json.Unmarshal(text.Bytes(), &full); err != nil || full.Sources.Parsed != 2 || full.Sources.Discovered != 2 || len(full.Matches) != 1 {
		t.Fatalf("replayed resolve: %+v: %v", full, err)
	}
	for _, hint := range []string{full.Matches[0].CallersCommand, full.Matches[0].CalleesCommand} {
		hintArgs := graphReplayArgs(t, hint)
		follow := graphMetadataJSON(t, append([]string{hintArgs[0], "--json"}, hintArgs[1:]...)...)
		if follow.Sources.Parsed != 2 || follow.Sources.Discovered != 2 || len(follow.Metadata.Diagnostics) == 0 {
			t.Fatalf("scoped hint %q: %+v", hint, follow)
		}
	}
}

func TestGraphMetadataNoMatchDiagnosticDoesNotClaimUnfilteredCompleteness(t *testing.T) {
	graphMetadataFixture(t)
	var text bytes.Buffer
	exit := 0
	args := []string{"resolve", "--symbol", "Missing", "--purpose", "implementation", "--json"}
	if err := New(cliruntime.Environment{Output: &text, Exit: func(code int) { exit = code }}).Run(args); err != nil || exit != 1 {
		t.Fatalf("resolve err=%v exit=%d", err, exit)
	}
	var resolved ResolveOutput
	if err := json.Unmarshal(text.Bytes(), &resolved); err != nil || len(resolved.Metadata.Diagnostics) != 2 || resolved.Metadata.Diagnostics[1].Code != "metadata-no-matches" || resolved.Sources.Parsed != 1 {
		t.Fatalf("filtered resolve=%+v err=%v", resolved, err)
	}
	text.Reset()
	if err := New(cliruntime.Environment{Output: &text, Exit: func(int) {}}).Run([]string{"resolve", "--symbol", "Missing", "--purpose", "implementation"}); err != nil || !strings.Contains(text.String(), "grepple verify") {
		t.Fatalf("human no-match=%q err=%v", text.String(), err)
	}
	unfiltered := graphMetadataJSON(t, "callees", "--symbol", "Root", "--json")
	if len(unfiltered.Metadata.Diagnostics) != 0 {
		t.Fatalf("unfiltered diagnostics changed: %+v", unfiltered.Metadata.Diagnostics)
	}
}

func TestGraphMetadataFollowupsRetainRepositoryScopeFlags(t *testing.T) {
	flags := func(parts []string) []string { return append(parts, "--production-only", "--no-config-ignore") }
	context := cliruntime.Environment{RepositoryContext: cliruntime.RepositoryServices{AppendScopeFlagsFunc: flags}}
	query := graphQueryContinuationCommand(context, search.NavigationQueryCallees, QueryArgs{Symbol: "Root", Depth: 2, Purposes: []string{"Implementation Work"}, Paths: []string{"a path"}}, &Truncation{})
	for _, command := range []string{query, resolveMetadata(ResolveArgs{Symbol: "Root", Purposes: []string{"Implementation Work"}, Paths: []string{"a path"}}, Output{Truncation: &Truncation{}}, 1, flags).NextCommand, resolveMatches([]parser.NavigationDeclaration{{Name: "Root", Path: "a path/a.go", Start: 2}}, "Root", []string{"a path"}, []string{"Implementation Work"}, nil, nil, flags)[0].CallersCommand} {
		parsed := graphReplayArgs(t, command)
		joined := strings.Join(parsed, "|")
		for _, expected := range []string{"--production-only", "--no-config-ignore", "--purpose|Implementation Work", "a path"} {
			if !strings.Contains(joined, expected) {
				t.Fatalf("follow-up lost %q: %v", expected, parsed)
			}
		}
	}
}
