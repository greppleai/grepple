package cli

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"charm.land/fantasy"
)

func TestAskTelemetrySeparatesLLMAndOverlappingToolWallTime(t *testing.T) {
	start := time.Date(2026, 9, 16, 8, 0, 0, 0, time.UTC)
	telemetry := newAskTelemetry(start)
	telemetry.startStream(start.Add(10 * time.Millisecond))
	first := telemetry.beginTool("search_code", map[string]any{"query": "Symbol", "mode": "files"}, start.Add(20*time.Millisecond))
	telemetry.startToolExecution(first, start.Add(25*time.Millisecond))
	second := telemetry.beginTool("read_file", map[string]any{"path": "source.go", "start_line": 4, "end_line": 9}, start.Add(40*time.Millisecond))
	firstEvent := telemetry.finishTool(first, start.Add(50*time.Millisecond), fantasy.NewTextResponse("first evidence"), nil, researchCacheStatus{Tool: "search_code", Key: "first", Executed: true})
	telemetry.finishTool(second, start.Add(70*time.Millisecond), fantasy.NewTextResponse("second evidence"), nil, researchCacheStatus{Tool: "read_file", Key: "second", Hit: true})
	telemetry.finishStream(start.Add(100 * time.Millisecond))

	performance := telemetry.performance(start.Add(120 * time.Millisecond))
	assertMilliseconds(t, "total", performance.TotalDurationMS, 120)
	assertMilliseconds(t, "stream", performance.StreamDurationMS, 90)
	assertMilliseconds(t, "tool wall", performance.ToolWallDurationMS, 50)
	assertMilliseconds(t, "tool cumulative", performance.ToolCumulativeDurationMS, 60)
	assertMilliseconds(t, "tool execution wall", performance.ToolExecutionWallDurationMS, 25)
	assertMilliseconds(t, "tool execution cumulative", performance.ToolExecutionCumulativeDurationMS, 25)
	assertMilliseconds(t, "LLM requests", performance.LLMDurationMS, 0)
	assertMilliseconds(t, "non-tool wall", performance.NonToolWallDurationMS, 40)
	assertMilliseconds(t, "other", performance.OtherDurationMS, 30)
	if performance.Schema != askPerformanceSchema || performance.ToolCalls != 2 || performance.ToolExecutions != 1 || len(performance.Tools) != 2 || performance.Tools[0].Tool != "read_file" || performance.Tools[1].Tool != "search_code" {
		t.Fatalf("performance=%+v", performance)
	}
	encoded, err := json.Marshal(firstEvent)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	if !strings.Contains(text, `"input":{"mode":"files","query":"Symbol"}`) || !strings.Contains(text, `"responseBytes":14`) || !strings.Contains(text, `"executed":true`) || strings.Contains(text, `"response":`) || strings.Contains(text, "first evidence") {
		t.Fatalf("timing event leaked response or omitted command details: %s", text)
	}
}

func TestAskTelemetryMeasuresTimeToFirstOutputWithoutLoggingChunks(t *testing.T) {
	start := time.Date(2026, 9, 16, 8, 0, 0, 0, time.UTC)
	telemetry := newAskTelemetry(start)
	telemetry.beginLLM(1, start)
	telemetry.recordChunk(fantasy.StreamPart{Type: fantasy.StreamPartTypeTextStart}, start.Add(10*time.Millisecond))
	telemetry.recordChunk(fantasy.StreamPart{Type: fantasy.StreamPartTypeTextDelta, Delta: "secret response"}, start.Add(20*time.Millisecond))
	event, ok := telemetry.finishLLM(start.Add(50*time.Millisecond), fantasy.Usage{InputTokens: 100, OutputTokens: 10}, fantasy.FinishReasonStop)
	if !ok {
		t.Fatal("missing LLM timing event")
	}
	assertMilliseconds(t, "LLM request", event.DurationMS, 50)
	assertMilliseconds(t, "LLM first chunk", event.TimeToFirstChunkMS, 10)
	assertMilliseconds(t, "LLM first output", event.TimeToFirstOutputMS, 10)
	assertMilliseconds(t, "LLM streaming", event.StreamingAfterFirstOutputMS, 40)
	if event.Chunks != 2 || event.DeltaBytes != len("secret response") || event.OutputTokensPerSecond != 250 {
		t.Fatalf("event=%+v", event)
	}
	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "secret response") {
		t.Fatalf("LLM timing leaked streamed content: %s", encoded)
	}
	performance := telemetry.performance(start.Add(60 * time.Millisecond))
	assertMilliseconds(t, "LLM requests", performance.LLMRequestDurationMS, 50)
	assertMilliseconds(t, "LLM cumulative first output", performance.LLMTimeToFirstOutputMS, 10)
	if performance.LLMRequests != 1 || performance.LLMChunks != 2 || performance.LLMDeltaBytes != len("secret response") || performance.LLMOutputTokens != 10 {
		t.Fatalf("performance=%+v", performance)
	}
}

func assertMilliseconds(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 0.0001 {
		t.Fatalf("%s duration=%f want %f", name, got, want)
	}
}
