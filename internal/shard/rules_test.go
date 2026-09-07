package shard

import (
	"os"
	"path/filepath"
	"testing"

	"grepple/internal/api"
	"grepple/internal/repository"
	"grepple/internal/search"
)

// ruleTestState builds a shard state with the given repos, each containing a
// single file with the provided body, and returns it plus the repo root.
func ruleTestState(t *testing.T, files map[string]string) (*shardImpl, string) {
	t.Helper()
	root := t.TempDir()
	registry := repository.NewRegistry(root)
	if err := registry.Init(); err != nil {
		t.Fatal(err)
	}
	for repo, body := range files {
		dir := filepath.Join(root, filepath.FromSlash(repo))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		registry.Record(repo, "", "HEAD", dir)
	}
	state := &shardImpl{registry: registry, root: root}
	t.Chdir(root)
	return state, root
}

func countRule(id, query string) api.Rule {
	r, _ := search.NormalizeRule(api.Rule{ID: id, Mode: api.RuleModeCount, Request: api.SearchRequest{Query: strptrD(query)}})
	return r
}

func strptrD(s string) *string { return &s }

func TestRuleBackfillCountMode(t *testing.T) {
	state, root := ruleTestState(t, map[string]string{
		"owner/foo": "package foo\n// needle here\n",
		"owner/bar": "package bar\n// nothing\n",
	})
	rs := newRuleService(filepath.Join(root, "rules.json"), state, nil).(*ruleServiceImpl)
	rs.apply(api.RuleSet{Generation: 1, Rules: []api.Rule{countRule("needle", "needle")}})
	rs.backfillRule("needle") // process synchronously (worker not running in test)

	mode, repos := rs.resultsFor("needle")
	if mode != api.RuleModeCount {
		t.Errorf("mode = %q", mode)
	}
	if len(repos) != 1 || repos[0].Repo != "owner/foo" || repos[0].Files != 1 || repos[0].Matches < 1 {
		t.Fatalf("expected owner/foo 1 file >=1 match, got %#v", repos)
	}
}

func TestRuleIncrementalEvaluateRepo(t *testing.T) {
	state, root := ruleTestState(t, map[string]string{
		"owner/foo": "package foo\n// needle here\n",
		"owner/bar": "package bar\n// nothing\n",
	})
	rs := newRuleService(filepath.Join(root, "rules.json"), state, nil).(*ruleServiceImpl)
	rs.apply(api.RuleSet{Generation: 1, Rules: []api.Rule{countRule("needle", "needle")}})
	rs.backfillRule("needle")

	// Introduce a match in owner/bar and re-evaluate just that repo.
	if err := os.WriteFile(filepath.Join(root, "owner", "bar", "main.go"), []byte("package bar\n// needle now\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rs.evaluateRepo("owner/bar")
	_, repos := rs.resultsFor("needle")
	if len(repos) != 2 {
		t.Fatalf("expected owner/foo and owner/bar after incremental eval, got %#v", repos)
	}

	// Remove the match again; the repo's row must disappear.
	if err := os.WriteFile(filepath.Join(root, "owner", "bar", "main.go"), []byte("package bar\n// gone\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rs.evaluateRepo("owner/bar")
	_, repos = rs.resultsFor("needle")
	if len(repos) != 1 || repos[0].Repo != "owner/foo" {
		t.Fatalf("expected only owner/foo after match removed, got %#v", repos)
	}
}

func TestRuleFilesMode(t *testing.T) {
	state, root := ruleTestState(t, map[string]string{
		"owner/foo": "package foo\n// needle\n",
	})
	rs := newRuleService(filepath.Join(root, "rules.json"), state, nil).(*ruleServiceImpl)
	rule, _ := search.NormalizeRule(api.Rule{ID: "wf", Mode: api.RuleModeFiles, Request: api.SearchRequest{Query: strptrD("needle")}})
	rs.apply(api.RuleSet{Generation: 1, Rules: []api.Rule{rule}})
	rs.backfillRule("wf")

	_, repos := rs.resultsFor("wf")
	if len(repos) != 1 || len(repos[0].Paths) != 1 || repos[0].Paths[0] != "owner/foo/main.go" {
		t.Fatalf("expected owner/foo/main.go path, got %#v", repos)
	}
}

func TestRuleForgetRepo(t *testing.T) {
	state, root := ruleTestState(t, map[string]string{"owner/foo": "package foo\n// needle\n"})
	rs := newRuleService(filepath.Join(root, "rules.json"), state, nil).(*ruleServiceImpl)
	rs.apply(api.RuleSet{Generation: 1, Rules: []api.Rule{countRule("needle", "needle")}})
	rs.backfillRule("needle")
	rs.forgetRepo("owner/foo")
	if _, repos := rs.resultsFor("needle"); len(repos) != 0 {
		t.Fatalf("expected no rows after forgetRepo, got %#v", repos)
	}
}

func TestRuleApplyIgnoresOlderGeneration(t *testing.T) {
	state, root := ruleTestState(t, map[string]string{"owner/foo": "package foo\n// needle\n"})
	rs := newRuleService(filepath.Join(root, "rules.json"), state, nil).(*ruleServiceImpl)
	if !rs.apply(api.RuleSet{Generation: 5, Rules: []api.Rule{countRule("needle", "needle")}}) {
		t.Fatal("gen 5 should apply")
	}
	if rs.apply(api.RuleSet{Generation: 3, Rules: nil}) {
		t.Fatal("gen 3 should be ignored (older)")
	}
	if len(rs.snapshotRules()) != 1 {
		t.Fatalf("rule set should be unchanged, got %d rules", len(rs.snapshotRules()))
	}
}

func TestRulePersistenceRoundTrip(t *testing.T) {
	state, root := ruleTestState(t, map[string]string{"owner/foo": "package foo\n// needle\n"})
	path := filepath.Join(root, "rules.json")
	rs := newRuleService(path, state, nil).(*ruleServiceImpl)
	rs.apply(api.RuleSet{Generation: 7, Rules: []api.Rule{countRule("needle", "needle")}})
	rs.backfillRule("needle")

	// A fresh store over the same file recovers rules and materialized results.
	reloaded := newRuleService(path, state, nil).(*ruleServiceImpl)
	if reloaded.currentGeneration() != 7 {
		t.Errorf("generation = %d, want 7", reloaded.currentGeneration())
	}
	if _, repos := reloaded.resultsFor("needle"); len(repos) != 1 || repos[0].Repo != "owner/foo" {
		t.Fatalf("results not recovered from disk, got %#v", repos)
	}
}
