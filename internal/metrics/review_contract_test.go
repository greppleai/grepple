package metrics

import (
	"strings"
	"testing"
)

func TestReviewedEventContractUsesAgentWithoutModelMetadata(t *testing.T) {
	decoded, err := DecodeEventData(EventRunStart, []byte(`{"agent":"pi"}`))
	if err != nil {
		t.Fatal(err)
	}
	start, ok := decoded.(*RunStartData)
	if !ok || start.Agent != "pi" {
		t.Fatalf("run_start = %#v", decoded)
	}
	if _, err := DecodeEventData(EventAssistant, []byte(`{"turn":1,"usage":{"input":1,"output":2,"cacheRead":0,"cacheWrite":0,"totalTokens":3,"cost":0.01}}`)); err != nil {
		t.Fatalf("agent-neutral assistant event: %v", err)
	}
}

func TestReviewedEventContractRejectsMissingAgentAndRemovedMetadata(t *testing.T) {
	tests := []struct {
		name      string
		eventType string
		data      string
	}{
		{name: "missing agent", eventType: EventRunStart, data: `{}`},
		{name: "oversized agent", eventType: EventRunStart, data: `{"agent":"` + strings.Repeat("a", 129) + `"}`},
		{name: "cohort metadata", eventType: EventRunStart, data: `{"agent":"pi","assignedCohort":"control"}`},
		{name: "task metadata", eventType: EventRunStart, data: `{"agent":"pi","taskId":"task"}`},
		{name: "repository metadata", eventType: EventRunStart, data: `{"agent":"pi","repository":"repo"}`},
		{name: "revision metadata", eventType: EventRunStart, data: `{"agent":"pi","revision":"abc"}`},
		{name: "model metadata", eventType: EventAssistant, data: `{"turn":1,"model":"opus","usage":{"totalTokens":1}}`},
		{name: "provider metadata", eventType: EventAssistant, data: `{"turn":1,"provider":"anthropic","usage":{"totalTokens":1}}`},
		{name: "thinking metadata", eventType: EventAssistant, data: `{"turn":1,"thinkingLevel":"high","usage":{"totalTokens":1}}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := DecodeEventData(test.eventType, []byte(test.data)); err == nil {
				t.Fatal("expected strict contract error")
			}
		})
	}
}
