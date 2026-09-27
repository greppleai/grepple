package grit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/greppleai/grepple/internal/wire"
)

func TestParseGritArgsKeepsStructuralFlagsSeparate(t *testing.T) {
	options, err := parseGritArgs([]string{
		"--remote", "--json", "--pattern-id", "calls", "--message", "avoid target",
		"--exclude-glob", "vendor/**", "--max-files", "12", "--max-ast-steps", "345",
		"--skip", "2", "--limit", "5", "--repo", "acme/*", "--exclude-repo", "acme/legacy",
		"language go\n`target($x)`", "src", "**/*.go",
	})
	if err != nil {
		t.Fatal(err)
	}
	if options.Query != "language go\n`target($x)`" || !options.JSON || options.PatternID != "calls" || options.Message != "avoid target" {
		t.Fatalf("options=%#v", options)
	}
	if options.MaxFiles != 12 || options.MaxASTSteps != 345 || options.Skip != 2 || options.Limit != 5 {
		t.Fatalf("limits=%#v", options)
	}
	if len(options.Globs) != 2 || len(options.ExcludeGlobs) != 1 || len(options.Repositories) != 1 || len(options.ExcludeRepositories) != 1 {
		t.Fatalf("scopes=%#v", options)
	}
}

func TestGritScanOptionsDisableImplicitLocalTimeouts(t *testing.T) {
	defaults := gritScanOptions(gritArgs{}).EvaluateOptions
	if !defaults.DisableFileTimeout || !defaults.DisableBatchTimeout {
		t.Fatalf("default local timeouts remain enabled: %#v", defaults)
	}
	explicit := gritScanOptions(gritArgs{MaxFileMilliseconds: 25, TimeoutMilliseconds: 50}).EvaluateOptions
	if explicit.DisableFileTimeout || explicit.DisableBatchTimeout || explicit.MaxElapsed != 25*time.Millisecond || explicit.MaxBatchElapsed != 50*time.Millisecond {
		t.Fatalf("explicit local timeouts not preserved: %#v", explicit)
	}
}

func TestParseGritArgsRejectsQueryAndQueryFile(t *testing.T) {
	if _, err := parseGritArgs([]string{"--query-file", "rule.grit", "language go\n`x`"}); err == nil || !strings.Contains(err.Error(), "query file") {
		t.Fatalf("conflict error=%v", err)
	}
}

