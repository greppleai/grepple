package aiprovider

import "testing"

func TestResolveSelection(t *testing.T) {
	tests := []struct {
		name, provider, model, wantProvider, wantModel string
		wantError                                      bool
	}{
		{name: "defaults", wantProvider: "codex"},
		{name: "provider default model", provider: "copilot", wantProvider: "copilot"},
		{name: "model", model: "gpt-5.1", wantProvider: "codex", wantModel: "gpt-5.1"},
		{name: "prefixed model", model: "copilot/gpt-4.1", wantProvider: "copilot", wantModel: "gpt-4.1"},
		{name: "matching provider", provider: "anthropic", model: "anthropic/claude", wantProvider: "anthropic", wantModel: "claude"},
		{name: "trimmed", provider: " Anthropic ", model: " anthropic/claude ", wantProvider: "anthropic", wantModel: "claude"},
		{name: "conflict", provider: "openai", model: "anthropic/claude", wantError: true},
		{name: "missing model", model: "copilot/", wantError: true},
		{name: "missing provider", model: "/claude", wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider, model, err := ResolveSelection(test.provider, test.model)
			if (err != nil) != test.wantError || provider != test.wantProvider || model != test.wantModel {
				t.Fatalf("provider=%q model=%q err=%v", provider, model, err)
			}
		})
	}
}
