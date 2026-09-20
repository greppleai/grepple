package agent

import (
	"encoding/json"
	"sort"
	"sync"
	"time"

	"charm.land/fantasy"
)

const performanceSchema = "grepple-ask-performance-v1"

// Telemetry accumulates agent stream and tool execution measurements.
type Telemetry struct {
	mu          sync.Mutex
	started     time.Time
	streamStart time.Time
	streamEnd   time.Time
	tools       []*ToolMeasurement
	llms        []*llmMeasurement
	currentLLM  *llmMeasurement
}

// CacheStatus describes how a tool call used the research-session cache.
type CacheStatus struct {
	Tool     string `json:"tool"`
	Key      string `json:"key"`
	Hit      bool   `json:"hit"`
	Shared   bool   `json:"shared"`
	Executed bool   `json:"-"`
}

// ToolMeasurement tracks one in-flight tool invocation.
type ToolMeasurement struct {
	tool             string
	callID           string
	input            json.RawMessage
	started          time.Time
	ended            time.Time
	executionStarted time.Time
	executionEnded   time.Time
	responseBytes    int
	cache            CacheStatus
	failed           bool
}

// ToolTimingEvent is the persisted timing projection for one tool call.
type ToolTimingEvent struct {
	Tool                string          `json:"tool"`
	CallID              string          `json:"callId,omitempty"`
	Input               json.RawMessage `json:"input"`
	StartedMS           float64         `json:"startedMs"`
	DurationMS          float64         `json:"durationMs"`
	ExecutionDurationMS float64         `json:"executionDurationMs"`
	ResponseBytes       int             `json:"responseBytes"`
	CacheHit            bool            `json:"cacheHit"`
	CacheShared         bool            `json:"cacheShared"`
	CacheKey            string          `json:"cacheKey,omitempty"`
	Executed            bool            `json:"executed"`
	Failed              bool            `json:"failed"`
}

type llmMeasurement struct {
	step        int
	started     time.Time
	firstChunk  time.Time
	firstOutput time.Time
	ended       time.Time
	chunks      map[string]int
	deltaBytes  int
	usage       fantasy.Usage
	reason      fantasy.FinishReason
}

type llmTimingEvent struct {
	Step                        int                  `json:"step"`
	StartedMS                   float64              `json:"startedMs"`
	DurationMS                  float64              `json:"durationMs"`
	TimeToFirstChunkMS          float64              `json:"timeToFirstChunkMs"`
	TimeToFirstOutputMS         float64              `json:"timeToFirstOutputMs"`
	StreamingAfterFirstOutputMS float64              `json:"streamingAfterFirstOutputMs"`
	Chunks                      int                  `json:"chunks"`
	ChunksByType                map[string]int       `json:"chunksByType"`
	DeltaBytes                  int                  `json:"deltaBytes"`
	InputTokens                 int64                `json:"inputTokens"`
	OutputTokens                int64                `json:"outputTokens"`
	OutputTokensPerSecond       float64              `json:"outputTokensPerSecond"`
	FinishReason                fantasy.FinishReason `json:"finishReason"`
}

