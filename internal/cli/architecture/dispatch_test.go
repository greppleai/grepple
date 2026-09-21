package architecture

import (
	"errors"
	"testing"

	"github.com/greppleai/grepple/search"
)

func TestBuildUsesInjectedSourceConfiguration(t *testing.T) {
	expected := errors.New("source config")
	_, err := Build(nil, 0, Dependencies{ApplySourceConfig: func(*search.Params) error { return expected }})
	if !errors.Is(err, expected) {
		t.Fatalf("Build error=%v, want %v", err, expected)
	}
}

func TestRunRejectsUnknownArchitectureCommand(t *testing.T) {
	if err := Run([]string{"unknown"}, Dependencies{}); err == nil {
		t.Fatal("unknown command accepted")
	}
}
