package analysis

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/greppleai/grepple/search"
)

//revive:disable-next-line:cognitive-complexity
func TestAnalysisProjectionsAreDeterministicAndQueryable(t *testing.T) {
	sources := []Source{
		{Path: "internal/target.go", Content: []byte("package internal\nfunc Target() {}\n")},
		{Path: "main.go", Content: []byte("package main\nfunc Caller() { Target() }\n")},
	}
	build := func() (*Universe, []byte) {
		universe, err := NewUniverse(sources, 0)
		if err != nil {
			t.Fatal(err)
		}
		report, err := BuildGraph(universe, nil)
		if err != nil {
			t.Fatal(err)
		}
		content, err := json.Marshal(report)
		if err != nil {
			t.Fatal(err)
		}
		return universe, content
	}
	firstUniverse, first := build()
	defer firstUniverse.Close()
	secondUniverse, second := build()
	defer secondUniverse.Close()
	if string(first) != string(second) {
		t.Fatalf("graph output is nondeterministic\n%s\n%s", first, second)
	}
	full, err := BuildGraph(firstUniverse, nil)
	if err != nil {
		t.Fatal(err)
	}
	var symbol string
	for _, declaration := range full.Declarations {
		if strings.HasSuffix(declaration.Name, "Target") {
			symbol = declaration.Name
			break
		}
	}
	if symbol == "" {
		t.Fatalf("declarations=%+v", full.Declarations)
	}
	queried, err := BuildGraph(firstUniverse, &GraphQuery{Direction: string(search.NavigationQueryCallers), Depth: 1, Symbol: symbol})
	if err != nil {
		t.Fatal(err)
	}
	if queried.Query == nil || len(queried.Query.RootIDs) != 1 {
		t.Fatalf("query=%+v", queried.Query)
	}
	architecture := BuildArchitecture(firstUniverse)
	if architecture.Schema != ArchitectureSchema || len(architecture.Directories) == 0 {
		t.Fatalf("architecture=%+v", architecture)
	}
	responsibilities := BuildResponsibilities(firstUniverse)
	if len(responsibilities.Responsibilities) != len(architecture.Directories) {
		t.Fatalf("responsibilities=%+v", responsibilities)
	}
	boundaries, err := BuildBoundaries(firstUniverse, nil, 1, search.BoundaryPolicy{}, "")
	if err != nil || boundaries.Schema != "grepple-boundaries-v3" {
		t.Fatalf("boundaries=%+v err=%v", boundaries, err)
	}
}

func TestUniverseReportsTruncationAndUnsupportedSources(t *testing.T) {
	universe, err := NewUniverse([]Source{{Path: "README.md", Content: []byte("text")}, {Path: "a.go", Content: []byte("package a\n")}, {Path: "b.go", Content: []byte("package b\n")}}, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer universe.Close()
	if universe.Truncation() == nil || universe.Truncation().Skipped != 1 {
		t.Fatalf("truncation=%+v", universe.Truncation())
	}
	if universe.Summary().Skipped == 0 || universe.Summary().Selected != 1 {
		t.Fatalf("summary=%+v", universe.Summary())
	}
}

func TestUniverseReportsDiscoveredReadFailures(t *testing.T) {
	universe, err := NewUniverse([]Source{{Path: "missing.go", ReadError: os.ErrNotExist}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer universe.Close()
	if universe.Summary().Discovered != 1 || universe.Summary().Selected != 1 || universe.Summary().Failed != 1 {
		t.Fatalf("summary=%+v", universe.Summary())
	}
}
