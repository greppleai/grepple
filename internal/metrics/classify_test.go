package metrics

import (
	"encoding/json"
	"testing"
)

func TestClassifyCall(t *testing.T) {
	tests := []struct {
		name      string
		tool      string
		arguments string
		grepple   bool
		ambiguous bool
		test      bool
		mode      string
	}{
		{name: "namespaced read", tool: "functions.read", arguments: `{"path":"x.go"}`},
		{name: "absolute grepple", tool: "functions.bash", arguments: `{"command":"cd repo && /usr/local/bin/grepple graph callers Symbol"}`, grepple: true, mode: "graph"},
		{name: "env wrapper", tool: "bash", arguments: `{"command":"env COLOR=0 grepple -n symbol ."}`, grepple: true, mode: "search"},
		{name: "mention is ambiguous", tool: "bash", arguments: `{"command":"echo grepple"}`, ambiguous: true},
		{name: "go test", tool: "functions.bash", arguments: `{"command":"go test ./..."}`, test: true},
		{name: "test mention", tool: "bash", arguments: `{"command":"echo go test"}`},
		{name: "hidden script mutation", tool: "bash", arguments: `{"command":"python update.py"}`, ambiguous: true},
		{name: "shell redirection", tool: "bash", arguments: `{"command":"printf x > generated.go"}`, ambiguous: true},
		{name: "known test is not ambiguous", tool: "bash", arguments: `{"command":"npm run test:unit"}`, test: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			kind := classifyCall(test.tool, json.RawMessage(test.arguments))
			if kind.grepple != test.grepple || kind.ambiguous != test.ambiguous || kind.test != test.test || kind.mode != test.mode {
				t.Fatalf("kind = %#v", kind)
			}
		})
	}
}

func TestClassifyNamespacedMutation(t *testing.T) {
	kind := classifyCall("functions.edit", json.RawMessage(`{"path":"x.go"}`))
	if !kind.mutation {
		t.Fatalf("kind = %#v", kind)
	}
}