func TestLoadGritQueryFileIsBounded(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rule.grit")
	if err := os.WriteFile(path, []byte("language go\n`target($x)`"), 0o644); err != nil {
		t.Fatal(err)
	}
	query, err := loadGritQuery(gritArgs{QueryFile: path})
	if err != nil {
		t.Fatal(err)
	}
	if query != "language go\n`target($x)`" {
		t.Fatalf("query=%q", query)
	}

	large := filepath.Join(t.TempDir(), "large.grit")
	if err := os.WriteFile(large, make([]byte, wire.MaxGritQueryBytes+1), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadGritQuery(gritArgs{QueryFile: large}); err == nil || !strings.Contains(err.Error(), "maximum") {
		t.Fatalf("oversized query error=%v", err)
	}
}

func TestGritExplainReportsWrappersVariablesAndCompatibility(t *testing.T) {
	query := "language go\n`exec.Command($args)` where { $args <: r\"^ctx\" }"
	run := func() string {
		return captureStdout(t, func() {
			if err := newWithDependencies(Dependencies{}).Run([]string{"explain", "--json", query}); err != nil {
				t.Fatal(err)
			}
		})
	}
	first, second := run(), run()
	if first != second {
		t.Fatalf("explain JSON is not deterministic:\n%s\n%s", first, second)
	}
	var output gritExplainOutput
	if err := json.Unmarshal([]byte(first), &output); err != nil {
		t.Fatal(err)
	}
	if !output.OK || output.Schema != gritExplainSchema || output.Language != "go" || output.Compatibility != wire.GritCompatibilityV1 || output.GrammarABI == 0 || output.Grammar == "" {
		t.Fatalf("explain contract=%#v", output)
	}
	if len(output.Interpretations) != 1 || output.Interpretations[0].Context != "expression" || output.Interpretations[0].RootKind != "call_expression" {
		t.Fatalf("wrapper interpretations=%#v", output.Interpretations)
	}
	if len(output.Variables) != 1 || output.Variables[0].Name != "args" || output.Variables[0].Occurrences != 2 || !slices.Contains(output.Variables[0].BindingKinds, "list") || !slices.Contains(output.Variables[0].Roles, "constraint:left") {
		t.Fatalf("variable roles=%#v", output.Variables)
	}
	for _, feature := range []string{"snippet", "regex", "where", "variables"} {
		if !slices.Contains(output.Features, feature) {
			t.Fatalf("features missing %q: %#v", feature, output.Features)
		}
	}
}

func TestGritExplainReportsNodeRootCapture(t *testing.T) {
	output := explainGritQuery("language go\ntype_identifier() as $name where { $name <: r\"^[A-Z]\" }", gritExplainArgs{})
	if !output.OK || !slices.Contains(output.Features, "as") || len(output.Variables) != 1 {
		t.Fatalf("root capture explanation=%#v", output)
	}
	variable := output.Variables[0]
	if variable.Name != "name" || !slices.Contains(variable.BindingKinds, "node") || !slices.Contains(variable.Roles, "expression:as") || !slices.Contains(variable.Roles, "constraint:left") {
		t.Fatalf("root capture roles=%#v", variable)
	}
}

func TestGritExplainReturnsBoundedCompileDiagnostics(t *testing.T) {
	output := explainGritQuery("language go\n`unterminated", gritExplainArgs{})
	if output.OK || len(output.Diagnostics) != 1 || output.Diagnostics[0].Code == "" || output.Diagnostics[0].Severity != "error" {
		t.Fatalf("compile diagnostics=%#v", output)
	}
	human := captureStdout(t, func() {
		if err := newWithDependencies(Dependencies{}).Run([]string{"explain", "--max-output-bytes", "120", "language go\n`target($value)`"}); err != nil {
			t.Fatal(err)
		}
	})
	if len(human) == 0 || len(human) > 120 {
		t.Fatalf("bounded explain output=%q", human)
	}
}

func TestGritExplainHelpIsRecursive(t *testing.T) {
	output := captureStdout(t, func() {
		if err := newWithDependencies(Dependencies{}).Run([]string{"explain", "--help"}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(output, "No source files are read") || !strings.Contains(output, "--json") || !strings.Contains(output, "--max-parse-depth") {
		t.Fatalf("grit explain help is incomplete:\n%s", output)
	}
}

func TestRunGritLocalJSONIsDeterministic(t *testing.T) {
	dir := chdirTemp(t)
	source := "package sample\n\nfunc f() {\n\ttarget(value)\n}\n"
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	options := gritArgs{Query: "language go\n`target($x)`", JSON: true, Limit: DefaultGritResultLimit}

	run := func() string {
		return captureStdout(t, func() {
			if err := runGritLocal(context.Background(), options); err != nil {
				t.Fatal(err)
			}
		})
	}
	first, second := run(), run()
	if first != second {
		t.Fatalf("JSON output changed between runs:\n%s\n%s", first, second)
	}
	var response wire.GritResponse
	if err := json.Unmarshal([]byte(first), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Findings) != 1 || response.Findings[0].Path != "a.go" || response.Findings[0].Text != "target(value)" {
		t.Fatalf("response=%#v", response)
	}
	if response.ResultMetadata == nil || !response.ResultMetadata.Page.Complete || response.ResultMetadata.Page.Returned != 1 || !response.ResultMetadata.Limits.JSONByteUncapped {
		t.Fatalf("result metadata=%#v", response.ResultMetadata)
	}
	finding := response.Findings[0]
	if finding.Range.Start.Line != 4 || finding.Range.Start.Column != 2 || finding.Range.StartByte >= finding.Range.EndByte {
		t.Fatalf("range=%#v", finding.Range)
	}
	if len(finding.Bindings) != 1 || finding.Bindings[0].Name != "x" {
		t.Fatalf("bindings=%#v", finding.Bindings)
	}
}
func TestRunGritLocalTypeScriptUsesUnifiedContract(t *testing.T) {
	dir := chdirTemp(t)
	if err := os.WriteFile(filepath.Join(dir, "app.ts"), []byte("const result = target(value);\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	values := gritArgs{
		Query: "language typescript\n`target($value)`",
		JSON:  true, Limit: DefaultGritResultLimit,
	}
	output := captureStdout(t, func() {
		if err := runGritLocal(context.Background(), values); err != nil {
			t.Fatal(err)
		}
	})
	var response wire.GritResponse
	if err := json.Unmarshal([]byte(output), &response); err != nil {
		t.Fatal(err)
	}
	if response.Metadata.Compatibility != wire.GritCompatibilityV1 || response.Metadata.Language != "typescript" || len(response.Findings) != 1 || response.Findings[0].Text != "target(value)" {
		t.Fatalf("response=%#v", response)
	}
}

func TestRunGritLocalJavaScriptUsesUnifiedContract(t *testing.T) {
	dir := chdirTemp(t)
	if err := os.WriteFile(filepath.Join(dir, "app.jsx"), []byte("const result = target(value);\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	values := gritArgs{Query: "language javascript\n`target($value)`", JSON: true, Limit: DefaultGritResultLimit}
	output := captureStdout(t, func() {
		if err := runGritLocal(context.Background(), values); err != nil {
			t.Fatal(err)
		}
	})
	var response wire.GritResponse
	if err := json.Unmarshal([]byte(output), &response); err != nil {
		t.Fatal(err)
	}
	if response.Metadata.Compatibility != wire.GritCompatibilityV1 || response.Metadata.Language != "javascript" || len(response.Findings) != 1 || response.Findings[0].Text != "target(value)" {
		t.Fatalf("response=%#v", response)
	}
}

func TestRunGritLocalPythonUsesUnifiedContract(t *testing.T) {
	dir := chdirTemp(t)
	if err := os.WriteFile(filepath.Join(dir, "app.py"), []byte("result = target(value)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	values := gritArgs{Query: "language python\n`target($value)`", JSON: true, Limit: DefaultGritResultLimit}
	output := captureStdout(t, func() {
		if err := runGritLocal(context.Background(), values); err != nil {
			t.Fatal(err)
		}
	})
	var response wire.GritResponse
	if err := json.Unmarshal([]byte(output), &response); err != nil {
		t.Fatal(err)
	}
	if response.Metadata.Compatibility != wire.GritCompatibilityV1 || response.Metadata.Language != "python" || len(response.Findings) != 1 || response.Findings[0].Text != "target(value)" {
		t.Fatalf("response=%#v", response)
	}
}
func TestGritCandidatesLeaveAcquisitionToBoundedScanner(t *testing.T) {
	dir := chdirTemp(t)
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package p\nvar x = target(value)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	candidates, err := gritCandidates(context.Background(), dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || candidates[0].Content != nil {
		t.Fatalf("candidates=%#v", candidates)
	}
}

func TestRunGritLocalHumanOutputUsesStructuralRange(t *testing.T) {
	dir := chdirTemp(t)
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package p\nvar x = target(value)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := captureStdout(t, func() {
		if err := runGritLocal(context.Background(), gritArgs{Query: "language go\n`target($x)`", Limit: DefaultGritResultLimit}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, "a.go:2:9-2:22") || !strings.Contains(out, "target(value)") || !strings.Contains(out, "$x") {
		t.Fatalf("human output=%q", out)
	}
}

func TestRunDispatchesGritWithoutChangingSearchParsing(t *testing.T) {
	if err := newWithDependencies(Dependencies{}).Run([]string{"language go\n`=>`"}); err == nil {
		t.Fatal("invalid structural query was dispatched as legacy text search")
	}
}

func TestParseGritArgsRejectsRemovedCompatibilityFlagAndTargetConflict(t *testing.T) {
	if _, err := parseGritArgs([]string{"--compatibility", "gritql-v1", "language go\n`x`"}); err == nil {
		t.Fatal("removed compatibility flag was accepted")
	}
	if _, err := parseGritArgs([]string{"--local", "--remote", "language go\n`x`"}); err == nil || !strings.Contains(err.Error(), "--local") {
		t.Fatalf("target conflict error=%v", err)
	}
	if _, _, err := compileGritQuery(gritArgs{Query: "language typescript\n`x`"}); err != nil {
		t.Fatalf("unified TypeScript compilation: %v", err)
	}
}

func TestLoadGritQueryFromStdin(t *testing.T) {
	withStdin(t, "language go\n`target($x)`", func() {
		query, err := loadGritQuery(gritArgs{QueryFile: "-"})
		if err != nil {
			t.Fatal(err)
		}
		if query != "language go\n`target($x)`" {
			t.Fatalf("query=%q", query)
		}
	})
}

func TestRunGritLocalAppliesScopePagingAndReportsTruncation(t *testing.T) {
	dir := chdirTemp(t)
	for _, name := range []string{"a.go", "b.go", "skip.go"} {
		content := "package p\nvar _ = target(" + strings.TrimSuffix(name, ".go") + ")\n"
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	output := captureStdout(t, func() {
		values := gritArgs{Query: "language go\n`target($x)`", JSON: true, Limit: 1, Skip: 1, ExcludeGlobs: []string{"skip.go"}}
		if err := runGritLocal(context.Background(), values); err != nil {
			t.Fatal(err)
		}
	})
	var response wire.GritResponse
	if err := json.Unmarshal([]byte(output), &response); err != nil {
		t.Fatal(err)
	}
	if response.Total != 2 || len(response.Findings) != 1 || response.Findings[0].Path != "b.go" {
		t.Fatalf("paged response=%#v", response)
	}

	output = captureStdout(t, func() {
		values := gritArgs{Query: "language go\n`target($x)`", JSON: true, Limit: 1, MaxFiles: 1}
		if err := runGritLocal(context.Background(), values); err != nil {
			t.Fatal(err)
		}
	})
	if err := json.Unmarshal([]byte(output), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Truncations) != 1 || response.Truncations[0].Reason != "max_files" || response.Truncations[0].Skipped != 2 {
		t.Fatalf("truncations=%#v", response.Truncations)
	}
}

func TestFormatGritCompileErrorIncludesCodeAndRange(t *testing.T) {
	values := gritArgs{Query: "language go\n`x` => `y`", Limit: DefaultGritResultLimit}
	if err := runGritLocal(context.Background(), values); err == nil || !strings.Contains(err.Error(), "PATTERN_UNSUPPORTED") || !strings.Contains(err.Error(), " at ") {
		t.Fatalf("compile error=%v", err)
	}
}

func TestRunGritLocalHonorsCanceledContext(t *testing.T) {
	dir := chdirTemp(t)
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package p\nvar x = target(value)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := runGritLocal(ctx, gritArgs{Query: "language go\n`target($x)`", Limit: DefaultGritResultLimit}); err == nil || !strings.Contains(err.Error(), "canceled") {
		t.Fatalf("cancellation error=%v", err)
	}
}

func TestRunGritRemoteUsesAuthenticatedStructuralEndpoint(t *testing.T) {
	t.Setenv("GREPPLE_TOKEN", "structural-token")
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/public/grit" {
			t.Errorf("request=%s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("authorization") != "Bearer structural-token" {
			t.Errorf("authorization=%q", request.Header.Get("authorization"))
		}
		var body wire.GritRequest
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body.Query != "language go\n`target($x)`" || body.Compatibility != wire.GritCompatibilityV1 {
			t.Errorf("body=%#v", body)
		}
		response.Header().Set("content-type", "application/json")
		_ = json.NewEncoder(response).Encode(wire.GritResponse{
			Metadata:    wire.GritMetadata{Compatibility: wire.GritCompatibilityV1, GoGrammar: "go1.25"},
			Findings:    []wire.GritFinding{{Repo: "acme/one", Path: "a.go", Language: "go", Text: "target(value)", Bindings: []wire.GritBinding{}}},
			Diagnostics: []wire.GritDiagnostic{}, Truncations: []wire.GritTruncation{}, ShardErrors: []string{}, Total: 1,
		})
	}))
	defer server.Close()

	output := captureStdout(t, func() {
		if err := runGrit([]string{"--remote", "--server", server.URL, "--json", "language go\n`target($x)`"}); err != nil {
			t.Fatal(err)
		}
	})
	var response wire.GritResponse
	if err := json.Unmarshal([]byte(output), &response); err != nil {
		t.Fatal(err)
	}
	if response.Total != 1 || len(response.Findings) != 1 || response.Findings[0].Repo != "acme/one" {
		t.Fatalf("response=%#v", response)
	}
}

func TestMergeGritResponsesNormalizesDeduplicatesAndAggregates(t *testing.T) {
	metadata := wire.GritMetadata{Compatibility: wire.GritCompatibilityV1, GoGrammar: "go1.25"}
	duplicate := gritFindingForTest("acme/remote", "dir/../a.go", 3, "x")
	distinctBinding := gritFindingForTest("acme/remote", "a.go", 3, "y")
	currentCheckout := gritFindingForTest("owner/current", "local.go", 1, "x")
	localFinding := gritFindingForTest("", "z.go", 2, "x")
	local := wire.GritResponse{
		Metadata: metadata, Findings: []wire.GritFinding{localFinding},
		Diagnostics: []wire.GritDiagnostic{}, Truncations: []wire.GritTruncation{}, ShardErrors: []string{},
		Statistics: wire.GritStatistics{Candidates: 2, Evaluated: 1, BytesRead: 10, SkippedAnchor: 1}, Total: 1,
	}
	remote := wire.GritResponse{
		Metadata: metadata, Findings: []wire.GritFinding{duplicate, duplicate, distinctBinding, currentCheckout},
		Diagnostics: []wire.GritDiagnostic{{Code: "REMOTE_WARNING"}},
		Truncations: []wire.GritTruncation{{Reason: "max_files", Limit: 3, Skipped: 1}},
		ShardErrors: []string{"shard unavailable"},
		Statistics:  wire.GritStatistics{Candidates: 4, Evaluated: 3, BytesRead: 20, SkippedAnchor: 2}, Total: 4,
	}

	merged, err := mergeGritResponses(local, remote, "owner/current")
	if err != nil {
		t.Fatal(err)
	}
	if merged.Total != 3 || len(merged.Findings) != 3 {
		t.Fatalf("findings=%#v total=%d", merged.Findings, merged.Total)
	}
	if merged.Findings[0].Path != "z.go" || merged.Findings[1].Path != "a.go" || merged.Findings[2].Bindings[0].Name != "y" {
		t.Fatalf("order=%#v", merged.Findings)
	}
	if merged.Statistics.Candidates != 6 || merged.Statistics.Evaluated != 4 || merged.Statistics.BytesRead != 30 || merged.Statistics.SkippedAnchor != 3 {
		t.Fatalf("statistics=%#v", merged.Statistics)
	}
	if len(merged.Diagnostics) != 1 || len(merged.Truncations) != 1 || len(merged.ShardErrors) != 1 {
		t.Fatalf("supplemental response fields lost: %#v", merged)
	}
	if merged.Findings == nil || merged.Diagnostics == nil || merged.Truncations == nil || merged.ShardErrors == nil {
		t.Fatalf("response collections must be non-nil: %#v", merged)
	}
}

func TestMergeGritResponsesRejectsIncompatibleMetadata(t *testing.T) {
	local := wire.GritResponse{Metadata: wire.GritMetadata{Compatibility: wire.GritCompatibilityV1, GoGrammar: "go1.25"}}
	remote := wire.GritResponse{Metadata: wire.GritMetadata{Compatibility: "other", GoGrammar: "go1.25"}}
	if _, err := mergeGritResponses(local, remote, ""); err == nil || !strings.Contains(err.Error(), "compatibility") {
		t.Fatalf("metadata error=%v", err)
	}
}
func TestMergeGritResponsesRejectsTargetGrammarMismatch(t *testing.T) {
	local := wire.GritResponse{Metadata: wire.GritMetadata{Compatibility: wire.GritCompatibilityV1, Language: "typescript", Grammar: "typescript"}}
	remote := wire.GritResponse{Metadata: wire.GritMetadata{Compatibility: wire.GritCompatibilityV1, Language: "tsx", Grammar: "tsx"}}
	if _, err := mergeGritResponses(local, remote, ""); err == nil || !strings.Contains(err.Error(), "language") {
		t.Fatalf("metadata error=%v", err)
	}
	remote.Metadata.Language = "typescript"
	if _, err := mergeGritResponses(local, remote, ""); err == nil || !strings.Contains(err.Error(), "grammar") {
		t.Fatalf("metadata error=%v", err)
	}
}

func TestWindowGritFindingsAppliesGlobalPage(t *testing.T) {
	findings := []wire.GritFinding{
		gritFindingForTest("", "a.go", 1, "x"),
		gritFindingForTest("acme/one", "b.go", 1, "x"),
		gritFindingForTest("acme/two", "c.go", 1, "x"),
	}
	page := windowGritFindings(findings, 1, 1)
	if len(page) != 1 || page[0].Repo != "acme/one" {
		t.Fatalf("page=%#v", page)
	}
}

func gritFindingForTest(repo, path string, line int, binding string) wire.GritFinding {
	rng := wire.GritRange{
		StartByte: line, EndByte: line + 1,
		Start: wire.GritPosition{Line: line, Column: 1},
		End:   wire.GritPosition{Line: line, Column: 2},
	}
	return wire.GritFinding{
		Repo: repo, Path: path, Language: "go", Range: rng, Text: "x", PatternID: "rule",
		Bindings: []wire.GritBinding{{Name: binding, Kind: wire.GritBindingNode, Range: rng, Structural: []wire.GritStructuralNode{}}},
	}
}

func TestRunGritRemoteComposesCurrentCheckoutAndGlobalPaging(t *testing.T) {
	dir := chdirTemp(t)
	runGitForTest(t, dir, "init")
	runGitForTest(t, dir, "remote", "add", "origin", "https://github.com/acme/current.git")
	if err := os.WriteFile(filepath.Join(dir, "local.go"), []byte("package p\nvar _ = target(local)\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	remoteFindings := gritCompositionRemoteFindings()
	requests := 0
	server := httptest.NewServer(gritCompositionHandler(t, remoteFindings, &requests))
	defer server.Close()

	output := captureStdout(t, func() {
		err := runGrit([]string{"--remote", "--server", server.URL, "--json", "--skip", "110", "--limit", "2", "language go\n`target($x)`"})
		if err != nil {
			t.Fatal(err)
		}
	})
	var response wire.GritResponse
	if err := json.Unmarshal([]byte(output), &response); err != nil {
		t.Fatal(err)
	}
	assertGritCompositionResponse(t, requests, response)
}

func gritCompositionRemoteFindings() []wire.GritFinding {
	findings := []wire.GritFinding{gritFindingForTest("acme/current", "local.go", 1, "x")}
	for index := 0; index < 130; index++ {
		findings = append(findings, gritFindingForTest("z/remote", fmt.Sprintf("file-%03d.go", index), 1, "x"))
	}
	return findings
}

func gritCompositionHandler(t *testing.T, findings []wire.GritFinding, requests *int) http.HandlerFunc {
	t.Helper()
	return func(response http.ResponseWriter, request *http.Request) {
		*requests++
		var body wire.GritRequest
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		if !slices.Contains(body.ExcludeRepositories, "acme/current") {
			t.Errorf("excludeRepositories=%v", body.ExcludeRepositories)
		}
		skip, limit := pointerInt(body.Skip), pointerInt(body.Limit)
		end := min(skip+limit, len(findings))
		skip = min(skip, end)
		_ = json.NewEncoder(response).Encode(wire.GritResponse{
			Metadata: wire.GritMetadata{Compatibility: wire.GritCompatibilityV1, GoGrammar: "go1.25"},
			Findings: findings[skip:end], Diagnostics: []wire.GritDiagnostic{},
			Truncations: []wire.GritTruncation{}, ShardErrors: []string{"one shard unavailable"}, Total: len(findings),
		})
	}
}

func assertGritCompositionResponse(t *testing.T, requests int, response wire.GritResponse) {
	t.Helper()
	if requests != 2 {
		t.Fatalf("requests=%d, want 2", requests)
	}
	if response.Total != 131 || len(response.Findings) != 2 || response.Findings[0].Path != "file-109.go" || response.Findings[1].Path != "file-110.go" {
		t.Fatalf("response=%#v", response)
	}
	if len(response.ShardErrors) != 1 || response.ShardErrors[0] != "one shard unavailable" {
		t.Fatalf("shard errors=%v", response.ShardErrors)
	}
}

func pointerInt(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func TestCollectGritRemoteHonorsCancellationBeforeTransport(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests++ }))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	request := wire.GritRequest{Query: "language go\n`x`", Compatibility: wire.GritCompatibilityV1}
	if _, err := collectGritRemote(ctx, request, server.URL, 1); err == nil || !strings.Contains(err.Error(), "canceled") {
		t.Fatalf("cancellation error=%v", err)
	}
	if requests != 0 {
		t.Fatalf("requests=%d", requests)
	}
}

func TestRunGritRemoteDoesNotHideTransportFailure(t *testing.T) {
	chdirTemp(t)
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	serverURL := server.URL
	server.Close()
	err := runGrit([]string{"--remote", "--server", serverURL, "language go\n`target($x)`"})
	if err == nil {
		t.Fatal("remote transport failure was silently accepted")
	}
}

func TestRenderGritHumanBoundsFindings(t *testing.T) {
	finding := wire.GritFinding{Path: "large.go", Text: strings.Repeat("x", 200), Range: wire.GritRange{Start: wire.GritPosition{Line: 1, Column: 1}, End: wire.GritPosition{Line: 1, Column: 201}}}
	var renderErr error
	output := captureStdout(t, func() {
		renderErr = renderGritHuman(wire.GritResponse{Findings: []wire.GritFinding{finding}}, 200)
	})
	if !errors.Is(renderErr, errOutputTruncated) {
		t.Fatalf("render error=%v", renderErr)
	}
	if len(output) > 200 || !strings.Contains(output, "truncated") {
		t.Fatalf("bounded output length=%d: %q", len(output), output)
	}
}
