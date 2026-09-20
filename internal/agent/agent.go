// Package agent provides reusable AI-agent execution, event logging, and telemetry.
package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"charm.land/fantasy"
)

// EventSink records semantic agent lifecycle events.
type EventSink interface {
	Record(eventType string, data any) error
}

// ModelFactory constructs the language model used by one agent invocation.
type ModelFactory func(context.Context) (fantasy.LanguageModel, error)

// Request describes one agent execution independently of CLI argument handling.
type Request struct {
	Name         string
	Prompt       string
	SystemPrompt string
	Provider     string
	Model        string
	ModelFactory ModelFactory
	Tools        []fantasy.AgentTool
	Telemetry    *Telemetry
}

// Result contains the reusable result of an agent execution.
type Result struct {
	Answer string
	Usage  fantasy.Usage
	Steps  int
}

// Run constructs and streams an agent while recording lifecycle events.
func Run(ctx context.Context, sink EventSink, request Request) (Result, error) {
	name := strings.TrimSpace(request.Name)
	if name == "" {
		name = "agent"
	}
	telemetry := request.Telemetry
	if telemetry == nil {
		telemetry = NewTelemetry(time.Now())
	}
	model, err := request.ModelFactory(ctx)
	if err != nil {
		return Result{}, recordError(sink, err)
	}
	runner := fantasy.NewAgent(model, fantasy.WithSystemPrompt(request.SystemPrompt), fantasy.WithTools(request.Tools...))
	telemetry.startStream(time.Now())
	streamed, err := runner.Stream(ctx, streamCall(sink, telemetry, request.Prompt))
	telemetry.finishStream(time.Now())
	if err != nil {
		return Result{}, recordError(sink, fmt.Errorf("%s %s/%s: %w", name, request.Provider, request.Model, err))
	}
	answer := strings.TrimSpace(streamed.Response.Content.Text())
	if answer == "" {
		return Result{}, recordError(sink, fmt.Errorf("%s returned no text answer", name))
	}
	result := Result{Answer: answer, Usage: streamed.TotalUsage, Steps: len(streamed.Steps)}
	if err := record(sink, "session.finish", map[string]any{"answer": answer, "usage": result.Usage, "steps": result.Steps}); err != nil {
		return Result{}, err
	}
	return result, nil
}

func streamCall(sink EventSink, telemetry *Telemetry, prompt string) fantasy.AgentStreamCall {
	return fantasy.AgentStreamCall{
		Prompt: prompt,
		OnStepStart: func(step int) error {
			telemetry.beginLLM(step, time.Now())
			return record(sink, "step.start", map[string]int{"step": step})
		},
		OnChunk: func(part fantasy.StreamPart) error {
			telemetry.recordChunk(part, time.Now())
			return nil
		},
		OnToolCall:    func(call fantasy.ToolCallContent) error { return record(sink, "tool.call", call) },
		OnToolResult:  func(result fantasy.ToolResultContent) error { return record(sink, "tool.result", result) },
		OnStepFinish:  func(result fantasy.StepResult) error { return record(sink, "step.finish", result) },
		OnAgentFinish: func(result *fantasy.AgentResult) error { return record(sink, "agent.finish", result) },
		OnError: func(err error) {
			if timing, ok := telemetry.finishLLM(time.Now(), fantasy.Usage{}, fantasy.FinishReasonError); ok {
				_ = record(sink, "llm.timing", timing)
			}
			_ = record(sink, "agent.error", map[string]string{"error": err.Error()})
		},
		OnStreamFinish: func(usage fantasy.Usage, reason fantasy.FinishReason, metadata fantasy.ProviderMetadata) error {
			streamErr := record(sink, "stream.finish", map[string]any{"usage": usage, "finishReason": reason, "providerMetadata": metadata})
			timing, ok := telemetry.finishLLM(time.Now(), usage, reason)
			if !ok {
				return streamErr
			}
			return errors.Join(streamErr, record(sink, "llm.timing", timing))
		},
	}
}

func recordError(sink EventSink, err error) error {
	return errors.Join(err, record(sink, "session.error", map[string]string{"error": err.Error()}))
}

func record(sink EventSink, eventType string, data any) error {
	if sink == nil {
		return nil
	}
	return sink.Record(eventType, data)
}
