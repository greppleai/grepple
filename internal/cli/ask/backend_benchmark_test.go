package ask

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/greppleai/grepple/internal/cliruntime"
)

type askUniverseBenchmarkInputs struct {
	navigate     askNavigateInput
	graph        askGraphInput
	architecture askArchitectureInput
}

func BenchmarkResearchUniverseReuse(b *testing.B) {
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
	application := cliruntime.Environment{}
	for range b.N {
		for _, run := range []func(*researchSession) error{
			func(session *researchSession) error {
				_, err := runAskNavigateWithSession(context.Background(), session, root, "", inputs.navigate)
				return err
			},
			func(session *researchSession) error {
				_, err := runAskGraphWithSession(session, root, inputs.graph)
				return err
			},
			func(session *researchSession) error {
				_, err := runAskArchitectureWithSession(session, root, inputs.architecture)
				return err
			},
		} {
			session := newResearchSession(application, context.Background(), nil, root, "")
			if err := run(session); err != nil {
				b.Fatal(err)
			}
			session.Close()
		}
	}
}

func benchmarkSharedAskUniverse(b *testing.B, root string, inputs askUniverseBenchmarkInputs) {
	b.Helper()
	b.ReportAllocs()
	application := cliruntime.Environment{}
	for range b.N {
		session := newResearchSession(application, context.Background(), nil, root, "")
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
