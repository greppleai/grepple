// Package rulespec normalizes saved text and structural rule definitions.
package rulespec

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"github.com/greppleai/grepple/gritql"
	"github.com/greppleai/grepple/search"
)

var ruleIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

// Engine returns the effective rule engine, including the legacy empty-as-text default.
func Engine(rule Rule) string {
	if rule.Engine == "" {
		return EngineText
	}
	return rule.Engine
}

// Normalize fills rule defaults and validates the engine-specific request.
func Normalize(rule Rule) (Rule, error) {
	rule.Name = strings.TrimSpace(rule.Name)
	rule.ID = strings.TrimSpace(rule.ID)
	switch Engine(rule) {
	case EngineText:
		if rule.Structural != nil {
			return rule, fmt.Errorf("text rule must not include a structural request")
		}
		rule.Engine = ""
		if err := normalizeTextRequest(&rule); err != nil {
			return rule, err
		}
	case EngineGritQL:
		if !reflect.DeepEqual(rule.Request, search.Request{}) {
			return rule, fmt.Errorf("structural rule must not include a text request")
		}
		if err := normalizeStructuralRequest(&rule); err != nil {
			return rule, err
		}
	default:
		return rule, fmt.Errorf("invalid rule engine %q", rule.Engine)
	}
	if err := normalizeRuleMetadata(&rule); err != nil {
		return rule, err
	}
	return rule, nil
}

func normalizeTextRequest(rule *Rule) error {
	if rule.Mode == "" {
		if rule.Request.Files {
			rule.Mode = ModeFiles
		} else {
			rule.Mode = ModeCount
		}
	}
	request := rule.Request
	request.Files = rule.Mode == ModeFiles
	params, err := search.ResolveRequest(request)
	if err != nil {
		return err
	}
	if strings.TrimSpace(params.Query) == "" && len(params.Globs) == 0 {
		return fmt.Errorf("rule needs a query or at least one glob")
	}
	if params.Regex && strings.TrimSpace(params.Query) != "" {
		if _, err := regexp.Compile(params.Query); err != nil {
			return fmt.Errorf("invalid regex: %w", err)
		}
	}
	return nil
}

func normalizeStructuralRequest(rule *Rule) error {
	if rule.Structural == nil {
		return fmt.Errorf("structural rule requires a structural request")
	}
	request := *rule.Structural
	request.Globs = append([]string(nil), request.Globs...)
	request.ExcludeGlobs = append([]string(nil), request.ExcludeGlobs...)
	request.Repositories = append([]string(nil), request.Repositories...)
	request.ExcludeRepositories = append([]string(nil), request.ExcludeRepositories...)
	if request.Skip != nil || request.Limit != nil {
		return fmt.Errorf("structural rules do not support paging")
	}
	if err := validateStructuralRequest(request); err != nil {
		return err
	}
	limits := StructuralLimits{}
	if request.Limits != nil {
		limits = *request.Limits
	}
	program, err := gritql.Compile([]byte(request.Query), gritql.CompileOptions{
		MaxPatternBytes: optionalInt(limits.PatternBytes), MaxRegexBytes: optionalInt(limits.RegexBytes),
		MaxRegexInstructions: optionalInt(limits.RegexInstructions), MaxDepth: optionalInt(limits.ParseDepth),
	})
	if err != nil {
		return err
	}
	if program.Compatibility() != request.Compatibility {
		return fmt.Errorf("query language requires structural compatibility %q", program.Compatibility())
	}
	if rule.Mode == "" {
		rule.Mode = ModeCount
	}
	rule.Structural = &request
	return nil
}
func supportedStructuralCompatibility(compatibility string) bool {
	return compatibility == GritCompatibilityV1
}

func validateStructuralRequest(request StructuralRequest) error {
	if !supportedStructuralCompatibility(request.Compatibility) {
		return fmt.Errorf("unsupported structural compatibility %q", request.Compatibility)
	}
	if request.Query == "" || len(request.Query) > MaxGritQueryBytes {
		return fmt.Errorf("structural query is empty or exceeds its maximum size")
	}
	if len(request.PatternID) > MaxGritPatternIDBytes || len(request.Message) > MaxGritMessageBytes {
		return fmt.Errorf("structural pattern identifier or message exceeds its maximum size")
	}
	if len(request.Globs)+len(request.ExcludeGlobs) > MaxGritGlobs || len(request.Repositories)+len(request.ExcludeRepositories) > MaxGritRepositories {
		return fmt.Errorf("structural scope contains too many values")
	}
	for _, selector := range append(append([]string{}, request.Repositories...), request.ExcludeRepositories...) {
		if selector == "" || len(selector) > MaxGritRepositoryBytes {
			return fmt.Errorf("structural repository selector is invalid")
		}
	}
	globs := append(append([]string{}, request.Globs...), request.ExcludeGlobs...)
	for _, glob := range globs {
		if len(glob) > MaxGritGlobBytes {
			return fmt.Errorf("structural glob exceeds its maximum size")
		}
	}
	if err := gritql.ValidateGlobs(request.Globs, request.ExcludeGlobs); err != nil {
		return err
	}
	return validateStructuralLimits(request.Limits)
}

func validateStructuralLimits(limits *StructuralLimits) error {
	if limits == nil {
		return nil
	}
	ints := []*int{limits.PatternBytes, limits.RegexBytes, limits.RegexInstructions, limits.ParseDepth, limits.SourceBytes, limits.Candidates, limits.ASTSteps, limits.Findings, limits.Files, limits.Workers}
	for _, value := range ints {
		if value != nil && *value < 0 {
			return fmt.Errorf("structural resource limits must not be negative")
		}
	}
	if limits.FileTimeMillis != nil && (*limits.FileTimeMillis < 0 || *limits.FileTimeMillis > 10_000) ||
		limits.BatchTimeMillis != nil && (*limits.BatchTimeMillis < 0 || *limits.BatchTimeMillis > 300_000) ||
		limits.MemoryBytes != nil && *limits.MemoryBytes < 0 || limits.TotalBytes != nil && *limits.TotalBytes < 0 {
		return fmt.Errorf("structural resource or time limit is invalid")
	}
	return nil
}

func normalizeRuleMetadata(rule *Rule) error {
	if rule.Mode != ModeCount && rule.Mode != ModeFiles {
		return fmt.Errorf("invalid mode %q (want %q or %q)", rule.Mode, ModeCount, ModeFiles)
	}
	if rule.ID == "" {
		rule.ID = slugID(rule.Name)
	}
	if rule.ID == "" {
		rule.ID = randomID()
	}
	if !ruleIDPattern.MatchString(rule.ID) {
		return fmt.Errorf("invalid rule id %q (allowed: lowercase letters, digits, '-' and '_', up to 64 chars)", rule.ID)
	}
	return nil
}

func optionalInt(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func slugID(name string) string {
	var builder strings.Builder
	lastDash := false
	for _, char := range strings.ToLower(name) {
		if char >= 'a' && char <= 'z' || char >= '0' && char <= '9' {
			builder.WriteRune(char)
			lastDash = false
		} else if !lastDash && builder.Len() > 0 {
			builder.WriteByte('-')
			lastDash = true
		}
	}
	slug := strings.Trim(builder.String(), "-")
	if len(slug) > 64 {
		slug = strings.Trim(slug[:64], "-")
	}
	return slug
}

func randomID() string {
	var value [6]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "rule"
	}
	return "r" + hex.EncodeToString(value[:])
}
