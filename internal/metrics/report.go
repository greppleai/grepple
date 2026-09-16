package metrics

import (
	"fmt"
	"sort"
	"strconv"
	"time"
)

// BuildReport groups runs by a stable, privacy-safe dimension and derives its timestamp from evidence.
func BuildReport(runs []Run, groupBy string) Report {
	ordered := append([]Run(nil), runs...)
	sort.SliceStable(ordered, func(left, right int) bool {
		return runSortKey(ordered[left]) < runSortKey(ordered[right])
	})
	generated := time.Time{}
	buckets := make(map[string][]*Run)
	for index := range ordered {
		run := &ordered[index]
		if run.EndedAt.After(generated) {
			generated = run.EndedAt
		}
		key := groupKey(*run, groupBy)
		buckets[key] = append(buckets[key], run)
	}
	names := make([]string, 0, len(buckets))
	for name := range buckets {
		names = append(names, name)
	}
	sort.Strings(names)
	groups := make([]Group, 0, len(names))
	for _, name := range names {
		groups = append(groups, summarizeGroup(name, buckets[name]))
	}
	return Report{
		Schema:      "grepple-agent-report-v1",
		Generated:   generated.UTC(),
		Runs:        ordered,
		Groups:      groups,
		Comparisons: compareGroups(groups),
	}
}

// BuildComparison builds one explicit target-minus-baseline comparison.
func BuildComparison(runs []Run, groupBy, baselineName, targetName string) (ComparisonReport, error) {
	if baselineName == "" || targetName == "" {
		return ComparisonReport{}, fmt.Errorf("baseline and target are required")
	}
	if baselineName == targetName {
		return ComparisonReport{}, fmt.Errorf("baseline and target must differ")
	}
	report := BuildReport(runs, groupBy)
	groups := make(map[string]Group, len(report.Groups))
	for _, group := range report.Groups {
		groups[group.Name] = group
	}
	baseline, ok := groups[baselineName]
	if !ok {
		return ComparisonReport{}, fmt.Errorf("baseline group %q not found", baselineName)
	}
	target, ok := groups[targetName]
	if !ok {
		return ComparisonReport{}, fmt.Errorf("target group %q not found", targetName)
	}
	return ComparisonReport{
		Schema: "grepple-agent-comparison-v1", Generated: report.Generated, GroupBy: groupBy,
		Baseline: baseline, Target: target, Delta: compareGroupPair(baseline, target),
	}, nil
}

func runSortKey(run Run) string {
	return run.RunID + "\x00" + run.StartedAt.UTC().Format(time.RFC3339Nano) + "\x00" + run.Repository + "\x00" + run.TaskID
}

func compareGroups(groups []Group) []Comparison {
	if len(groups) < 2 {
		return nil
	}
	comparisons := make([]Comparison, 0, len(groups)-1)
	baseline := groups[0]
	for _, target := range groups[1:] {
		comparisons = append(comparisons, compareGroupPair(baseline, target))
	}
	return comparisons
}

func compareGroupPair(baseline, target Group) Comparison {
	metrics := make(map[string]Delta)
	if baseline.UsageSampleSize > 0 && target.UsageSampleSize > 0 {
		addDelta(metrics, "tokensMean", baseline.Tokens.Mean, target.Tokens.Mean)
		addDelta(metrics, "tokensMedian", baseline.Tokens.Median, target.Tokens.Median)
		addDelta(metrics, "tokensP90", baseline.Tokens.P90, target.Tokens.P90)
		addDelta(metrics, "costMean", baseline.Cost.Mean, target.Cost.Mean)
		addDelta(metrics, "costMedian", baseline.Cost.Median, target.Cost.Median)
		addDelta(metrics, "costP90", baseline.Cost.P90, target.Cost.P90)
	}
	addDelta(metrics, "elapsedMsMean", baseline.ElapsedMS.Mean, target.ElapsedMS.Mean)
	addDelta(metrics, "elapsedMsMedian", baseline.ElapsedMS.Median, target.ElapsedMS.Median)
	addDelta(metrics, "elapsedMsP90", baseline.ElapsedMS.P90, target.ElapsedMS.P90)
	addOptionalDelta(metrics, "successRate", baseline.SuccessRate, target.SuccessRate)
	addOptionalDelta(metrics, "tokensPerSuccessfulTask", baseline.TokensPerSuccessfulTask, target.TokensPerSuccessfulTask)
	addOptionalDelta(metrics, "costPerSuccessfulTask", baseline.CostPerSuccessfulTask, target.CostPerSuccessfulTask)
	addOptionalDelta(metrics, "timePerSuccessfulTask", baseline.TimePerSuccessfulTask, target.TimePerSuccessfulTask)
	return Comparison{Baseline: baseline.Name, Target: target.Name, Metrics: metrics}
}

