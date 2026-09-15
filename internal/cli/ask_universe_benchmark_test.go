package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

type askUniverseBenchmarkInputs struct {
	navigate     askNavigateInput
	graph        askGraphInput
	architecture askArchitectureInput
}

func BenchmarkAskResearchUniverseReuse(b *testing.B) {
	root := buildAskUniverseBenchmarkRepository(b, 30)
	b.Chdir(root)
	inputs := askUniverseBenchmarkInputs{
		navigate:     askNavigateInput{Location: "source_00.go:2", FollowDepth: 1},
		graph:        askGraphInput{Direction: "callees", Symbol: "Function0"},
		architecture: askArchitectureInput{Operation: "resolve", Symbol: "Shared"},
	}
	b.ReportMetric(30, "source_files")
	b.Run("cold-tools", func(b *testing.B) { benchmarkColdAskUniverse(b, root, inputs) })
	b.Run("shared-session", func(b *testing.B) { benchmarkSharedAskUniverse(b, root, inputs) })
}

func buildAskUniverseBenchmarkRepository(b *testing.B, files int) string {
	b.Helper()
	root := b.TempDir()
	for index := range files {
		content := fmt.Sprintf("package sample\nfunc Function%d() { Shared() }\n", index)
		if index == 0 {
			content += "func Shared() {}\n"
		}
		if err := os.WriteFile(filepath.Join(root, fmt.Sprintf("source_%02d.go", index)), []byte(content), 0o600); err != nil {
			b.Fatal(err)
		}
	}
	return root
}

func benchmarkColdAskUniverse(b *testing.B, root string, inputs askUniverseBenchmarkInputs) {
	b.Helper()
	b.ReportAllocs()
	for range b.N {
		if _, err := runAskNavigate(context.Background(), root, "", inputs.navigate); err != nil {
			b.Fatal(err)
		}
		if _, err := runAskGraph(root, inputs.graph); err != nil {
			b.Fatal(err)
		}
		if _, err := runAskArchitecture(root, inputs.architecture); err != nil {
			b.Fatal(err)
		}
	}
}

func benchmarkSharedAskUniverse(b *testing.B, root string, inputs askUniverseBenchmarkInputs) {
	b.Helper()
	b.ReportAllocs()
	for range b.N {
		session := newResearchSession(context.Background(), nil, root, "")
		if _, err := runAskNavigateWithSession(context.Background(), session, root, "", inputs.navigate); err != nil {
			b.Fatal(err)
		}
		if _, err := runAskGraphWithSession(session, root, inputs.graph); err != nil {
			b.Fatal(err)
		}
		if _, err := runAskArchitectureWithSession(session, root, inputs.architecture); err != nil {
			b.Fatal(err)
		}
		session.Close()
	}
}
