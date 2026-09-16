package cli

import (
	"bytes"
	"encoding/csv"
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	agentmetrics "github.com/greppleai/grepple/internal/metrics"
)

type stringFlags []string

func (values *stringFlags) String() string { return strings.Join(*values, ",") }
func (values *stringFlags) Set(value string) error {
	*values = append(*values, value)
	return nil
}

type metricsOptions struct {
	inputs   stringFlags
	baseline string
	target   string
	format   string
}

const metricsHelp = `Analyze externally generated, privacy-safe agent utility JSONL.

Usage:
  grepple metrics report --input PATH [OPTIONS]
  grepple metrics compare --input PATH --baseline NAME --target NAME [OPTIONS]

Report options:
  --input PATH          journal JSONL file or directory (required; repeatable)
  --format FORMAT       text, json, or csv

Compare options:
  --input PATH          journal JSONL file or directory (required; repeatable)
  --baseline AGENT      producing agent used as the comparison baseline
  --target AGENT        producing agent used as the comparison target
  --format FORMAT       text, json, or csv

Grepple reads only explicit --input paths; it does not collect, write, or discover journals.
Journals contain normalized opaque identifiers, counts, timestamps, and outcomes.
They never contain prompts, responses, source, tool output, raw queries, raw commands, command arguments, or content-bearing paths.
`

func runMetrics(args []string) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		return stdoutWriter().writeString(metricsHelp)
	}
	command := args[0]
	switch command {
	case "report", "compare":
		// Continue with report option parsing below.
	default:
		return fmt.Errorf("unknown metrics command %q", command)
	}
	options, help, err := parseMetricsOptions(command, args[1:])
	if err != nil {
		return err
	}
	if help {
		return stdoutWriter().writeString(metricsHelp)
	}
	runs, err := loadMetricRuns(options)
	if err != nil {
		return err
	}
	if command == "compare" {
		comparison, err := agentmetrics.BuildComparison(runs, options.baseline, options.target)
		if err != nil {
			return err
		}
		return outputMetricsComparison(comparison, options.format)
	}
	report := agentmetrics.BuildReport(runs)
	if options.format != "text" {
		return exportMetrics(report, options.format)
	}
	return writeMetricsReport(report)
}

func parseMetricsOptions(command string, args []string) (metricsOptions, bool, error) {
	options := metricsOptions{format: "text"}
	flags := flag.NewFlagSet("grepple metrics "+command, flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	flags.Var(&options.inputs, "input", "metrics journal JSONL file or directory")
	flags.StringVar(&options.format, "format", options.format, "text, json, or csv")
	if command == "compare" {
		flags.StringVar(&options.baseline, "baseline", "", "baseline agent name")
		flags.StringVar(&options.target, "target", "", "target agent name")
	}
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return options, true, nil
		}
		return options, false, err
	}
	if flags.NArg() != 0 {
		return options, false, fmt.Errorf("unexpected metrics arguments: %s", strings.Join(flags.Args(), " "))
	}
	if options.format != "text" && options.format != "json" && options.format != "csv" {
		return options, false, fmt.Errorf("--format must be text, json, or csv")
	}
	if len(options.inputs) == 0 {
		return options, false, fmt.Errorf("metrics %s requires --input", command)
	}
	if command == "compare" && (options.baseline == "" || options.target == "") {
		return options, false, fmt.Errorf("metrics compare requires --baseline and --target")
	}
	return options, false, nil
}

func loadMetricRuns(options metricsOptions) ([]agentmetrics.Run, error) {
	paths, err := agentmetrics.DiscoverJournals(options.inputs)
	if err != nil {
		return nil, err
	}
	var result []agentmetrics.Run
	for _, path := range paths {
		runs, err := agentmetrics.AnalyzeFile(path)
		if err != nil {
			return nil, err
		}
		result = append(result, runs...)
	}
	return result, nil
}