// Performance summarizes one complete agent session.
type Performance struct {
	Schema                            string                   `json:"schema"`
	TotalDurationMS                   float64                  `json:"totalDurationMs"`
	StreamDurationMS                  float64                  `json:"streamDurationMs"`
	LLMDurationMS                     float64                  `json:"llmDurationMs"`
	NonToolWallDurationMS             float64                  `json:"nonToolWallDurationMs"`
	LLMRequestDurationMS              float64                  `json:"llmRequestDurationMs"`
	LLMTimeToFirstChunkMS             float64                  `json:"llmTimeToFirstChunkMs"`
	LLMTimeToFirstOutputMS            float64                  `json:"llmTimeToFirstOutputMs"`
	LLMStreamingAfterFirstOutputMS    float64                  `json:"llmStreamingAfterFirstOutputMs"`
	LLMRequests                       int                      `json:"llmRequests"`
	LLMChunks                         int                      `json:"llmChunks"`
	LLMDeltaBytes                     int                      `json:"llmDeltaBytes"`
	LLMOutputTokens                   int64                    `json:"llmOutputTokens"`
	ToolWallDurationMS                float64                  `json:"toolWallDurationMs"`
	ToolCumulativeDurationMS          float64                  `json:"toolCumulativeDurationMs"`
	ToolExecutionWallDurationMS       float64                  `json:"toolExecutionWallDurationMs"`
	ToolExecutionCumulativeDurationMS float64                  `json:"toolExecutionCumulativeDurationMs"`
	OtherDurationMS                   float64                  `json:"otherDurationMs"`
	ToolCalls                         int                      `json:"toolCalls"`
	ToolExecutions                    int                      `json:"toolExecutions"`
	Tools                             []ToolPerformanceSummary `json:"tools,omitempty"`
}

// ToolPerformanceSummary aggregates measurements for one tool name.
type ToolPerformanceSummary struct {
	Tool                string  `json:"tool"`
	Calls               int     `json:"calls"`
	Executions          int     `json:"executions"`
	CacheHits           int     `json:"cacheHits"`
	SharedCalls         int     `json:"sharedCalls"`
	Failures            int     `json:"failures"`
	DurationMS          float64 `json:"durationMs"`
	ExecutionDurationMS float64 `json:"executionDurationMs"`
	MaxDurationMS       float64 `json:"maxDurationMs"`
	ResponseBytes       int     `json:"responseBytes"`
}

// NewTelemetry starts a telemetry accumulator at started.
func NewTelemetry(started time.Time) *Telemetry {
	return &Telemetry{started: started}
}

func (telemetry *Telemetry) startStream(now time.Time) {
	telemetry.mu.Lock()
	defer telemetry.mu.Unlock()
	telemetry.streamStart = now
	telemetry.streamEnd = time.Time{}
}

func (telemetry *Telemetry) finishStream(now time.Time) {
	telemetry.mu.Lock()
	defer telemetry.mu.Unlock()
	telemetry.streamEnd = now
}

func (telemetry *Telemetry) beginLLM(step int, now time.Time) {
	telemetry.mu.Lock()
	defer telemetry.mu.Unlock()
	measurement := &llmMeasurement{step: step, started: now, chunks: make(map[string]int)}
	telemetry.llms = append(telemetry.llms, measurement)
	telemetry.currentLLM = measurement
}

func (telemetry *Telemetry) recordChunk(part fantasy.StreamPart, now time.Time) {
	telemetry.mu.Lock()
	defer telemetry.mu.Unlock()
	measurement := telemetry.currentLLM
	if measurement == nil {
		return
	}
	if measurement.firstChunk.IsZero() {
		measurement.firstChunk = now
	}
	if measurement.firstOutput.IsZero() && isOutputStreamPart(part.Type) {
		measurement.firstOutput = now
	}
	measurement.chunks[string(part.Type)]++
	measurement.deltaBytes += len(part.Delta)
}

func (telemetry *Telemetry) finishLLM(now time.Time, usage fantasy.Usage, reason fantasy.FinishReason) (llmTimingEvent, bool) {
	telemetry.mu.Lock()
	defer telemetry.mu.Unlock()
	measurement := telemetry.currentLLM
	if measurement == nil {
		return llmTimingEvent{}, false
	}
	measurement.ended = now
	measurement.usage = usage
	measurement.reason = reason
	telemetry.currentLLM = nil
	return telemetry.llmTimingEvent(measurement), true
}

