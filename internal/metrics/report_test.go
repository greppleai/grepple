package metrics

import (
	"reflect"
	"testing"
	"time"
)

func TestBuildReportIsDeterministicAndNormalizesSuccessfulTasks(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	runs := []Run{
		{RunID: "b", AssignedCohort: "control", StartedAt: start, EndedAt: start.Add(3 * time.Second), Usage: Usage{TotalTokens: 300, Cost: 3}, Outcome: Outcome{Status: "failure"}},
		{RunID: "a", AssignedCohort: "control", StartedAt: start, EndedAt: start.Add(time.Second), Usage: Usage{TotalTokens: 100, Cost: 1}, Outcome: Outcome{Status: "success"}},
		{RunID: "c", AssignedCohort: "grepple", StartedAt: start, EndedAt: start.Add(500 * time.Millisecond), Usage: Usage{TotalTokens: 50, Cost: .5}, Outcome: Outcome{Status: "success"}},
	}
	report := BuildReport(runs, "cohort")
	reversed := []Run{runs[2], runs[1], runs[0]}
	if other := BuildReport(reversed, "cohort"); !reflect.DeepEqual(report, other) {
		t.Fatalf("reports differ by input order:\n%#v\n%#v", report, other)
	}
	if !report.Generated.Equal(start.Add(3*time.Second)) || report.Runs[0].RunID != "a" {
		t.Fatalf("generated/order = %s, %v", report.Generated, report.Runs)
	}
	if len(report.Groups) != 2 || report.Groups[0].Name != "control" {
		t.Fatalf("groups = %#v", report.Groups)
	}
	control := report.Groups[0]
	if control.SuccessRate == nil || *control.SuccessRate != .5 || control.Tokens.Median != 200 {
		t.Fatalf("control = %#v", control)
	}
	if control.TokensPerSuccessfulTask == nil || *control.TokensPerSuccessfulTask != 400 {
		t.Fatalf("successful normalization = %#v", control.TokensPerSuccessfulTask)
	}
}

func TestBuildReportExcludesUnavailableUsageFromAggregates(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	runs := []Run{
		{RunID: "known", AssignedCohort: "control", StartedAt: start, EndedAt: start.Add(time.Second), Usage: Usage{TotalTokens: 100, Cost: 1}, Outcome: Outcome{Status: "success"}},
		{RunID: "missing", AssignedCohort: "control", StartedAt: start, EndedAt: start.Add(2 * time.Second), Missing: []string{"model_usage"}, Outcome: Outcome{Status: "failure"}},
	}
	group := BuildReport(runs, "cohort").Groups[0]
	if group.UsageSampleSize != 1 || group.Tokens.Mean != 100 || group.Cost.Mean != 1 {
		t.Fatalf("usage summary = %#v", group)
	}
	if group.TokensPerSuccessfulTask != nil || group.CostPerSuccessfulTask != nil {
		t.Fatalf("unknown usage produced per-success metrics: %#v", group)
	}
	unknownGroup := BuildReport([]Run{{
		RunID: "unknown", AssignedCohort: "control", StartedAt: start, EndedAt: start.Add(time.Second),
		Missing: []string{"model_usage"},
	}}, "cohort").Groups[0]
	if unknownGroup.Tokens != nil || unknownGroup.Cost != nil {
		t.Fatalf("unavailable usage represented as a distribution: %#v", unknownGroup)
	}
}

func TestBuildComparisonUsesExplicitBaselineAndTarget(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	runs := []Run{
		{RunID: "a", AssignedCohort: "control", StartedAt: start, EndedAt: start.Add(time.Second), Usage: Usage{TotalTokens: 100}, Outcome: Outcome{Status: "success"}},
		{RunID: "b", AssignedCohort: "grepple", StartedAt: start, EndedAt: start.Add(2 * time.Second), Usage: Usage{TotalTokens: 50}, Outcome: Outcome{Status: "success"}},
	}
	comparison, err := BuildComparison(runs, "cohort", "control", "grepple")
	if err != nil {
		t.Fatal(err)
	}
	if comparison.Baseline.Name != "control" || comparison.Target.Name != "grepple" || comparison.Delta.Baseline != "control" || comparison.Delta.Target != "grepple" {
		t.Fatalf("comparison = %#v", comparison)
	}
	if comparison.Delta.Metrics["tokensMedian"].Absolute != -50 {
		t.Fatalf("delta = %#v", comparison.Delta)
	}
	if _, err := BuildComparison(runs, "cohort", "missing", "grepple"); err == nil {
		t.Fatal("expected missing baseline error")
	}
}