func writeMetricsReport(report agentmetrics.Report) error {
	var output strings.Builder
	fmt.Fprintln(&output, "Agent utility metrics (grouped by agent)")
	fmt.Fprintf(&output, "runs: %d  groups: %d\n\n", len(report.Runs), len(report.Groups))
	fmt.Fprintln(&output, "AGENT\tN\tSUCCESS\tTOKENS p50/p90\tCOST p50/p90\tTIME-ms p50/p90")
	for _, group := range report.Groups {
		writeMetricsGroup(&output, group)
	}
	for _, comparison := range report.Comparisons {
		writeMetricsReportComparison(&output, comparison)
	}
	return newBoundedOutputWriter(os.Stdout, DefaultTextOutputBytes).writeString(output.String())
}

func writeMetricsGroup(output *strings.Builder, group agentmetrics.Group) {
	success := "unknown"
	if group.SuccessRate != nil {
		success = fmt.Sprintf("%.1f%% (%d/%d)", *group.SuccessRate*100, group.Successful, group.OutcomeSampleSize)
	}
	tokens := "unknown"
	cost := "unknown"
	if group.UsageSampleSize > 0 {
		tokens = fmt.Sprintf("%.0f/%.0f", group.Tokens.P50, group.Tokens.P90)
		cost = fmt.Sprintf("%.4f/%.4f", group.Cost.P50, group.Cost.P90)
	}
	fmt.Fprintf(output, "%s\t%d\t%s\t%s\t%s\t%.0f/%.0f\n", group.Name, group.SampleSize, success, tokens, cost, group.ElapsedMS.P50, group.ElapsedMS.P90)
	perSuccess := make([]string, 0, 3)
	if group.TokensPerSuccessfulTask != nil {
		perSuccess = append(perSuccess, fmt.Sprintf("tokens %.0f", *group.TokensPerSuccessfulTask), fmt.Sprintf("cost %.4f", *group.CostPerSuccessfulTask))
	}
	if group.TimePerSuccessfulTask != nil {
		perSuccess = append(perSuccess, fmt.Sprintf("time %.0fms", *group.TimePerSuccessfulTask))
	}
	if len(perSuccess) > 0 {
		fmt.Fprintf(output, "  per successful task: %s\n", strings.Join(perSuccess, ", "))
	}
}

func writeMetricsReportComparison(output *strings.Builder, comparison agentmetrics.Comparison) {
	fmt.Fprintf(output, "\nDescriptive deltas %s - %s (absolute, percent):", comparison.Target, comparison.Baseline)
	for _, metric := range []string{"tokensMedian", "costMedian", "elapsedMsMedian", "successRate", "tokensPerSuccessfulTask", "costPerSuccessfulTask", "timePerSuccessfulTask"} {
		if delta, ok := comparison.Metrics[metric]; ok {
			fmt.Fprintf(output, " %s %s;", metric, formatMetricDelta(delta))
		}
	}
	fmt.Fprintln(output, " no significance claim.")
}

func formatMetricDelta(delta agentmetrics.Delta) string {
	percent := "n/a"
	if delta.Percent != nil {
		percent = fmt.Sprintf("%+.1f%%", *delta.Percent)
	}
	return fmt.Sprintf("%+.4g (%s)", delta.Absolute, percent)
}

