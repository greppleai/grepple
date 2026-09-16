package metrics

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// RunStartData identifies one controlled unit of work without storing task content.
type RunStartData struct {
	TaskID         string `json:"taskId"`
	Repository     string `json:"repository"`
	Revision       string `json:"revision"`
	AssignedCohort string `json:"assignedCohort"`
}

// AssistantData records provider-reported usage for one assistant response.
type AssistantData struct {
	Turn          int    `json:"turn"`
	Provider      string `json:"provider"`
	Model         string `json:"model"`
	ThinkingLevel string `json:"thinkingLevel"`
	Usage         Usage  `json:"usage"`
}

// ToolCallData records a normalized, content-free tool invocation.
type ToolCallData struct {
	CallID        string `json:"callId"`
	Tool          string `json:"tool"`
	Category      string `json:"category"`
	ArgumentShape string `json:"argumentShape"`
	ResourceID    string `json:"resourceId"`
	Offset        *int   `json:"offset"`
	Limit         *int   `json:"limit"`
	Turn          int    `json:"turn"`
	GreppleMode   string `json:"greppleMode"`
}

// ToolResultData records normalized outcome and volume evidence without tool output.
type ToolResultData struct {
	CallID            string `json:"callId"`
	Success           bool   `json:"success"`
	Bytes             int64  `json:"bytes"`
	Lines             int64  `json:"lines"`
	ZeroResult        bool   `json:"zeroResult"`
	AddedLines        *int   `json:"addedLines"`
	RemovedLines      *int   `json:"removedLines"`
	BeforeFingerprint string `json:"beforeFingerprint"`
	AfterFingerprint  string `json:"afterFingerprint"`
	TestOutcome       string `json:"testOutcome"`
}

// CommandData records one Grepple CLI invocation observed by Grepple itself.
type CommandData struct {
	Name        string `json:"name"`
	Success     bool   `json:"success"`
	DurationMS  int64  `json:"durationMs"`
	GreppleMode string `json:"greppleMode"`
}

// RunEndData records explicit evaluation of a completed run.
type RunEndData struct {
	Outcome            string   `json:"outcome"`
	HumanInterventions *int     `json:"humanInterventions"`
	EvaluatorScore     *float64 `json:"evaluatorScore"`
	Rubric             string   `json:"rubric"`
	Regressions        *int     `json:"regressions"`
	FirstEditSurvived  *bool    `json:"firstEditSurvived"`
}

// DecodeEventData validates and decodes the discriminated payload for an event type.
func DecodeEventData(eventType string, data json.RawMessage) (any, error) {
	var target any
	switch eventType {
	case EventRunStart:
		target = &RunStartData{}
	case EventAssistant:
		target = &AssistantData{}
	case EventToolCall:
		target = &ToolCallData{}
	case EventToolResult:
		target = &ToolResultData{}
	case EventCommand:
		target = &CommandData{}
	case EventCompaction:
		target = &struct{}{}
	case EventRunEnd:
		target = &RunEndData{}
	default:
		return nil, fmt.Errorf("unsupported metrics event %q", eventType)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return nil, fmt.Errorf("decode %s data: %w", eventType, err)
	}
	if err := ensureJSONEnd(decoder); err != nil {
		return nil, fmt.Errorf("decode %s data: %w", eventType, err)
	}
	if err := validateEventData(eventType, target); err != nil {
		return nil, err
	}
	return target, nil
}

