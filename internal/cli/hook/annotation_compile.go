// Package hook runs repository-owned, read-only GritQL checks.
package hook

import (
	"fmt"
	"github.com/greppleai/grepple/internal/gritql"
)

func annotationRules(rules []compiledRule) map[string]*annotationConfig {
	configs := map[string]*annotationConfig{}
	for _, r := range rules {
		if r.Annotation != nil {
			configs[r.ID] = r.Annotation
		}
	}
	return configs
}
func validateRuleSourceConstraints(r rule) error {
	if err := gritql.ValidateGlobs(r.Include, r.Exclude); err != nil {
		return err
	}
	if err := validateAssertionConfig(r); err != nil {
		return err
	}
	return validateAnnotationConfig(r)
}
func compileFileProgram(r rule) (*gritql.Program, error) {
	program, err := gritql.Compile([]byte(r.Query), gritql.CompileOptions{})
	if err != nil {
		return nil, err
	}
	if r.Annotation != nil && program.Language() != "go" {
		return nil, fmt.Errorf("annotation currently supports only Go declarations")
	}
	if err := validateAssertionProgram(r, program); err != nil {
		return nil, err
	}
	return program, nil
}
