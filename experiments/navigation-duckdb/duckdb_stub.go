package main

import (
	"fmt"
	"time"

	"github.com/greppleai/grepple/internal/parser"
)

// The optional native implementation replaces these defaults during package
// initialization. Unique declaration names also allow build-tag-agnostic schema
// analysis to inspect the entire experiment.
var (
	duckDBEnabled         bool
	selectedDuckOpener    = unavailableDuckOpen
	selectedDuckBuilder   = unavailableDuckBuild
	selectedColumnOpener  = unavailableDuckOpen
	selectedColumnBuilder = unavailableDuckBuild
)

func openDuck(path string) (queryIndex, error) {
	return selectedDuckOpener(path)
}

func buildDuck(path string, graph parser.NavigationGraph) (time.Duration, error) {
	return selectedDuckBuilder(path, graph)
}

func unavailableDuckOpen(string) (queryIndex, error) {
	return nil, fmt.Errorf("DuckDB is not linked: build with -tags duckdb")
}

func unavailableDuckBuild(string, parser.NavigationGraph) (time.Duration, error) {
	return 0, fmt.Errorf("DuckDB is not linked: build with -tags duckdb")
}