func exportMetrics(report agentmetrics.Report, format string) error {
	if format == "json" {
		return stdoutWriter().writeJSON(report)
	}
	var output bytes.Buffer
	writer := csv.NewWriter(&output)
	header := []string{"run_id", "agent", "observed_grepple_use", "complete", "outcome", "turns", "tool_calls", "grepple_calls", "tokens", "cost", "elapsed_ms", "files_inspected", "files_edited", "test_fix_cycles"}
	if err := writer.Write(header); err != nil {
		return err
	}
	for _, run := range report.Runs {
		elapsed := run.EndedAt.Sub(run.StartedAt).Milliseconds()
		turns := strconv.Itoa(run.Turns)
		if metricRunMissing(run, "agent_turns") {
			turns = ""
		}
		tokens := strconv.FormatInt(run.Usage.TotalTokens, 10)
		cost := strconv.FormatFloat(run.Usage.Cost, 'f', 6, 64)
		if metricRunMissing(run, "assistant_usage") {
			tokens = ""
			cost = ""
		}
		row := []string{run.RunID, run.Agent, strconv.FormatBool(run.ObservedGreppleUse), strconv.FormatBool(run.Complete), run.Outcome.Status, turns, strconv.Itoa(run.Tools.Total), strconv.Itoa(run.Tools.Grepple), tokens, cost, strconv.FormatInt(elapsed, 10), strconv.Itoa(run.DistinctInspectedFiles), strconv.Itoa(run.DistinctEditedFiles), strconv.Itoa(run.TestFixCycles)}
		if err := writer.Write(row); err != nil {
			return err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return err
	}
	return stdoutWriter().writeString(output.String())
}

func metricRunMissing(run agentmetrics.Run, evidence string) bool {
	for _, missing := range run.Missing {
		if missing == evidence {
			return true
		}
	}
	return false
}

func outputMetricsComparison(report agentmetrics.ComparisonReport, format string) error {
	if format == "json" {
		return stdoutWriter().writeJSON(report)
	}
	metricNames := make([]string, 0, len(report.Delta.Metrics))
	for name := range report.Delta.Metrics {
		metricNames = append(metricNames, name)
	}
	sort.Strings(metricNames)
	if format == "csv" {
		return writeMetricsComparisonCSV(report, metricNames)
	}
	var output strings.Builder
	fmt.Fprintf(&output, "Agent utility comparison: %s -> %s\n", report.Baseline.Name, report.Target.Name)
	for _, name := range metricNames {
		fmt.Fprintf(&output, "%s\t%s\n", name, formatMetricDelta(report.Delta.Metrics[name]))
	}
	fmt.Fprintln(&output, "Descriptive deltas only; no significance claim.")
	return newBoundedOutputWriter(os.Stdout, DefaultTextOutputBytes).writeString(output.String())
}

func writeMetricsComparisonCSV(report agentmetrics.ComparisonReport, metricNames []string) error {
	var output bytes.Buffer
	writer := csv.NewWriter(&output)
	if err := writer.Write([]string{"metric", "baseline", "target", "absolute", "percent"}); err != nil {
		return err
	}
	for _, name := range metricNames {
		delta := report.Delta.Metrics[name]
		percent := ""
		if delta.Percent != nil {
			percent = strconv.FormatFloat(*delta.Percent, 'g', -1, 64)
		}
		row := []string{name, strconv.FormatFloat(comparisonMetricValue(report.Baseline, name), 'g', -1, 64), strconv.FormatFloat(comparisonMetricValue(report.Target, name), 'g', -1, 64), strconv.FormatFloat(delta.Absolute, 'g', -1, 64), percent}
		if err := writer.Write(row); err != nil {
			return err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return err
	}
	return stdoutWriter().writeString(output.String())
}

func comparisonMetricValue(group agentmetrics.Group, name string) float64 {
	switch name {
	case "tokensMean":
		return group.Tokens.Mean
	case "tokensMedian":
		return group.Tokens.Median
	case "tokensP90":
		return group.Tokens.P90
	case "costMean":
		return group.Cost.Mean
	case "costMedian":
		return group.Cost.Median
	case "costP90":
		return group.Cost.P90
	case "elapsedMsMean":
		return group.ElapsedMS.Mean
	case "elapsedMsMedian":
		return group.ElapsedMS.Median
	case "elapsedMsP90":
		return group.ElapsedMS.P90
	case "successRate":
		return *group.SuccessRate
	case "tokensPerSuccessfulTask":
		return *group.TokensPerSuccessfulTask
	case "costPerSuccessfulTask":
		return *group.CostPerSuccessfulTask
	case "timePerSuccessfulTask":
		return *group.TimePerSuccessfulTask
	default:
		return 0
	}
}
