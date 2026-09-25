package metrics

import (
	"reflect"
	"testing"
	"time"
)

func TestBuildReportIsDeterministicAndGroupsAgents(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	runs := []Run{
		{RunID: "b", Agent: "pi", StartedAt: start, EndedAt: start.Add(3 * time.Second), Usage: Usage{TotalTokens: 300, Cost: 3}, Outcome: Outcome{Status: "failure"}},
		{RunID: "a", Agent: "pi", StartedAt: start, EndedAt: start.Add(time.Second), Usage: Usage{TotalTokens: 100, Cost: 1}, Outcome: Outcome{Status: "success"}},
		{RunID: "c", Agent: "claude-code", StartedAt: start, EndedAt: start.Add(500 * time.Millisecond), Usage: Usage{TotalTokens: 50, Cost: .5}, Outcome: Outcome{Status: "success"}},
	}
	report := BuildReport(runs)
	reversed := []Run{runs[2], runs[1], runs[0]}
	if other := BuildReport(reversed); !reflect.DeepEqual(report, other) {
		t.Fatalf("reports differ by input order:\n%#v\n%#v", report, other)
	}
	if !report.Generated.Equal(start.Add(3*time.Second)) || report.Runs[0].RunID != "a" {
		t.Fatalf("generated/order = %s, %v", report.Generated, report.Runs)
	}
	if len(report.Groups) != 2 || report.Groups[0].Name != "claude-code" || report.Groups[1].Name != "pi" {
		t.Fatalf("agent groups = %#v", report.Groups)
	}
	pi := report.Groups[1]
	if pi.SuccessRate == nil || *pi.SuccessRate != .5 || pi.Tokens.Median != 200 {
		t.Fatalf("pi = %#v", pi)
	}
	if pi.TokensPerSuccessfulTask == nil || *pi.TokensPerSuccessfulTask != 400 {
		t.Fatalf("successful normalization = %#v", pi.TokensPerSuccessfulTask)
	}
}

func TestBuildReportExcludesUnavailableUsageFromAggregates(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	runs := []Run{
		{RunID: "known", Agent: "pi", StartedAt: start, EndedAt: start.Add(time.Second), Usage: Usage{TotalTokens: 100, Cost: 1}, Outcome: Outcome{Status: "success"}},
		{RunID: "missing", Agent: "pi", StartedAt: start, EndedAt: start.Add(2 * time.Second), Missing: []string{"assistant_usage"}, Outcome: Outcome{Status: "failure"}},
	}
	group := BuildReport(runs).Groups[0]
	if group.UsageSampleSize != 1 || group.Tokens.Mean != 100 || group.Cost.Mean != 1 {
		t.Fatalf("usage summary = %#v", group)
	}
	if group.TokensPerSuccessfulTask != nil || group.CostPerSuccessfulTask != nil {
		t.Fatalf("unknown usage produced per-success metrics: %#v", group)
	}
	unknownGroup := BuildReport([]Run{{
		RunID: "unknown", Agent: "pi", StartedAt: start, EndedAt: start.Add(time.Second),
		Missing: []string{"assistant_usage"},
	}}).Groups[0]
	if unknownGroup.Tokens != nil || unknownGroup.Cost != nil {
		t.Fatalf("unavailable usage represented as a distribution: %#v", unknownGroup)
	}
}

func TestBuildComparisonUsesSeparateSameAgentReports(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	baseline := BuildReport([]Run{{
		RunID: "without-grepple", Agent: "pi", StartedAt: start, EndedAt: start.Add(2 * time.Second),
		Usage: Usage{TotalTokens: 100}, Outcome: Outcome{Status: "success"},
	}})
	target := BuildReport([]Run{{
		RunID: "with-grepple", Agent: "pi", StartedAt: start, EndedAt: start.Add(time.Second),
		Usage: Usage{TotalTokens: 50}, Outcome: Outcome{Status: "success"},
	}})

	comparison, err := BuildComparison(baseline, target)
	if err != nil {
		t.Fatal(err)
	}
	if comparison.Baseline.Name != "pi" || comparison.Target.Name != "pi" || comparison.Delta.Baseline != "pi" || comparison.Delta.Target != "pi" {
		t.Fatalf("comparison = %#v", comparison)
	}
	if comparison.Delta.Metrics["tokensMedian"].Absolute != -50 {
		t.Fatalf("delta = %#v", comparison.Delta)
	}
	if !comparison.Generated.Equal(baseline.Generated) {
		t.Fatalf("generatedAt = %s", comparison.Generated)
	}

	multipleAgents := BuildReport([]Run{
		{RunID: "pi", Agent: "pi", StartedAt: start, EndedAt: start},
		{RunID: "claude", Agent: "claude-code", StartedAt: start, EndedAt: start},
	})
	if _, err := BuildComparison(multipleAgents, target); err == nil {
		t.Fatal("expected multi-agent report rejection")
	}
	if _, err := BuildComparison(Report{}, target); err == nil {
		t.Fatal("expected empty report rejection")
	}
}
