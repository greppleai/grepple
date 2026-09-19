package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
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
	if stats.Calls.Total != 2 || stats.Details.SegmentsEmitted != 1 || stats.Details.SegmentsRemoved != 1 || stats.TotalBytesRemoved <= 0 {
		t.Fatalf("unexpected stats: %#v", stats)
	}
	expectedPercent := float64(stats.TotalBytesRemoved) * 100 / float64(stats.TotalBytesReturned)
	if stats.SavingsPercent != expectedPercent {
		t.Fatalf("savings percentage = %v, want %v from returned bytes", stats.SavingsPercent, expectedPercent)
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
	if rotated.Period != 1 || rotated.ResetReason != "compact" || rotated.Calls.Total != 0 {
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
	if stats.Calls.Total != 1 || stats.Calls.PotentialSpillReads != 1 || stats.Calls.StructuredReads != 1 {
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
	if stats.Calls.Total != 1 || stats.Calls.Reads != 1 || stats.Calls.OtherReads != 1 || stats.Details.ResultFiles != 2 || stats.TotalBytesReturned != 321 {
		t.Fatalf("non-structural call was not recorded: %#v", stats)
	}
}

func TestRepeatSourceBypassesAndRecordsContextCache(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("GREPPLE_CONTEXT_GUARD_DIR", directory)
	previous := activeInlineOutputThreshold
	activeInlineOutputThreshold = 4096
	defer func() { activeInlineOutputThreshold = previous }()
	result := api.FileResult{Path: "repeat.go", Segments: []api.ResultSegment{{Kind: "function", Start: 1, End: 1, Text: "func Repeat() {}"}}}
	renderWithSegmentGuard(t, result)
	guard := contextGuardForResults(&cliOptions{JSON: "off", RepeatSource: true}, []api.FileResult{result})
	if guard == nil || guard.deduplicate || !guard.recordSegments || guard.bypassReason != "requested" {
		t.Fatalf("repeat-source guard = %#v", guard)
	}
	var output bytes.Buffer
	if err := (segmentRenderer{output: newOutputWriter(&output), contextGuard: guard}).Render([]api.FileResult{result}); err != nil {
		t.Fatal(err)
	}
	guard.returnedBytes = output.Len()
	guard.close()
	if !strings.Contains(output.String(), "func Repeat()") || strings.Contains(output.String(), "already emitted") {
		t.Fatalf("repeat source did not emit the declaration: %q", output.String())
	}
	stats := readRenderedContextStats(filepath.Join(directory, "stats-0.json"), 0)
	if stats.Calls.BypassReads != 1 {
		t.Fatalf("requested bypass was not counted: %#v", stats)
	}
}

func TestWriteAnchorsCompletePreviouslyEmittedSegmentCoverage(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("GREPPLE_CONTEXT_GUARD_DIR", directory)
	previous := activeInlineOutputThreshold
	activeInlineOutputThreshold = 4096
	defer func() { activeInlineOutputThreshold = previous }()
	root := t.TempDir()
	path := filepath.Join(root, "service.go")
	oldSegment := api.ResultSegment{Kind: "function", Start: 1, End: 3, Text: "func Run() {\n\toldCall()\n}"}
	renderWithSegmentGuard(t, api.FileResult{Path: path, Segments: []api.ResultSegment{oldSegment}})
	recordWriteResponseContext(root, writeResponse{Applied: true, Files: []writeResponseFile{{
		Path: "service.go", Operation: "edit", Changed: true,
		ChangeDetails: []writeResponseChange{{Anchors: []writeAnchor{{Line: 2, Content: "\tnewCall()"}}}},
	}}}, 100, true, false, true)
	cacheContent, err := os.ReadFile(filepath.Join(directory, "cache.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(cacheContent), "newCall") {
		t.Fatalf("write coverage persisted source text: %s", cacheContent)
	}
	newSegment := api.ResultSegment{Kind: "function", Start: 1, End: 3, Text: "func Run() {\n\tnewCall()\n}"}
	guard, err := openSegmentContextGuard()
	if err != nil {
		t.Fatal(err)
	}
	if !guard.seen(path, nil, newSegment) {
		guard.close()
		t.Fatal("write delta did not complete coverage for the updated segment")
	}
	guard.close()
	stats := readRenderedContextStats(filepath.Join(directory, "stats-0.json"), 0)
	if stats.Calls.Writes != 1 || stats.Calls.AppliedWrites != 1 || stats.Details.WriteAnchorsRecorded != 1 || stats.Details.SegmentsRemovedByLineCoverage != 1 {
		t.Fatalf("write coverage statistics = %#v", stats)
	}
}

func TestWriteFailureIsRecordedInContextStats(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("GREPPLE_CONTEXT_GUARD_DIR", directory)
	recordWriteResponseContext(t.TempDir(), writeResponse{}, 123, false, true, false)
	stats := readRenderedContextStats(filepath.Join(directory, "stats-0.json"), 0)
	if stats.Calls.Total != 1 || stats.Calls.Writes != 1 || stats.Calls.FailedWrites != 1 || stats.Calls.SuccessfulWrites != 0 || stats.Calls.AppliedWrites != 0 {
		t.Fatalf("write failure statistics = %#v", stats)
	}
	if stats.TotalBytesReturned != 123 {
		t.Fatalf("write failure returned bytes = %d", stats.TotalBytesReturned)
	}
}

func TestLegacyContextStatsMigrateToSimpleTotals(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stats-0.json")
	legacy := legacyRenderedContextStats{
		Schema: "grepple-context-segment-stats-v4", Period: 0, ObservedCalls: 10,
		DeduplicationEnabledCalls: 5, LineCoverageCalls: 2, WriteCalls: 2,
		BypassRequestedCalls: 1, PotentialSpillCalls: 1, ReturnedBytes: 1000,
		NetSavedBytes: 50, EmittedSourceBytes: 300, SearchLineBytesEmitted: 25,
		EmittedSegments: 4, RemovedSegments: 3, WriteAnchorsRecorded: 2,
	}
	content, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	stats := readRenderedContextStats(path, 0)
	if stats.Schema != contextGuardStatsSchema || stats.TotalBytesReturned != 1000 || stats.TotalBytesRemoved != 50 || stats.TotalBytesEmitted != 325 || stats.SavingsPercent != 5 {
		t.Fatalf("migrated byte totals = %#v", stats)
	}
	if stats.Calls.Total != 10 || stats.Calls.Reads != 8 || stats.Calls.StructuredReads != 3 || stats.Calls.FocusedReads != 2 || stats.Calls.OtherReads != 3 || stats.Calls.Writes != 2 {
		t.Fatalf("migrated call totals = %#v", stats.Calls)
	}
}

func TestWriteAnchorsDoNotSuppressPartiallyCoveredSegment(t *testing.T) {
	t.Setenv("GREPPLE_CONTEXT_GUARD_DIR", t.TempDir())
	guard, err := openSegmentContextGuard()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "partial.go")
	guard.recordWriteAnchors(path, []writeAnchor{{Line: 2, Content: "\tcovered()"}})
	segment := api.ResultSegment{Kind: "function", Start: 1, End: 3, Text: "func Partial() {\n\tcovered()\n}"}
	if guard.seen(path, nil, segment) {
		guard.close()
		t.Fatal("partial write coverage suppressed a complete declaration")
	}
	guard.close()
}

func TestContextGuardCanBeDisabledInUserSettings(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("GREPPLE_CONTEXT_GUARD_DIR", directory)
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	t.Setenv("GREPPLE_SETTINGS", settingsPath)
	disabled := false
	writeJSONFile(t, settingsPath, userSettings{ContextGuard: contextGuardSettings{Enabled: &disabled}})
	previous := activeInlineOutputThreshold
	activeInlineOutputThreshold = 4096
	defer func() { activeInlineOutputThreshold = previous }()
	result := api.FileResult{Path: "disabled.go", Segments: []api.ResultSegment{{Kind: "function", Start: 1, End: 1, Text: "func Disabled() {}"}}}
	if guard := contextGuardForResults(&cliOptions{JSON: "off"}, []api.FileResult{result}); guard != nil {
		guard.close()
		t.Fatal("disabled context guard opened for search output")
	}
	recordWriteResponseContext(t.TempDir(), writeResponse{Applied: true}, 10, true, false, true)
	if _, err := os.Stat(filepath.Join(directory, "cache.json")); !os.IsNotExist(err) {
		t.Fatalf("disabled context guard wrote cache state: %v", err)
	}
	if _, err := os.Stat(filepath.Join(directory, "stats-0.json")); !os.IsNotExist(err) {
		t.Fatalf("disabled context guard wrote statistics: %v", err)
	}
}

func TestAnchoredContextReadProducesCoverageWithoutChangingOutput(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("GREPPLE_CONTEXT_GUARD_DIR", directory)
	previous := activeInlineOutputThreshold
	activeInlineOutputThreshold = 4096
	defer func() { activeInlineOutputThreshold = previous }()
	lineResult, segment, anchors := focusedLineCoverageFixture(t)
	contextLines := make([]api.ContextLine, len(lineResult.Matches))
	for index, match := range lineResult.Matches {
		contextLines[index] = api.ContextLine{Line: match.Line, Text: match.Text, Match: index == 3}
	}
	result := api.FileResult{Path: lineResult.Path, Context: contextLines}
	first := renderContextWithGuard(t, result, anchors)
	second := renderContextWithGuard(t, result, anchors)
	if first != second || strings.Contains(second, "omitted; unchanged") || !strings.Contains(second, "lineThree") {
		t.Fatalf("context output changed after recording coverage: first=%q second=%q", first, second)
	}
	stats := readRenderedContextStats(filepath.Join(directory, "stats-0.json"), 0)
	if stats.Calls.OtherReads != 2 || stats.Details.SearchLinesRecorded != 8 || stats.TotalBytesRemoved != 0 {
		t.Fatalf("context producer statistics = %#v", stats)
	}
	guard, err := openSegmentContextGuard()
	if err != nil {
		t.Fatal(err)
	}
	if !guard.seen(result.Path, nil, segment) {
		guard.close()
		t.Fatal("anchored context lines did not cover the later structural segment")
	}
	guard.close()
}

func TestContextProducerDoesNotRecordPotentiallySpilledRows(t *testing.T) {
	t.Setenv("GREPPLE_CONTEXT_GUARD_DIR", t.TempDir())
	previous := activeInlineOutputThreshold
	activeInlineOutputThreshold = 1
	defer func() { activeInlineOutputThreshold = previous }()
	lineResult, _, anchors := focusedLineCoverageFixture(t)
	result := api.FileResult{Path: lineResult.Path, Context: []api.ContextLine{{Line: 1, Text: "lineOne", Match: true}}}
	options := &cliOptions{JSON: "off", AnchorLines: anchors}
	options.Params.BeforeContext = 1
	guard := contextGuardForResults(options, []api.FileResult{result})
	if guard == nil || guard.recordLines || guard.bypassReason != "potential-spill" {
		t.Fatalf("context potential-spill guard = %#v", guard)
	}
	guard.close()
}

func TestBroadLineProducerDoesNotRecordPotentiallySpilledRows(t *testing.T) {
	t.Setenv("GREPPLE_CONTEXT_GUARD_DIR", t.TempDir())
	previous := activeInlineOutputThreshold
	activeInlineOutputThreshold = 1
	defer func() { activeInlineOutputThreshold = previous }()
	result, _, anchors := focusedLineCoverageFixture(t)
	guard := contextGuardForResults(&cliOptions{JSON: "off", LineOnly: true, AnchorLines: anchors}, []api.FileResult{result})
	if guard == nil || guard.recordLines || guard.bypassReason != "potential-spill" {
		t.Fatalf("broad potential-spill guard = %#v", guard)
	}
	guard.close()
}

func TestBroadAnchoredLineReadProducesCoverageWithoutSuppressingRows(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("GREPPLE_CONTEXT_GUARD_DIR", directory)
	previous := activeInlineOutputThreshold
	activeInlineOutputThreshold = 4096
	defer func() { activeInlineOutputThreshold = previous }()
	result, segment, anchors := focusedLineCoverageFixture(t)
	first := renderBroadLinesWithGuard(t, result, anchors)
	second := renderBroadLinesWithGuard(t, result, anchors)
	if first != second || strings.Contains(second, "omitted; unchanged") || !strings.Contains(second, "lineThree") {
		t.Fatalf("broad line output changed after recording coverage: first=%q second=%q", first, second)
	}
	stats := readRenderedContextStats(filepath.Join(directory, "stats-0.json"), 0)
	if stats.Calls.OtherReads != 2 || stats.Details.SearchLinesRecorded != 8 || stats.TotalBytesRemoved != 0 {
		t.Fatalf("broad line producer statistics = %#v", stats)
	}
	guard, err := openSegmentContextGuard()
	if err != nil {
		t.Fatal(err)
	}
	if !guard.seen(result.Path, nil, segment) {
		guard.close()
		t.Fatal("broad anchored lines did not cover the later structural segment")
	}
	guard.close()
}

func TestStructuralCoverageCollapsesFocusedAtRange(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("GREPPLE_CONTEXT_GUARD_DIR", directory)
	previous := activeInlineOutputThreshold
	activeInlineOutputThreshold = 4096
	defer func() { activeInlineOutputThreshold = previous }()
	result, segment, anchors := focusedLineCoverageFixture(t)
	renderWithSegmentGuard(t, api.FileResult{Path: result.Path, Segments: []api.ResultSegment{segment}})
	output := renderFocusedAtWithGuard(t, result, anchors, false)
	if !strings.Contains(output, "lines 2-7 omitted; unchanged anchored source already exists in context") {
		t.Fatalf("focused range was not collapsed: %q", output)
	}
	if !strings.Contains(output, "lineOne") || strings.Contains(output, "lineThree") || !strings.Contains(output, "lineEight") {
		t.Fatalf("focused range boundaries were not preserved: %q", output)
	}
	stats := readRenderedContextStats(filepath.Join(directory, "stats-0.json"), 0)
	if stats.Calls.FocusedReads != 1 || stats.Details.LineRangesRemoved != 1 || stats.Details.LinesRemoved != 6 {
		t.Fatalf("focused range statistics = %#v", stats)
	}
}

func TestFocusedAtCoverageSuppressesLaterStructuralSegment(t *testing.T) {
	t.Setenv("GREPPLE_CONTEXT_GUARD_DIR", t.TempDir())
	previous := activeInlineOutputThreshold
	activeInlineOutputThreshold = 4096
	defer func() { activeInlineOutputThreshold = previous }()
	result, segment, anchors := focusedLineCoverageFixture(t)
	first := renderFocusedAtWithGuard(t, result, anchors, false)
	if strings.Contains(first, "omitted; unchanged") {
		t.Fatalf("new focused range was unexpectedly collapsed: %q", first)
	}
	guard, err := openSegmentContextGuard()
	if err != nil {
		t.Fatal(err)
	}
	if !guard.seen(result.Path, nil, segment) {
		guard.close()
		t.Fatal("focused --at coverage did not suppress the complete structural segment")
	}
	guard.close()
}

func TestPartialFocusedCoverageCollapsesStructuralRuns(t *testing.T) {
	t.Setenv("GREPPLE_CONTEXT_GUARD_DIR", t.TempDir())
	previous := activeInlineOutputThreshold
	activeInlineOutputThreshold = 4096
	defer func() { activeInlineOutputThreshold = previous }()
	result, segment, anchors := focusedLineCoverageFixture(t)
	guard, err := openSegmentContextGuard()
	if err != nil {
		t.Fatal(err)
	}
	guard.observed = true
	for _, match := range result.Matches[1:7] {
		guard.recordWriteAnchors(result.Path, []writeAnchor{{Line: match.Line, Content: match.Text}})
	}
	guard.close()
	guard = contextGuardForResults(&cliOptions{JSON: "off", AnchorLines: anchors}, []api.FileResult{{Path: result.Path, Segments: []api.ResultSegment{segment}}})
	if guard == nil {
		t.Fatal("structural context guard was not enabled")
	}
	var output bytes.Buffer
	if err := (segmentRenderer{output: newOutputWriter(&output), anchors: anchors, contextGuard: guard}).Render([]api.FileResult{{Path: result.Path, Segments: []api.ResultSegment{segment}}}); err != nil {
		guard.close()
		t.Fatal(err)
	}
	guard.returnedBytes = output.Len()
	guard.close()
	if !strings.Contains(output.String(), "lines 3-6 omitted; unchanged anchored source already exists in context") || !strings.Contains(output.String(), "lineOne") || !strings.Contains(output.String(), "lineEight") {
		t.Fatalf("structural covered run was not collapsed safely: %q", output.String())
	}
}

func TestRepeatSourceRestoresFocusedAtRange(t *testing.T) {
	t.Setenv("GREPPLE_CONTEXT_GUARD_DIR", t.TempDir())
	previous := activeInlineOutputThreshold
	activeInlineOutputThreshold = 4096
	defer func() { activeInlineOutputThreshold = previous }()
	result, segment, anchors := focusedLineCoverageFixture(t)
	renderWithSegmentGuard(t, api.FileResult{Path: result.Path, Segments: []api.ResultSegment{segment}})
	output := renderFocusedAtWithGuard(t, result, anchors, true)
	if strings.Contains(output, "omitted; unchanged") || !strings.Contains(output, "lineThree") {
		t.Fatalf("--repeat-source did not restore focused lines: %q", output)
	}
	stats := readRenderedContextStats(filepath.Join(os.Getenv("GREPPLE_CONTEXT_GUARD_DIR"), "stats-0.json"), 0)
	if stats.Calls.BypassReads != 1 {
		t.Fatalf("focused bypass was not counted: %#v", stats)
	}
}

func focusedLineCoverageFixture(t *testing.T) (api.FileResult, api.ResultSegment, anchorLookup) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "focused.go")
	lines := []string{"lineOne", "lineTwo", "lineThree", "lineFour", "lineFive", "lineSix", "lineSeven", "lineEight"}
	matches := make([]api.ResultMatch, 0, len(lines))
	anchors := anchorLookup{path: map[int]string{}}
	for index, line := range lines {
		lineNumber := index + 1
		matches = append(matches, api.ResultMatch{Line: lineNumber, Text: line})
		anchors[path][lineNumber] = fmt.Sprintf("h%02d", lineNumber)
	}
	return api.FileResult{Path: path, Matches: matches}, api.ResultSegment{Kind: "function", Start: 1, End: len(lines), Text: strings.Join(lines, "\n")}, anchors
}

