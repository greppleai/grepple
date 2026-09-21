// Package graph implements navigation graph command dispatch.
package graph

import "fmt"

// Dependencies supplies independently testable graph operations.
type Dependencies struct {
	Build   func([]string) error
	Resolve func([]string) error
	Diff    func([]string) error
	Query   func([]string) error
}

// Run dispatches graph subcommands without depending on the parent CLI package.
func Run(args []string, dependencies Dependencies) error {
	if len(args) > 0 {
		switch args[0] {
		case "resolve":
			if dependencies.Resolve == nil {
				return fmt.Errorf("graph resolve is unavailable")
			}
			return dependencies.Resolve(args[1:])
		case "diff":
			if dependencies.Diff == nil {
				return fmt.Errorf("graph diff is unavailable")
			}
			return dependencies.Diff(args[1:])
		case "callers", "callees", "impact", "dependencies", "dependents":
			if dependencies.Query == nil {
				return fmt.Errorf("graph query is unavailable")
			}
			return dependencies.Query(args)
		}
	}
	if dependencies.Build == nil {
		return fmt.Errorf("graph build is unavailable")
	}
	return dependencies.Build(args)
}