func (telemetry *Telemetry) llmTimingEvent(measurement *llmMeasurement) llmTimingEvent {
	duration := nonNegativeDuration(measurement.ended.Sub(measurement.started))
	timeToFirstChunk := durationUntil(measurement.started, measurement.firstChunk)
	timeToFirstOutput := durationUntil(measurement.started, measurement.firstOutput)
	streamingAfterOutput := durationUntil(measurement.firstOutput, measurement.ended)
	chunks := 0
	for _, count := range measurement.chunks {
		chunks += count
	}
	outputTokensPerSecond := 0.0
	if streamingAfterOutput > 0 {
		outputTokensPerSecond = float64(measurement.usage.OutputTokens) / streamingAfterOutput.Seconds()
	}
	return llmTimingEvent{
		Step: measurement.step, StartedMS: milliseconds(measurement.started.Sub(telemetry.started)), DurationMS: milliseconds(duration),
		TimeToFirstChunkMS: milliseconds(timeToFirstChunk), TimeToFirstOutputMS: milliseconds(timeToFirstOutput), StreamingAfterFirstOutputMS: milliseconds(streamingAfterOutput),
		Chunks: chunks, ChunksByType: cloneChunkCounts(measurement.chunks), DeltaBytes: measurement.deltaBytes,
		InputTokens: measurement.usage.InputTokens, OutputTokens: measurement.usage.OutputTokens, OutputTokensPerSecond: outputTokensPerSecond, FinishReason: measurement.reason,
	}
}

func isOutputStreamPart(partType fantasy.StreamPartType) bool {
	switch partType {
	case fantasy.StreamPartTypeTextStart, fantasy.StreamPartTypeTextDelta, fantasy.StreamPartTypeReasoningStart, fantasy.StreamPartTypeReasoningDelta, fantasy.StreamPartTypeToolInputStart, fantasy.StreamPartTypeToolInputDelta, fantasy.StreamPartTypeToolCall:
		return true
	default:
		return false
	}
}

func cloneChunkCounts(source map[string]int) map[string]int {
	result := make(map[string]int, len(source))
	for kind, count := range source {
		result[kind] = count
	}
	return result
}

// BeginTool starts measuring one tool invocation.
func (telemetry *Telemetry) BeginTool(tool string, input any, now time.Time, callID ...string) *ToolMeasurement {
	encoded, err := json.Marshal(input)
	if err != nil {
		encoded = []byte("null")
	}
	measurement := &ToolMeasurement{tool: tool, input: encoded, started: now}
	if len(callID) > 0 {
		measurement.callID = callID[0]
	}
	telemetry.mu.Lock()
	telemetry.tools = append(telemetry.tools, measurement)
	telemetry.mu.Unlock()
	return measurement
}

// StartToolExecution marks the beginning of uncached tool work.
func (telemetry *Telemetry) StartToolExecution(measurement *ToolMeasurement, now time.Time) {
	telemetry.mu.Lock()
	defer telemetry.mu.Unlock()
	measurement.executionStarted = now
}

// FinishTool completes a tool measurement and returns its log event.
func (telemetry *Telemetry) FinishTool(measurement *ToolMeasurement, now time.Time, response fantasy.ToolResponse, runErr error, cache CacheStatus) ToolTimingEvent {
	telemetry.mu.Lock()
	defer telemetry.mu.Unlock()
	measurement.ended = now
	measurement.responseBytes = len(response.Content) + len(response.Data)
	measurement.cache = cache
	if cache.Executed {
		measurement.executionEnded = now
	}
	measurement.failed = runErr != nil || response.IsError
	return ToolTimingEvent{
		Tool: measurement.tool, CallID: measurement.callID, Input: measurement.input,
		StartedMS: milliseconds(measurement.started.Sub(telemetry.started)), DurationMS: milliseconds(measurement.ended.Sub(measurement.started)),
		ExecutionDurationMS: milliseconds(nonNegativeDuration(measurement.executionEnded.Sub(measurement.executionStarted))),
		ResponseBytes:       measurement.responseBytes, CacheHit: cache.Hit, CacheShared: cache.Shared, CacheKey: cache.Key, Executed: cache.Executed, Failed: measurement.failed,
	}
}