func renderContextWithGuard(t *testing.T, result api.FileResult, anchors anchorLookup) string {
	t.Helper()
	options := &cliOptions{JSON: "off", AnchorLines: anchors}
	options.Params.BeforeContext = 1
	guard := contextGuardForResults(options, []api.FileResult{result})
	if guard == nil || !guard.recordLines || guard.deduplicate {
		t.Fatalf("context producer guard = %#v", guard)
	}
	var output bytes.Buffer
	renderer := contextRenderer{output: newOutputWriter(&output), anchors: anchors, contextGuard: guard}
	if err := renderer.Render([]api.FileResult{result}); err != nil {
		guard.close()
		t.Fatal(err)
	}
	guard.returnedBytes = output.Len()
	guard.close()
	return output.String()
}

func renderBroadLinesWithGuard(t *testing.T, result api.FileResult, anchors anchorLookup) string {
	t.Helper()
	options := &cliOptions{JSON: "off", LineOnly: true, AnchorLines: anchors}
	guard := contextGuardForResults(options, []api.FileResult{result})
	if guard == nil || !guard.recordLines || guard.deduplicate {
		t.Fatalf("broad line producer guard = %#v", guard)
	}
	var output bytes.Buffer
	renderer := lineRenderer{output: newOutputWriter(&output), anchors: anchors, contextGuard: guard}
	if err := renderer.Render([]api.FileResult{result}); err != nil {
		guard.close()
		t.Fatal(err)
	}
	guard.returnedBytes = output.Len()
	guard.close()
	return output.String()
}