func addOptionalDelta(metrics map[string]Delta, name string, baseline, target *float64) {
	if baseline != nil && target != nil {
		addDelta(metrics, name, *baseline, *target)
	}
}

func addDelta(metrics map[string]Delta, name string, baseline, target float64) {
	absolute := target - baseline
	delta := Delta{Absolute: absolute}
	if baseline != 0 {
		percent := absolute / baseline * 100
		delta.Percent = &percent
	}
	metrics[name] = delta
}

func groupKey(run Run, groupBy string) string {
	var key string
	switch groupBy {
	case "observed":
		key = strconv.FormatBool(run.ObservedGreppleUse)
	case "model":
		key = run.Provider + "/" + run.Model
	case "repository":
		key = run.Repository
	case "task":
		key = run.TaskID
	default:
		key = run.AssignedCohort
	}
	if key == "" {
		return "unknown"
	}
	return key
}

func summarizeGroup(name string, runs []*Run) Group {
	group := Group{Name: name, SampleSize: len(runs)}
	tokens := make([]float64, 0, len(runs))
	costs := make([]float64, 0, len(runs))
	times := make([]float64, 0, len(runs))
	totalOutcomeTokens := float64(0)
	totalOutcomeCost := float64(0)
	totalOutcomeTime := float64(0)
	usageComplete := true
	for _, run := range runs {
		usageKnown := !runMissing(*run, "model_usage")
		if usageKnown {
			group.UsageSampleSize++
			tokens = append(tokens, float64(run.Usage.TotalTokens))
			costs = append(costs, run.Usage.Cost)
		} else {
			usageComplete = false
		}
		elapsed := float64(run.EndedAt.Sub(run.StartedAt).Milliseconds())
		if elapsed < 0 {
			elapsed = 0
		}
		times = append(times, elapsed)
		switch run.Outcome.Status {
		case "success", "failure", "abandoned":
			group.OutcomeSampleSize++
			totalOutcomeTokens += float64(run.Usage.TotalTokens)
			totalOutcomeCost += run.Usage.Cost
			totalOutcomeTime += elapsed
			if run.Outcome.Status == "success" {
				group.Successful++
			}
		}
	}
	if len(tokens) > 0 {
		tokenDistribution := distribution(tokens)
		costDistribution := distribution(costs)
		group.Tokens = &tokenDistribution
		group.Cost = &costDistribution
	}
	group.ElapsedMS = distribution(times)
	if group.OutcomeSampleSize > 0 {
		value := float64(group.Successful) / float64(group.OutcomeSampleSize)
		group.SuccessRate = &value
	}
	if group.Successful > 0 {
		if usageComplete {
			tokensPerSuccess := totalOutcomeTokens / float64(group.Successful)
			costPerSuccess := totalOutcomeCost / float64(group.Successful)
			group.TokensPerSuccessfulTask = &tokensPerSuccess
			group.CostPerSuccessfulTask = &costPerSuccess
		}
		timePerSuccess := totalOutcomeTime / float64(group.Successful)
		group.TimePerSuccessfulTask = &timePerSuccess
	}
	return group
}

func runMissing(run Run, evidence string) bool {
	for _, missing := range run.Missing {
		if missing == evidence {
			return true
		}
	}
	return false
}

func distribution(values []float64) Distribution {
	if len(values) == 0 {
		return Distribution{}
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	total := float64(0)
	for _, value := range sorted {
		total += value
	}
	median := percentile(sorted, 0.5)
	return Distribution{Mean: total / float64(len(sorted)), Median: median, P50: median, P90: percentile(sorted, 0.9)}
}

func percentile(sorted []float64, fraction float64) float64 {
	if len(sorted) == 1 {
		return sorted[0]
	}
	position := fraction * float64(len(sorted)-1)
	lower := int(position)
	upper := lower + 1
	if upper >= len(sorted) {
		return sorted[lower]
	}
	weight := position - float64(lower)
	return sorted[lower]*(1-weight) + sorted[upper]*weight
}
