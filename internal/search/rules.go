package search

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"grepple/internal/api"
	"regexp"
	"sort"
	"strings"
)

var ruleIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

// NormalizeRule fills defaults (mode, id), trims fields, and validates the rule
// the same way a search request is validated, so an invalid rule is rejected at
// creation rather than silently failing on every reindex. It returns the
// normalized rule ready to store.
func NormalizeRule(r api.Rule) (api.Rule, error) {
	r.Name = strings.TrimSpace(r.Name)
	r.ID = strings.TrimSpace(r.ID)
	if r.Mode == "" {
		if r.Request.Files {
			r.Mode = api.RuleModeFiles
		} else {
			r.Mode = api.RuleModeCount
		}
	}
	if r.Mode != api.RuleModeCount && r.Mode != api.RuleModeFiles {
		return r, fmt.Errorf("invalid mode %q (want %q or %q)", r.Mode, api.RuleModeCount, api.RuleModeFiles)
	}

	// Validate the search options through the same resolver a live search uses.
	// For files mode an empty query is allowed (glob-only rules), so mark Files
	// before resolving; count mode requires a query, which ResolveRequest
	// enforces.
	vr := r.Request
	vr.Files = r.Mode == api.RuleModeFiles
	p, err := ResolveRequest(vr)
	if err != nil {
		return r, err
	}
	if strings.TrimSpace(p.Query) == "" && len(p.Globs) == 0 {
		return r, fmt.Errorf("rule needs a query or at least one glob")
	}
	if p.Regex && strings.TrimSpace(p.Query) != "" {
		if _, err := regexp.Compile(p.Query); err != nil {
			return r, fmt.Errorf("invalid regex: %w", err)
		}
	}

	if r.ID == "" {
		r.ID = slugID(r.Name)
	}
	if r.ID == "" {
		r.ID = randomID()
	}
	if !ruleIDPattern.MatchString(r.ID) {
		return r, fmt.Errorf("invalid rule id %q (allowed: lowercase letters, digits, '-' and '_', up to 64 chars)", r.ID)
	}
	return r, nil
}

// slugID turns a human name into a stable, url-safe id: lowercase, non
// alphanumeric runs collapsed to a single '-', trimmed. Returns "" when nothing
// usable remains (caller then falls back to a random id).
func slugID(name string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	slug := strings.Trim(b.String(), "-")
	if len(slug) > 64 {
		slug = strings.Trim(slug[:64], "-")
	}
	return slug
}

// randomID returns a short random hex id, used when a rule has no id and no
// sluggable name.
func randomID() string {
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "rule"
	}
	return "r" + hex.EncodeToString(b[:])
}

// SortRuleRepoResults orders results by repository name for a deterministic,
// diff-friendly response (no ranking), matching how search results are ordered.
func SortRuleRepoResults(results []api.RuleRepoResult) {
	sort.Slice(results, func(i, j int) bool { return results[i].Repo < results[j].Repo })
}
