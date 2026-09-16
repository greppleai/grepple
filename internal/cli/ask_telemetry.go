package cli

import (
	"encoding/json"
	"sort"
	"sync"
	"time"

	"charm.land/fantasy"
)

const askPerformanceSchema = "grepple-ask-performance-v1"

type askTelemetry struct {
	mu          sync.Mutex
	started     time.Time
	streamStart time.Time
	streamEnd   time.Time
	tools       []*askToolMeasurement
}

type askToolMeasurement struct {
	tool             string
	callID           string
	input            json.RawMessage
	started          time.Time
	ended            time.Time
	executionStarted time.Time
	executionEnded   time.Time
	responseBytes    int
	cache            researchCacheStatus
	failed           bool
}

type askToolTimingEvent struct {
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

type askPerformance struct {
	Schema                            string                      `json:"schema"`
	TotalDurationMS                   float64                     `json:"totalDurationMs"`
	StreamDurationMS                  float64                     `json:"streamDurationMs"`
	LLMDurationMS                     float64                     `json:"llmDurationMs"`
	ToolWallDurationMS                float64                     `json:"toolWallDurationMs"`
	ToolCumulativeDurationMS          float64                     `json:"toolCumulativeDurationMs"`
	ToolExecutionWallDurationMS       float64                     `json:"toolExecutionWallDurationMs"`
	ToolExecutionCumulativeDurationMS float64                     `json:"toolExecutionCumulativeDurationMs"`
	OtherDurationMS                   float64                     `json:"otherDurationMs"`
	ToolCalls                         int                         `json:"toolCalls"`
	ToolExecutions                    int                         `json:"toolExecutions"`
	Tools                             []askToolPerformanceSummary `json:"tools,omitempty"`
}

type askToolPerformanceSummary struct {
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

func newAskTelemetry(started time.Time) *askTelemetry {
	return &askTelemetry{started: started}
}

func (telemetry *askTelemetry) startStream(now time.Time) {
	telemetry.mu.Lock()
	defer telemetry.mu.Unlock()
	telemetry.streamStart = now
	telemetry.streamEnd = time.Time{}
}

func (telemetry *askTelemetry) finishStream(now time.Time) {
	telemetry.mu.Lock()
	defer telemetry.mu.Unlock()
	telemetry.streamEnd = now
}

func (telemetry *askTelemetry) beginTool(tool string, input any, now time.Time, callID ...string) *askToolMeasurement {
	encoded, err := json.Marshal(input)
	if err != nil {
		encoded = []byte("null")
	}
	measurement := &askToolMeasurement{tool: tool, input: encoded, started: now}
	if len(callID) > 0 {
		measurement.callID = callID[0]
	}
	telemetry.mu.Lock()
	telemetry.tools = append(telemetry.tools, measurement)
	telemetry.mu.Unlock()
	return measurement
}

func (telemetry *askTelemetry) startToolExecution(measurement *askToolMeasurement, now time.Time) {
	telemetry.mu.Lock()
	defer telemetry.mu.Unlock()
	measurement.executionStarted = now
}

func (telemetry *askTelemetry) finishTool(measurement *askToolMeasurement, now time.Time, response fantasy.ToolResponse, runErr error, cache researchCacheStatus) askToolTimingEvent {
	telemetry.mu.Lock()
	defer telemetry.mu.Unlock()
	measurement.ended = now
	measurement.responseBytes = len(response.Content) + len(response.Data)
	measurement.cache = cache
	if cache.Executed {
		measurement.executionEnded = now
	}
	measurement.failed = runErr != nil || response.IsError
	return askToolTimingEvent{
		Tool: measurement.tool, CallID: measurement.callID, Input: measurement.input,
		StartedMS: milliseconds(measurement.started.Sub(telemetry.started)), DurationMS: milliseconds(measurement.ended.Sub(measurement.started)),
		ExecutionDurationMS: milliseconds(nonNegativeDuration(measurement.executionEnded.Sub(measurement.executionStarted))),
		ResponseBytes:       measurement.responseBytes, CacheHit: cache.Hit, CacheShared: cache.Shared, CacheKey: cache.Key, Executed: cache.Executed, Failed: measurement.failed,
	}
}

func (telemetry *askTelemetry) performance(now time.Time) askPerformance {
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
	accumulator := newAskPerformanceAccumulator(len(telemetry.tools))
	for _, tool := range telemetry.tools {
		accumulator.add(tool, now, telemetry.streamStart, streamEnd)
	}
	toolWall := mergedDuration(accumulator.intervals)
	executionWall := mergedDuration(accumulator.executionIntervals)
	llmDuration := nonNegativeDuration(streamDuration - toolWall)
	totalDuration := nonNegativeDuration(now.Sub(telemetry.started))
	otherDuration := nonNegativeDuration(totalDuration - streamDuration)
	tools := make([]askToolPerformanceSummary, 0, len(accumulator.summaries))
	for _, summary := range accumulator.summaries {
		tools = append(tools, *summary)
	}
	sort.Slice(tools, func(i, j int) bool { return tools[i].Tool < tools[j].Tool })
	return askPerformance{
		Schema: askPerformanceSchema, TotalDurationMS: milliseconds(totalDuration), StreamDurationMS: milliseconds(streamDuration),
		LLMDurationMS: milliseconds(llmDuration), ToolWallDurationMS: milliseconds(toolWall), ToolCumulativeDurationMS: milliseconds(accumulator.cumulative),
		ToolExecutionWallDurationMS: milliseconds(executionWall), ToolExecutionCumulativeDurationMS: milliseconds(accumulator.executionCumulative),
		OtherDurationMS: milliseconds(otherDuration), ToolCalls: len(telemetry.tools), ToolExecutions: accumulator.executions, Tools: tools,
	}
}

type askPerformanceAccumulator struct {
	intervals           []timeInterval
	executionIntervals  []timeInterval
	summaries           map[string]*askToolPerformanceSummary
	cumulative          time.Duration
	executionCumulative time.Duration
	executions          int
}

func newAskPerformanceAccumulator(capacity int) *askPerformanceAccumulator {
	return &askPerformanceAccumulator{
		intervals: make([]timeInterval, 0, capacity), executionIntervals: make([]timeInterval, 0, capacity),
		summaries: make(map[string]*askToolPerformanceSummary),
	}
}

func (accumulator *askPerformanceAccumulator) add(tool *askToolMeasurement, now, streamStart, streamEnd time.Time) {
	end := tool.ended
	if end.IsZero() {
		end = now
	}
	duration := nonNegativeDuration(end.Sub(tool.started))
	accumulator.cumulative += duration
	accumulator.addSummary(tool, duration)
	accumulator.addStreamIntervals(tool, end, streamStart, streamEnd)
}

func (accumulator *askPerformanceAccumulator) addSummary(tool *askToolMeasurement, duration time.Duration) {
	summary := accumulator.summaries[tool.tool]
	if summary == nil {
		summary = &askToolPerformanceSummary{Tool: tool.tool}
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

func (accumulator *askPerformanceAccumulator) addStreamIntervals(tool *askToolMeasurement, end, streamStart, streamEnd time.Time) {
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