// Performance returns the current aggregate session measurements.
func (telemetry *Telemetry) Performance(now time.Time) Performance {
	telemetry.mu.Lock()
	defer telemetry.mu.Unlock()
	streamEnd := telemetry.streamEnd
	if !telemetry.streamStart.IsZero() && streamEnd.IsZero() {
		streamEnd = now
	}
	streamDuration := nonNegativeDuration(streamEnd.Sub(telemetry.streamStart))
	if telemetry.streamStart.IsZero() {
		streamDuration = 0
	}
	accumulator := newPerformanceAccumulator(len(telemetry.tools))
	for _, tool := range telemetry.tools {
		accumulator.add(tool, now, telemetry.streamStart, streamEnd)
	}
	toolWall := mergedDuration(accumulator.intervals)
	executionWall := mergedDuration(accumulator.executionIntervals)
	llmDuration := nonNegativeDuration(streamDuration - toolWall)
	llm := summarizeLLMPerformance(telemetry.llms, now)
	totalDuration := nonNegativeDuration(now.Sub(telemetry.started))
	otherDuration := nonNegativeDuration(totalDuration - streamDuration)
	tools := make([]ToolPerformanceSummary, 0, len(accumulator.summaries))
	for _, summary := range accumulator.summaries {
		tools = append(tools, *summary)
	}
	sort.Slice(tools, func(i, j int) bool { return tools[i].Tool < tools[j].Tool })
	return Performance{
		Schema: performanceSchema, TotalDurationMS: milliseconds(totalDuration), StreamDurationMS: milliseconds(streamDuration),
		LLMDurationMS: milliseconds(llm.duration), NonToolWallDurationMS: milliseconds(llmDuration), LLMRequestDurationMS: milliseconds(llm.duration),
		LLMTimeToFirstChunkMS: milliseconds(llm.timeToFirstChunk), LLMTimeToFirstOutputMS: milliseconds(llm.timeToFirstOutput),
		LLMStreamingAfterFirstOutputMS: milliseconds(llm.streamingAfterFirstOutput), LLMRequests: len(telemetry.llms), LLMChunks: llm.chunks, LLMDeltaBytes: llm.deltaBytes, LLMOutputTokens: llm.outputTokens,
		ToolWallDurationMS: milliseconds(toolWall), ToolCumulativeDurationMS: milliseconds(accumulator.cumulative),
		ToolExecutionWallDurationMS: milliseconds(executionWall), ToolExecutionCumulativeDurationMS: milliseconds(accumulator.executionCumulative),
		OtherDurationMS: milliseconds(otherDuration), ToolCalls: len(telemetry.tools), ToolExecutions: accumulator.executions, Tools: tools,
	}
}

type llmPerformanceSummary struct {
	duration                  time.Duration
	timeToFirstChunk          time.Duration
	timeToFirstOutput         time.Duration
	streamingAfterFirstOutput time.Duration
	chunks                    int
	deltaBytes                int
	outputTokens              int64
}

func summarizeLLMPerformance(measurements []*llmMeasurement, now time.Time) llmPerformanceSummary {
	var summary llmPerformanceSummary
	for _, measurement := range measurements {
		end := measurement.ended
		if end.IsZero() {
			end = now
		}
		summary.duration += nonNegativeDuration(end.Sub(measurement.started))
		summary.timeToFirstChunk += durationUntil(measurement.started, measurement.firstChunk)
		summary.timeToFirstOutput += durationUntil(measurement.started, measurement.firstOutput)
		summary.streamingAfterFirstOutput += durationUntil(measurement.firstOutput, end)
		for _, count := range measurement.chunks {
			summary.chunks += count
		}
		summary.deltaBytes += measurement.deltaBytes
		summary.outputTokens += measurement.usage.OutputTokens
	}
	return summary
}

type performanceAccumulator struct {
	intervals           []timeInterval
	executionIntervals  []timeInterval
	summaries           map[string]*ToolPerformanceSummary
	cumulative          time.Duration
	executionCumulative time.Duration
	executions          int
}

