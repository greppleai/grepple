package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/greppleai/grepple/api"
)

func TestSegmentContextGuardOmitsUnchangedCompleteDeclaration(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("GREPPLE_CONTEXT_GUARD_DIR", directory)
	segment := api.ResultSegment{Kind: "lines", Start: 10, End: 12, Text: "type Something struct {\n\tName string // " + strings.Repeat("unchanged declaration detail ", 8) + "\n}"}
	result := api.FileResult{Path: "model.go", Segments: []api.ResultSegment{segment}}
	first := renderWithSegmentGuard(t, result)
	if !strings.Contains(first, "type Something struct") {
		t.Fatalf("first declaration missing: %q", first)
	}
	second := renderWithSegmentGuard(t, result)
	if strings.Contains(second, "type Something struct") || !strings.Contains(second, "unchanged segment already emitted: model.go:10-12") {
		t.Fatalf("unchanged declaration was not omitted: %q", second)
	}
	cacheContent, err := os.ReadFile(filepath.Join(directory, "cache.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(cacheContent), "Something") || strings.Contains(string(cacheContent), "Name string") {
		t.Fatalf("cache persisted source: %s", cacheContent)
	}
	stats := readRenderedContextStats(filepath.Join(directory, "stats-0.json"), 0)
	if stats.ObservedCalls != 2 || stats.EmittedSegments != 1 || stats.RemovedSegments != 1 || stats.NetSavedBytes <= 0 {
		t.Fatalf("unexpected stats: %#v", stats)
	}
}

func TestSegmentContextGuardEmitsChangedDeclaration(t *testing.T) {
	t.Setenv("GREPPLE_CONTEXT_GUARD_DIR", t.TempDir())
	original := api.FileResult{Path: "service.go", Segments: []api.ResultSegment{{Kind: "function", Start: 4, End: 6, Text: "func Run() {\n\toldCall()\n}"}}}
	if output := renderWithSegmentGuard(t, original); !strings.Contains(output, "oldCall") {
		t.Fatalf("original missing: %q", output)
	}
	changed := original
	changed.Segments = []api.ResultSegment{{Kind: "function", Start: 4, End: 6, Text: "func Run() {\n\tnewCall()\n}"}}
	if output := renderWithSegmentGuard(t, changed); !strings.Contains(output, "newCall") || strings.Contains(output, "already emitted") {
		t.Fatalf("changed declaration was suppressed: %q", output)
	}
}

func TestSegmentContextGuardOmitsExactExternalTypeBody(t *testing.T) {
	t.Setenv("GREPPLE_CONTEXT_GUARD_DIR", t.TempDir())
	artifact := &api.NavigationArtifactIdentity{Module: "client", Version: "1.0.0", Commit: "one", Digest: "artifact-one", Repository: "acme/client@tag~v1.0.0"}
	segment := api.ResultSegment{Kind: "lines", Start: 20, End: 22, Text: "export interface Client {\n  request(): Promise<Response>\n}"}
	result := api.FileResult{Path: "consumer.ts", Related: []api.RelatedSymbol{{
		Name: "Client", Path: "src/client.ts", Direction: "type", Role: "parameter", Start: 20, End: 22,
		Artifact: artifact, Segments: []api.ResultSegment{segment},
	}}}
	first := renderWithSegmentGuard(t, result)
	if !strings.Contains(first, "export interface Client") {
		t.Fatalf("external type missing: %q", first)
	}
	second := renderWithSegmentGuard(t, result)
	if strings.Contains(second, "export interface Client") || !strings.Contains(second, "unchanged remote parameter type Client already emitted at src/client.ts:20-22") {
		t.Fatalf("external type body was not omitted: %q", second)
	}
	if !strings.Contains(second, "acme/client@tag~v1.0.0:src/client.ts:20-22") {
		t.Fatalf("external provenance was lost: %q", second)
	}
}

func TestRelatedTypeContextLabelPreservesAllRoles(t *testing.T) {
	artifact := &api.NavigationArtifactIdentity{Digest: "artifact-one"}
	segment := api.ResultSegment{Kind: "lines", Start: 1, End: 1, Text: "type Client struct{}"}
	definitions := collectRelatedTypeDefinitions([]api.FileResult{{Related: []api.RelatedSymbol{
		{Name: "Client", Path: "client.go", Direction: "type", Role: "result", Start: 1, End: 1, Artifact: artifact, Segments: []api.ResultSegment{segment}},
		{Name: "Client", Path: "client.go", Direction: "type", Role: "parameter", Start: 1, End: 1, Artifact: artifact, Segments: []api.ResultSegment{segment}},
	}}})
	if len(definitions) != 1 {
		t.Fatalf("got %d definitions", len(definitions))
	}
	if label := relatedTypeContextLabel(definitions[0]); label != "remote parameter/result type Client" {
		t.Fatalf("unexpected label %q", label)
	}
}

func TestSegmentContextGuardUsesExactArtifactIdentity(t *testing.T) {
	t.Setenv("GREPPLE_CONTEXT_GUARD_DIR", t.TempDir())
	segment := api.ResultSegment{Kind: "lines", Start: 1, End: 1, Text: "export interface Client {}"}
	first := &api.NavigationArtifactIdentity{Module: "client", Version: "1.0.0", Commit: "one", Digest: "artifact-one"}
	second := &api.NavigationArtifactIdentity{Module: "client", Version: "2.0.0", Commit: "two", Digest: "artifact-two"}
	guard, err := openSegmentContextGuard()
	if err != nil {
		t.Fatal(err)
	}
	if guard.seen("client.ts", first, segment) {
		t.Fatal("new artifact was already present")
	}
	guard.record("client.ts", first, segment)
	guard.close()
	guard, err = openSegmentContextGuard()
	if err != nil {
		t.Fatal(err)
	}
	defer guard.close()
	if !guard.seen("client.ts", first, segment) {
		t.Fatal("same exact artifact was not recognized")
	}
	if guard.seen("client.ts", second, segment) {
		t.Fatal("different artifact version was suppressed")
	}
}

func TestSegmentContextGuardRejectsPartialRanges(t *testing.T) {
	partial := api.ResultSegment{Kind: "lines", Start: 10, End: 20, Text: "only one returned line"}
	if completeStructuralSegment(partial) {
		t.Fatal("partial range treated as a complete declaration")
	}
}

func TestSegmentContextInvalidationRotatesStatsAndRestoresDeclaration(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("GREPPLE_CONTEXT_GUARD_DIR", directory)
	result := api.FileResult{Path: "model.go", Segments: []api.ResultSegment{{Kind: "lines", Start: 1, End: 1, Text: "type Model struct{}"}}}
	renderWithSegmentGuard(t, result)
	if err := invalidateRenderedContext("compact"); err != nil {
		t.Fatal(err)
	}
	rotated := readRenderedContextStats(filepath.Join(directory, "stats-1.json"), 1)
	if rotated.Period != 1 || rotated.ResetReason != "compact" || rotated.ObservedCalls != 0 {
		t.Fatalf("stats did not rotate: %#v", rotated)
	}
	if output := renderWithSegmentGuard(t, result); !strings.Contains(output, "type Model struct") {
		t.Fatalf("declaration remained suppressed after invalidation: %q", output)
	}
}

func TestContextGuardIsDisabledForPotentiallySpilledResult(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("GREPPLE_CONTEXT_GUARD_DIR", directory)
	previous := activeInlineOutputThreshold
	activeInlineOutputThreshold = 1
	defer func() { activeInlineOutputThreshold = previous }()
	options := &cliOptions{JSON: "off"}
	results := []api.FileResult{{Path: "large.go", Segments: []api.ResultSegment{{Kind: "lines", Start: 1, End: 1, Text: "type Large struct{}"}}}}
	guard := contextGuardForResults(options, results)
	if guard == nil || guard.deduplicate || guard.bypassReason != "potential-spill" {
		t.Fatalf("potential spill was not tracked as a bypass: %#v", guard)
	}
	guard.close()
	if _, err := os.Stat(filepath.Join(directory, "cache.json")); !os.IsNotExist(err) {
		t.Fatalf("spill preflight created cache: %v", err)
	}
	stats := readRenderedContextStats(filepath.Join(directory, "stats-0.json"), 0)
	if stats.ObservedCalls != 1 || stats.PotentialSpillCalls != 1 || stats.DeduplicationEnabledCalls != 0 {
		t.Fatalf("potential spill call was not recorded: %#v", stats)
	}
}

func TestContextGuardRecordsNonStructuralSearchCalls(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("GREPPLE_CONTEXT_GUARD_DIR", directory)
	previous := activeInlineOutputThreshold
	activeInlineOutputThreshold = 1024
	defer func() { activeInlineOutputThreshold = previous }()
	guard := contextGuardForResults(&cliOptions{JSON: "off", LineOnly: true}, []api.FileResult{{Path: "one.go"}, {Path: "two.go"}})
	if guard == nil || guard.deduplicate || guard.bypassReason != "non-structural" {
		t.Fatalf("non-structural call was not tracked: %#v", guard)
	}
	guard.returnedBytes = 321
	guard.close()
	stats := readRenderedContextStats(filepath.Join(directory, "stats-0.json"), 0)
	if stats.ObservedCalls != 1 || stats.NonStructuralCalls != 1 || stats.ResultFiles != 2 || stats.ReturnedBytes != 321 {
		t.Fatalf("non-structural call was not recorded: %#v", stats)
	}
}

func TestContextStatsSurviveCacheWriteFailure(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("GREPPLE_CONTEXT_GUARD_DIR", directory)
	guard, err := openSegmentContextGuard()
	if err != nil {
		t.Fatal(err)
	}
	guard.observed = true
	guard.cachePath = t.TempDir() // Renaming a cache file over this directory must fail.
	guard.close()
	stats := readRenderedContextStats(filepath.Join(directory, "stats-0.json"), 0)
	if stats.ObservedCalls != 1 || stats.DeduplicationEnabledCalls != 1 {
		t.Fatalf("cache failure prevented statistics: %#v", stats)
	}
}

func TestRunContextInvalidateValidation(t *testing.T) {
	t.Setenv("GREPPLE_CONTEXT_GUARD_DIR", t.TempDir())
	if err := runContext([]string{"invalidate", "--reason", "compact"}); err != nil {
		t.Fatal(err)
	}
	if err := runContext([]string{"invalidate", "--unknown"}); err == nil {
		t.Fatal("unknown argument accepted")
	}
}

func renderWithSegmentGuard(t *testing.T, result api.FileResult) string {
	t.Helper()
	guard, err := openSegmentContextGuard()
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	renderer := segmentRenderer{output: newOutputWriter(&output), contextGuard: guard}
	if err := renderer.Render([]api.FileResult{result}); err != nil {
		guard.close()
		t.Fatal(err)
	}
	guard.close()
	return output.String()
}
