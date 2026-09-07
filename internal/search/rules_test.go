package search

import (
	"grepple/internal/api"
	"testing"
)

func strptr(s string) *string { return &s }
func boolptr(b bool) *bool    { return &b }

func TestNormalizeRuleDefaultsAndID(t *testing.T) {
	r, err := NormalizeRule(api.Rule{Name: "Uses Checkout Action", Request: api.SearchRequest{Query: strptr("actions/checkout")}})
	if err != nil {
		t.Fatal(err)
	}
	if r.Mode != api.RuleModeCount {
		t.Errorf("default mode = %q, want %q", r.Mode, api.RuleModeCount)
	}
	if r.ID != "uses-checkout-action" {
		t.Errorf("slug id = %q, want uses-checkout-action", r.ID)
	}
}

func TestNormalizeRuleInfersFilesMode(t *testing.T) {
	r, err := NormalizeRule(api.Rule{ID: "wf", Request: api.SearchRequest{Files: true, Globs: []string{"**/*.yml"}}})
	if err != nil {
		t.Fatal(err)
	}
	if r.Mode != api.RuleModeFiles {
		t.Errorf("mode = %q, want %q (inferred from Files)", r.Mode, api.RuleModeFiles)
	}
}

func TestNormalizeRuleRandomIDWhenNameUnusable(t *testing.T) {
	r, err := NormalizeRule(api.Rule{Name: "---", Request: api.SearchRequest{Query: strptr("x")}})
	if err != nil {
		t.Fatal(err)
	}
	if !ruleIDPattern.MatchString(r.ID) {
		t.Errorf("generated id %q does not match id pattern", r.ID)
	}
}

func TestNormalizeRuleRejectsBadInput(t *testing.T) {
	cases := map[string]api.Rule{
		"bad mode":        {ID: "a", Mode: "matches", Request: api.SearchRequest{Query: strptr("x")}},
		"count no query":  {ID: "a", Mode: api.RuleModeCount, Request: api.SearchRequest{}},
		"files no target": {ID: "a", Mode: api.RuleModeFiles, Request: api.SearchRequest{}},
		"bad regex":       {ID: "a", Request: api.SearchRequest{Query: strptr("("), Regex: boolptr(true)}},
		"bad id":          {ID: "Has Spaces", Request: api.SearchRequest{Query: strptr("x")}},
	}
	for name, rule := range cases {
		if _, err := NormalizeRule(rule); err == nil {
			t.Errorf("%s: expected error, got nil", name)
		}
	}
}

func TestNormalizeRuleGlobOnlyFilesModeAllowed(t *testing.T) {
	if _, err := NormalizeRule(api.Rule{ID: "go", Mode: api.RuleModeFiles, Request: api.SearchRequest{Globs: []string{"**/go.mod"}}}); err != nil {
		t.Errorf("glob-only files rule should be valid: %v", err)
	}
}

func TestSortRuleRepoResults(t *testing.T) {
	in := []api.RuleRepoResult{{Repo: "o/c"}, {Repo: "o/a"}, {Repo: "o/b"}}
	SortRuleRepoResults(in)
	if in[0].Repo != "o/a" || in[1].Repo != "o/b" || in[2].Repo != "o/c" {
		t.Errorf("not sorted: %+v", in)
	}
}
