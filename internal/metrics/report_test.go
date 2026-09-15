package metrics

import (
	"testing"
	"time"
)

func TestBuildReportGroupsAndNormalizesSuccessfulTasks(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	runs := []Run{
		{AssignedCohort: "control", StartedAt: start, EndedAt: start.Add(time.Second), Usage: Usage{TotalTokens: 100, Cost: 1}, Outcome: Outcome{Status: "success"}},
		{AssignedCohort: "control", StartedAt: start, EndedAt: start.Add(3 * time.Second), Usage: Usage{TotalTokens: 300, Cost: 3}, Outcome: Outcome{Status: "failure"}},
		{AssignedCohort: "grepple", StartedAt: start, EndedAt: start.Add(500 * time.Millisecond), Usage: Usage{TotalTokens: 50, Cost: .5}, Outcome: Outcome{Status: "success"}},
	}
	report := BuildReport(runs, "cohort", start)
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
	if len(report.Comparisons) != 1 {
		t.Fatalf("comparisons = %#v", report.Comparisons)
	}
	tokenDelta := report.Comparisons[0].Metrics["tokensMedian"]
	if tokenDelta.Absolute != -150 || tokenDelta.Percent == nil || *tokenDelta.Percent != -75 {
		t.Fatalf("token delta = %#v", tokenDelta)
	}
	successDelta := report.Comparisons[0].Metrics["successRate"]
	if successDelta.Absolute != .5 || successDelta.Percent == nil || *successDelta.Percent != 100 {
		t.Fatalf("success delta = %#v", successDelta)
	}
}