func newPerformanceAccumulator(capacity int) *performanceAccumulator {
	return &performanceAccumulator{
		intervals: make([]timeInterval, 0, capacity), executionIntervals: make([]timeInterval, 0, capacity),
		summaries: make(map[string]*ToolPerformanceSummary),
	}
}

func (accumulator *performanceAccumulator) add(tool *ToolMeasurement, now, streamStart, streamEnd time.Time) {
	end := tool.ended
	if end.IsZero() {
		end = now
	}
	duration := nonNegativeDuration(end.Sub(tool.started))
	accumulator.cumulative += duration
	accumulator.addSummary(tool, duration)
	accumulator.addStreamIntervals(tool, end, streamStart, streamEnd)
}

func (accumulator *performanceAccumulator) addSummary(tool *ToolMeasurement, duration time.Duration) {
	summary := accumulator.summaries[tool.tool]
	if summary == nil {
		summary = &ToolPerformanceSummary{Tool: tool.tool}
		accumulator.summaries[tool.tool] = summary
	}
	summary.Calls++
	if tool.cache.Executed {
		accumulator.executions++
		summary.Executions++
		executionDuration := nonNegativeDuration(tool.executionEnded.Sub(tool.executionStarted))
		accumulator.executionCumulative += executionDuration
		summary.ExecutionDurationMS += milliseconds(executionDuration)
	}
	if tool.cache.Hit {
		summary.CacheHits++
	}
	if tool.cache.Shared {
		summary.SharedCalls++
	}
	if tool.failed {
		summary.Failures++
	}
	durationMS := milliseconds(duration)
	summary.DurationMS += durationMS
	if durationMS > summary.MaxDurationMS {
		summary.MaxDurationMS = durationMS
	}
	summary.ResponseBytes += tool.responseBytes
}

func (accumulator *performanceAccumulator) addStreamIntervals(tool *ToolMeasurement, end, streamStart, streamEnd time.Time) {
	if streamStart.IsZero() {
		return
	}
	if interval, ok := clippedInterval(tool.started, end, streamStart, streamEnd); ok {
		accumulator.intervals = append(accumulator.intervals, interval)
	}
	if !tool.cache.Executed {
		return
	}
	if interval, ok := clippedInterval(tool.executionStarted, tool.executionEnded, streamStart, streamEnd); ok {
		accumulator.executionIntervals = append(accumulator.executionIntervals, interval)
	}
}

func clippedInterval(start, end, lower, upper time.Time) (timeInterval, bool) {
	start = maxTime(start, lower)
	end = minTime(end, upper)
	return timeInterval{start: start, end: end}, end.After(start)
}

type timeInterval struct {
	start time.Time
	end   time.Time
}

func mergedDuration(intervals []timeInterval) time.Duration {
	if len(intervals) == 0 {
		return 0
	}
	sort.Slice(intervals, func(i, j int) bool { return intervals[i].start.Before(intervals[j].start) })
	start, end := intervals[0].start, intervals[0].end
	var total time.Duration
	for _, interval := range intervals[1:] {
		if !interval.start.After(end) {
			if interval.end.After(end) {
				end = interval.end
			}
			continue
		}
		total += end.Sub(start)
		start, end = interval.start, interval.end
	}
	return total + end.Sub(start)
}

func milliseconds(duration time.Duration) float64 {
	return float64(duration) / float64(time.Millisecond)
}

func durationUntil(start, end time.Time) time.Duration {
	if start.IsZero() || end.IsZero() {
		return 0
	}
	return nonNegativeDuration(end.Sub(start))
}

func nonNegativeDuration(duration time.Duration) time.Duration {
	if duration < 0 {
		return 0
	}
	return duration
}

func minTime(left, right time.Time) time.Time {
	if left.Before(right) {
		return left
	}
	return right
}

func maxTime(left, right time.Time) time.Time {
	if left.After(right) {
		return left
	}
	return right
}
