package architecture

import (
	"errors"
	"testing"

	"github.com/greppleai/grepple/internal/search"
)

func TestBuildUsesInjectedSourceConfiguration(t *testing.T) {
	expected := errors.New("source config")
	_, err := Build(nil, 0, Dependencies{ApplySourceConfig: func(*search.Params) error { return expected }})
	if !errors.Is(err, expected) {
		t.Fatalf("Build error=%v, want %v", err, expected)
	}
}

func TestRunRejectsUnregisteredArchitectureCommands(t *testing.T) {
	for _, name := range []string{"resolve", "why", "responsibilities", "compare", "unknown"} {
		if err := newWithDependencies(Dependencies{}).Run([]string{name}); err == nil {
			t.Errorf("unregistered architecture command %q was accepted", name)
		}
	}
}
