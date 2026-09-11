// Package rulespec normalizes saved text and structural rule definitions.
package rulespec

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"github.com/greppleai/grepple/api"
	"github.com/greppleai/grepple/gritql"
	"github.com/greppleai/grepple/search"
)

var ruleIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

// Engine returns the effective rule engine, including the legacy empty-as-text default.
func Engine(rule api.Rule) string {
	if rule.Engine == "" {
		return api.RuleEngineText
	}
	return rule.Engine
}

// Normalize fills rule defaults and validates the engine-specific request.
func Normalize(rule api.Rule) (api.Rule, error) {
	rule.Name = strings.TrimSpace(rule.Name)
	rule.ID = strings.TrimSpace(rule.ID)
	switch Engine(rule) {
	case api.RuleEngineText:
		if rule.Structural != nil {
			return rule, fmt.Errorf("text rule must not include a structural request")
		}
		rule.Engine = ""
		if err := normalizeTextRequest(&rule); err != nil {
			return rule, err
		}
	case api.RuleEngineGritQL:
		if !reflect.DeepEqual(rule.Request, api.SearchRequest{}) {
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

func normalizeTextRequest(rule *api.Rule) error {
	if rule.Mode == "" {
		if rule.Request.Files {
			rule.Mode = api.RuleModeFiles
		} else {
			rule.Mode = api.RuleModeCount
		}
	}
	request := rule.Request
	request.Files = rule.Mode == api.RuleModeFiles
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

func normalizeStructuralRequest(rule *api.Rule) error {
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
	limits := api.GritLimits{}
	if request.Limits != nil {
		limits = *request.Limits
	}
	if _, err := gritql.Compile([]byte(request.Query), gritql.CompileOptions{
		MaxPatternBytes: optionalInt(limits.PatternBytes), MaxRegexBytes: optionalInt(limits.RegexBytes),
		MaxRegexInstructions: optionalInt(limits.RegexInstructions), MaxDepth: optionalInt(limits.ParseDepth),
	}); err != nil {
		return err
	}
	if rule.Mode == "" {
		rule.Mode = api.RuleModeCount
	}
	rule.Structural = &request
	return nil
}

func validateStructuralRequest(request api.GritRequest) error {
	if request.Compatibility != api.GritCompatibilityV1 {
		return fmt.Errorf("unsupported structural compatibility %q", request.Compatibility)
	}
	if request.Query == "" || len(request.Query) > api.MaxGritQueryBytes {
		return fmt.Errorf("structural query is empty or exceeds its maximum size")
	}
	if len(request.PatternID) > api.MaxGritPatternIDBytes || len(request.Message) > api.MaxGritMessageBytes {
		return fmt.Errorf("structural pattern identifier or message exceeds its maximum size")
	}
	if len(request.Globs)+len(request.ExcludeGlobs) > api.MaxGritGlobs || len(request.Repositories)+len(request.ExcludeRepositories) > api.MaxGritRepositories {
		return fmt.Errorf("structural scope contains too many values")
	}
	for _, selector := range append(append([]string{}, request.Repositories...), request.ExcludeRepositories...) {
		if selector == "" || len(selector) > api.MaxGritRepositoryBytes {
			return fmt.Errorf("structural repository selector is invalid")
		}
	}
	globs := append(append([]string{}, request.Globs...), request.ExcludeGlobs...)
	for _, glob := range globs {
		if len(glob) > api.MaxGritGlobBytes {
			return fmt.Errorf("structural glob exceeds its maximum size")
		}
	}
	if err := gritql.ValidateGlobs(request.Globs, request.ExcludeGlobs); err != nil {
		return err
	}
	return validateStructuralLimits(request.Limits)
}

func validateStructuralLimits(limits *api.GritLimits) error {
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

func normalizeRuleMetadata(rule *api.Rule) error {
	if rule.Mode != api.RuleModeCount && rule.Mode != api.RuleModeFiles {
		return fmt.Errorf("invalid mode %q (want %q or %q)", rule.Mode, api.RuleModeCount, api.RuleModeFiles)
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
