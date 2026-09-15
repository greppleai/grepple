package metrics

import (
	"sort"
	"strconv"
	"time"
)

// BuildReport groups runs by a stable, privacy-safe dimension.
func BuildReport(runs []Run, groupBy string, generated time.Time) Report {
	buckets := make(map[string][]*Run)
	for index := range runs {
		run := &runs[index]
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
		Runs:        runs,
		Groups:      groups,
		Comparisons: compareGroups(groups),
	}
}

func compareGroups(groups []Group) []Comparison {
	if len(groups) < 2 {
		return nil
	}
	comparisons := make([]Comparison, 0, len(groups)-1)
	baseline := groups[0]
	for _, target := range groups[1:] {
		metrics := make(map[string]Delta)
		addDelta(metrics, "tokensMean", baseline.Tokens.Mean, target.Tokens.Mean)
		addDelta(metrics, "tokensMedian", baseline.Tokens.Median, target.Tokens.Median)
		addDelta(metrics, "tokensP90", baseline.Tokens.P90, target.Tokens.P90)
		addDelta(metrics, "costMean", baseline.Cost.Mean, target.Cost.Mean)
		addDelta(metrics, "costMedian", baseline.Cost.Median, target.Cost.Median)
		addDelta(metrics, "costP90", baseline.Cost.P90, target.Cost.P90)
		addDelta(metrics, "elapsedMsMean", baseline.ElapsedMS.Mean, target.ElapsedMS.Mean)
		addDelta(metrics, "elapsedMsMedian", baseline.ElapsedMS.Median, target.ElapsedMS.Median)
		addDelta(metrics, "elapsedMsP90", baseline.ElapsedMS.P90, target.ElapsedMS.P90)
		addOptionalDelta(metrics, "successRate", baseline.SuccessRate, target.SuccessRate)
		addOptionalDelta(metrics, "tokensPerSuccessfulTask", baseline.TokensPerSuccessfulTask, target.TokensPerSuccessfulTask)
		addOptionalDelta(metrics, "costPerSuccessfulTask", baseline.CostPerSuccessfulTask, target.CostPerSuccessfulTask)
		addOptionalDelta(metrics, "timePerSuccessfulTask", baseline.TimePerSuccessfulTask, target.TimePerSuccessfulTask)
		comparisons = append(comparisons, Comparison{Baseline: baseline.Name, Target: target.Name, Metrics: metrics})
	}
	return comparisons
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
	for _, run := range runs {
		tokens = append(tokens, float64(run.Usage.TotalTokens))
		costs = append(costs, run.Usage.Cost)
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
	group.Tokens = distribution(tokens)
	group.Cost = distribution(costs)
	group.ElapsedMS = distribution(times)
	if group.OutcomeSampleSize > 0 {
		value := float64(group.Successful) / float64(group.OutcomeSampleSize)
		group.SuccessRate = &value
	}
	if group.Successful > 0 {
		tokensPerSuccess := totalOutcomeTokens / float64(group.Successful)
		costPerSuccess := totalOutcomeCost / float64(group.Successful)
		timePerSuccess := totalOutcomeTime / float64(group.Successful)
		group.TokensPerSuccessfulTask = &tokensPerSuccess
		group.CostPerSuccessfulTask = &costPerSuccess
		group.TimePerSuccessfulTask = &timePerSuccess
	}
	return group
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