func renderFocusedAtWithGuard(t *testing.T, result api.FileResult, anchors anchorLookup, repeat bool) string {
	t.Helper()
	options := &cliOptions{JSON: "off", LineOnly: true, RepeatSource: repeat, AnchorLines: anchors}
	options.Params.At = result.Path + ":1-8"
	guard := contextGuardForResults(options, []api.FileResult{result})
	if guard == nil {
		t.Fatal("focused line context guard was not enabled")
	}
	var output bytes.Buffer
	renderer := lineRenderer{output: newOutputWriter(&output), anchors: anchors, contextGuard: guard, repeatSource: repeat}
	if err := renderer.Render([]api.FileResult{result}); err != nil {
		guard.close()
		t.Fatal(err)
	}
	guard.returnedBytes = output.Len()
	guard.close()
	return output.String()
}

func TestContextStatsSurviveCacheWriteFailure(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("GREPPLE_CONTEXT_GUARD_DIR", directory)
	guard, err := openSegmentContextGuard()
	if err != nil {
		t.Fatal(err)
	}
	guard.observed = true
	guard.cacheDirty = true
	guard.cachePath = t.TempDir() // Renaming a cache file over this directory must fail.
	guard.close()
	stats := readRenderedContextStats(filepath.Join(directory, "stats-0.json"), 0)
	if stats.Calls.Total != 1 || stats.Calls.Reads != 1 {
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
	guard.returnedBytes = output.Len()
	guard.close()
	return output.String()
}