func ensureJSONEnd(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func validateEventData(eventType string, value any) error {
	switch data := value.(type) {
	case *RunStartData:
		if !validLabel(data.TaskID, 256) || !validLabel(data.AssignedCohort, 128) {
			return errors.New("run_start requires bounded taskId and assignedCohort labels")
		}
		if (data.Repository != "" && !validLabel(data.Repository, 256)) || (data.Revision != "" && !validLabel(data.Revision, 256)) {
			return errors.New("run_start repository and revision must be bounded labels")
		}
	case *AssistantData:
		if data.Turn < 1 {
			return errors.New("assistant turn must be positive")
		}
		if data.Usage.Input < 0 || data.Usage.Output < 0 || data.Usage.CacheRead < 0 || data.Usage.CacheWrite < 0 || data.Usage.TotalTokens < 0 || data.Usage.Cost < 0 {
			return errors.New("assistant usage must be non-negative")
		}
		if (data.Provider != "" && !validLabel(data.Provider, 128)) || (data.Model != "" && !validLabel(data.Model, 256)) || (data.ThinkingLevel != "" && !validLabel(data.ThinkingLevel, 64)) {
			return errors.New("assistant provider, model, and thinkingLevel must be bounded labels")
		}
	case *ToolCallData:
		if !validOpaqueToken(data.CallID) || !validOpaqueToken(data.Tool) || data.Turn < 1 {
			return errors.New("tool_call requires opaque callId and tool identifiers and a positive turn")
		}
		categories := map[string]bool{"navigation": true, "read": true, "mutation": true, "test": true, "verification": true, "ambiguous": true}
		if !categories[data.Category] {
			return errors.New("tool_call category must be navigation, read, mutation, test, verification, or ambiguous")
		}
		if data.ArgumentShape != "" && !validOpaqueToken(data.ArgumentShape) {
			return errors.New("tool_call argumentShape must be an opaque identifier")
		}
		if data.ResourceID != "" && !validOpaqueToken(data.ResourceID) {
			return errors.New("tool_call resourceId must be an opaque identifier")
		}
		if data.Offset != nil && *data.Offset < 0 {
			return errors.New("tool_call offset must be non-negative")
		}
		if data.Limit != nil && *data.Limit < 1 {
			return errors.New("tool_call limit must be positive")
		}
	case *ToolResultData:
		if !validOpaqueToken(data.CallID) || data.Bytes < 0 || data.Lines < 0 {
			return errors.New("tool_result requires opaque callId and non-negative volume")
		}
		if (data.AddedLines != nil && *data.AddedLines < 0) || (data.RemovedLines != nil && *data.RemovedLines < 0) {
			return errors.New("tool_result edit counts must be non-negative")
		}
		if (data.BeforeFingerprint != "" && !validOpaqueToken(data.BeforeFingerprint)) || (data.AfterFingerprint != "" && !validOpaqueToken(data.AfterFingerprint)) {
			return errors.New("tool_result fingerprints must be opaque identifiers")
		}
		if data.TestOutcome != "" && data.TestOutcome != "pass" && data.TestOutcome != "fail" && data.TestOutcome != "unknown" {
			return errors.New("tool_result testOutcome must be pass, fail, unknown, or empty")
		}
	case *CommandData:
		if !validOpaqueToken(data.Name) || data.DurationMS < 0 || (data.GreppleMode != "" && !validOpaqueToken(data.GreppleMode)) {
			return errors.New("command requires opaque name/mode identifiers and non-negative duration")
		}
	case *RunEndData:
		if data.Outcome != "success" && data.Outcome != "failure" && data.Outcome != "abandoned" && data.Outcome != "unknown" {
			return errors.New("run_end outcome must be success, failure, abandoned, or unknown")
		}
		if (data.HumanInterventions != nil && *data.HumanInterventions < 0) || (data.Regressions != nil && *data.Regressions < 0) {
			return errors.New("run_end counts must be non-negative")
		}
		if data.Rubric != "" && !validOpaqueToken(data.Rubric) {
			return errors.New("run_end rubric must be an opaque identifier")
		}
	}
	return nil
}

func validOpaqueToken(value string) bool {
	if len(value) == 0 || len(value) > 256 {
		return false
	}
	for index := 0; index < len(value); index++ {
		character := value[index]
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') {
			continue
		}
		switch character {
		case '.', '_', '-', ':', '@', '+':
			continue
		default:
			return false
		}
	}
	return true
}

func validLabel(value string, maximum int) bool {
	if strings.TrimSpace(value) == "" || len(value) > maximum {
		return false
	}
	return !strings.ContainsAny(value, "\x00\r\n\t")
}
