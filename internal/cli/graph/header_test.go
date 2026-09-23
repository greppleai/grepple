package graph

import (
	"bytes"
	"strings"
	"testing"
)

func TestHumanBuildDisclosesHiddenCallsWithoutResolutionHeader(t *testing.T) {
	chdirTemp(t)
	writeGraphSource(t, ".", "main.go", "package sample\nfunc Run(){ external() }\n")
	text := captureStdout(t, func() {
		if err := Run([]string{"graph", "main.go"}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.HasPrefix(text, "graph files=1 declarations=1 calls=1 shown=0\n") || !strings.Contains(text, "func Run") || strings.Contains(text, "outcomes=") {
		t.Fatalf("human build must disclose the omitted unresolved call without routine resolution stats:\n%s", text)
	}
}

func TestHumanDiffWarnsWhenRecoveredSourcesRequireInspection(t *testing.T) {
	var text bytes.Buffer
	diff := DiffOutput{BeforeFiles: 1, AfterFiles: 1, BeforeSources: navigationSourceSummary{Recovered: 1}}
	if err := renderCompactGraphDiff(&text, diff, 0); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text.String(), "! incomplete before(discovered:0,selected:0,parsed:0,skipped:0,failed:0,recovered:1)") || strings.Contains(text.String(), "grepple-navigation-diff-v5") {
		t.Fatalf("recovered sources should remain visible without a schema header:\n%s", text.String())
	}
}
