package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type agentWorkflowBenchmark struct {
	name     string
	commands [][]string
	want     []string
}

func BenchmarkAgentWorkflows(b *testing.B) {
	root := writeAgentWorkflowBenchmarkFixture(b)
	previous, err := os.Getwd()
	if err != nil {
		b.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = os.Chdir(previous) })

	workflows := []agentWorkflowBenchmark{
		{name: "BreadthSummary", commands: [][]string{{"-F", "func ", "--count-summary", "service.go", "handler.go"}}, want: []string{"2 files\t5 matches"}},
		{name: "BroadAccidental", commands: [][]string{{"--line-only", "--no-anchors", "-F", "NOISE_MATCH", "noise.txt", "--limit", "0", "--max-segments", "1000"}}, want: []string{"noise.txt:2:", "grepple output truncated"}},
		{name: "OutlineDiscovery", commands: [][]string{{"--outline", "service.go", "--no-anchors"}}, want: []string{"func\tRun", "func\tConsumer"}},
		{name: "StructuralLookup", commands: [][]string{{"-F", "func Run(", "service.go", "--no-anchors"}}, want: []string{"func Run()", "validate()", "save()"}},
		{name: "LineLocateThenAt", commands: [][]string{{"--line-only", "-F", "func Run(", "service.go", "--no-anchors"}, {"--at", "service.go:2-6", "--no-anchors"}}, want: []string{"service.go:2-6:func Run()", "validate()", "save()"}},
		{name: "RelatedNavigation", commands: [][]string{{"--related", "--at", "service.go:2", "--no-anchors"}}, want: []string{"Next points", "→ validate", "← Handler"}},
		{name: "ImpactGraph", commands: [][]string{{"graph", "impact", "--at", "service.go:2", "--depth", "1", "--compact", "."}}, want: []string{"query impact depth=1", "Run -> validate#", "Handler -> Run#"}},
		{name: "ArchitectureResolve", commands: [][]string{{"architecture", "resolve", "--symbol", "Run", "--compact", "."}}, want: []string{"architecture resolve symbol=Run matches=1", "service.go:2-6"}},
		{name: "EnclosingScope", commands: [][]string{{"--line-only", "--enclosing", "-F", "save()", "service.go", "--no-anchors"}}, want: []string{"service.go:5@2-6:\tsave()"}},
		{name: "EditLocation", commands: [][]string{{"--line-only", "-F", "EDIT_NEEDLE", "service.go", "--no-anchors"}}, want: []string{"service.go:4:\tvalidate() // EDIT_NEEDLE"}},
	}
	for _, workflow := range workflows {
		workflow := workflow
		b.Run(workflow.name, func(b *testing.B) { benchmarkAgentWorkflow(b, workflow) })
	}
}

func benchmarkAgentWorkflow(b *testing.B, workflow agentWorkflowBenchmark) {
	b.Helper()
	b.ReportAllocs()
	b.ReportMetric(float64(len(workflow.commands)), "tool_calls/op")
	totalBytes := 0
	for range b.N {
		var combined strings.Builder
		for _, command := range workflow.commands {
			output := captureStdout(b, func() {
				if err := Run(command); err != nil {
					b.Fatal(err)
				}
			})
			combined.WriteString(output)
		}
		output := combined.String()
		for _, expected := range workflow.want {
			if !strings.Contains(output, expected) {
				b.Fatalf("workflow %s missing %q:\n%s", workflow.name, expected, output)
			}
		}
		totalBytes += len(output)
	}
	if b.N > 0 {
		bytesPerOperation := float64(totalBytes) / float64(b.N)
		b.ReportMetric(bytesPerOperation, "retrieved_bytes/op")
		b.ReportMetric(bytesPerOperation/4, "approx_tokens/op")
	}
}

func writeAgentWorkflowBenchmarkFixture(tb testing.TB) string {
	tb.Helper()
	root := tb.TempDir()
	files := map[string]string{
		"go.mod":     "module example.com/agentbench\n",
		"service.go": "package sample\nfunc Run() {\n\tvalidate()\n\tvalidate() // EDIT_NEEDLE\n\tsave()\n}\nfunc validate() {}\nfunc save() {}\nfunc Consumer() { Run() }\n",
		"handler.go": "package sample\nfunc Handler() { Run() }\n",
	}
	var noise strings.Builder
	noise.WriteString("synthetic broad-output fixture\n")
	for index := range 800 {
		fmt.Fprintf(&noise, "func noise%d() { println(\"NOISE_MATCH\") }\n", index)
	}
	files["noise.txt"] = noise.String()
	for path, content := range files {
		if err := os.WriteFile(filepath.Join(root, path), []byte(content), 0o644); err != nil {
			tb.Fatal(err)
		}
	}
	return root
}
