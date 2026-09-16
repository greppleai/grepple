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
	"time"

	agentmetrics "github.com/greppleai/grepple/internal/metrics"
)

type stringFlags []string

func (values *stringFlags) String() string { return strings.Join(*values, ",") }
func (values *stringFlags) Set(value string) error {
	*values = append(*values, value)
	return nil
}

type metricsOptions struct {
	inputs       stringFlags
	groupBy      string
	repository   string
	model        string
	task         string
	cohort       string
	baseline     string
	target       string
	since        time.Time
	until        time.Time
	onlyComplete bool
	format       string
}

const metricsHelp = `Record and analyze privacy-safe agent utility metrics in Grepple JSONL journals.

Usage:
  grepple metrics start --task ID --cohort NAME [--run ID]
  grepple metrics record --event TYPE --data JSON [--run ID]
  grepple metrics status
  grepple metrics end --outcome STATUS [--run ID]
  grepple metrics report [OPTIONS]
  grepple metrics compare --baseline NAME --target NAME [OPTIONS]
  grepple metrics export [OPTIONS]

Lifecycle options:
  --run ID              explicit run identifier
  --event-id ID         deterministic event identifier
  --at RFC3339          deterministic event time
  --repository NAME     repository label (start)
  --revision REVISION   repository revision (start)

Report and comparison options:
  --input PATH          journal JSONL file or directory (repeatable; default ~/.grepple/metrics)
  --group-by DIMENSION  cohort, observed, model, repository, or task (default cohort)
  --baseline NAME       explicit comparison baseline group
  --target NAME         explicit comparison target group
  --repository NAME     include one repository label
  --model NAME          include provider/model or model
  --task ID             include one task ID
  --cohort NAME         include one assigned cohort
  --since RFC3339       include runs ending at or after this time
  --until RFC3339       include runs starting at or before this time
  --complete            exclude incomplete runs
  --format FORMAT       text, json, or csv

Journals contain normalized opaque identifiers, counts, timestamps, and outcomes.
They never contain prompts, responses, source, tool output, raw queries, raw commands, or content-bearing paths.
`

func runMetrics(args []string) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		return stdoutWriter().writeString(metricsHelp)
	}
	command := args[0]
	switch command {
	case "start":
		return runMetricsStart(args[1:])
	case "record":
		return runMetricsRecord(args[1:])
	case "status":
		return runMetricsStatus(args[1:])
	case "end":
		return runMetricsEnd(args[1:])
	case "report", "export", "compare":
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
		comparison, err := agentmetrics.BuildComparison(runs, options.groupBy, options.baseline, options.target)
		if err != nil {
			return err
		}
		return outputMetricsComparison(comparison, options.format)
	}
	report := agentmetrics.BuildReport(runs, options.groupBy)
	if options.format != "text" {
		return exportMetrics(report, options.format)
	}
	return writeMetricsReport(report, options.groupBy)
}

func parseMetricsOptions(command string, args []string) (metricsOptions, bool, error) {
	defaultFormat := "text"
	if command == "export" {
		defaultFormat = "json"
	}
	options := metricsOptions{groupBy: "cohort", format: defaultFormat}
	flags := flag.NewFlagSet("grepple metrics "+command, flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	flags.Var(&options.inputs, "input", "metrics journal JSONL file or directory")
	flags.StringVar(&options.groupBy, "group-by", options.groupBy, "grouping dimension")
	flags.StringVar(&options.repository, "repository", "", "repository filter")
	flags.StringVar(&options.model, "model", "", "model filter")
	flags.StringVar(&options.task, "task", "", "task filter")
	flags.StringVar(&options.cohort, "cohort", "", "cohort filter")
	flags.StringVar(&options.baseline, "baseline", "", "baseline group name")
	flags.StringVar(&options.target, "target", "", "target group name")
	flags.BoolVar(&options.onlyComplete, "complete", false, "complete runs only")
	flags.StringVar(&options.format, "format", options.format, "text, json, or csv")
	var since string
	var until string
	flags.StringVar(&since, "since", "", "earliest run end")
	flags.StringVar(&until, "until", "", "latest run start")
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return options, true, nil
		}
		return options, false, err
	}
	if flags.NArg() != 0 {
		return options, false, fmt.Errorf("unexpected metrics arguments: %s", strings.Join(flags.Args(), " "))
	}
	validGroups := map[string]bool{"cohort": true, "observed": true, "model": true, "repository": true, "task": true}
	if !validGroups[options.groupBy] {
		return options, false, fmt.Errorf("--group-by must be cohort, observed, model, repository, or task")
	}
	if options.format != "text" && options.format != "json" && options.format != "csv" {
		return options, false, fmt.Errorf("--format must be text, json, or csv")
	}
	if command == "compare" && (options.baseline == "" || options.target == "") {
		return options, false, fmt.Errorf("metrics compare requires --baseline and --target")
	}
	var err error
	if since != "" {
		options.since, err = time.Parse(time.RFC3339, since)
		if err != nil {
			return options, false, fmt.Errorf("--since: %w", err)
		}
	}
	if until != "" {
		options.until, err = time.Parse(time.RFC3339, until)
		if err != nil {
			return options, false, fmt.Errorf("--until: %w", err)
		}
	}
	return options, false, nil
}

func loadMetricRuns(options metricsOptions) ([]agentmetrics.Run, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	paths, err := agentmetrics.DiscoverJournals(options.inputs, home)
	if err != nil {
		return nil, err
	}
	var result []agentmetrics.Run
	for _, path := range paths {
		runs, err := agentmetrics.AnalyzeFile(path)
		if err != nil {
			return nil, err
		}
		for _, run := range runs {
			if includeMetricRun(run, options) {
				result = append(result, run)
			}
		}
	}
	return result, nil
}

func includeMetricRun(run agentmetrics.Run, options metricsOptions) bool {
	if options.repository != "" && run.Repository != options.repository {
		return false
	}
	if options.model != "" && run.Model != options.model && run.Provider+"/"+run.Model != options.model {
		return false
	}
	if options.task != "" && run.TaskID != options.task {
		return false
	}
	if options.cohort != "" && run.AssignedCohort != options.cohort {
		return false
	}
	if options.onlyComplete && !run.Complete {
		return false
	}
	if !options.since.IsZero() && run.EndedAt.Before(options.since) {
		return false
	}
	return options.until.IsZero() || !run.StartedAt.After(options.until)
}

func writeMetricsReport(report agentmetrics.Report, groupBy string) error {
	var output strings.Builder
	fmt.Fprintf(&output, "Agent utility metrics (grouped by %s)\n", groupBy)
	fmt.Fprintf(&output, "runs: %d  groups: %d\n\n", len(report.Runs), len(report.Groups))
	fmt.Fprintln(&output, "GROUP\tN\tSUCCESS\tTOKENS p50/p90\tCOST p50/p90\tTIME-ms p50/p90")
	for _, group := range report.Groups {
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
		fmt.Fprintf(&output, "%s\t%d\t%s\t%s\t%s\t%.0f/%.0f\n", group.Name, group.SampleSize, success, tokens, cost, group.ElapsedMS.P50, group.ElapsedMS.P90)
		perSuccess := make([]string, 0, 3)
		if group.TokensPerSuccessfulTask != nil {
			perSuccess = append(perSuccess, fmt.Sprintf("tokens %.0f", *group.TokensPerSuccessfulTask), fmt.Sprintf("cost %.4f", *group.CostPerSuccessfulTask))
		}
		if group.TimePerSuccessfulTask != nil {
			perSuccess = append(perSuccess, fmt.Sprintf("time %.0fms", *group.TimePerSuccessfulTask))
		}
		if len(perSuccess) > 0 {
			fmt.Fprintf(&output, "  per successful task: %s\n", strings.Join(perSuccess, ", "))
		}
	}
	for _, comparison := range report.Comparisons {
		fmt.Fprintf(&output, "\nDescriptive deltas %s - %s (absolute, percent):", comparison.Target, comparison.Baseline)
		for _, metric := range []string{"tokensMedian", "costMedian", "elapsedMsMedian", "successRate", "tokensPerSuccessfulTask", "costPerSuccessfulTask", "timePerSuccessfulTask"} {
			if delta, ok := comparison.Metrics[metric]; ok {
				fmt.Fprintf(&output, " %s %s;", metric, formatMetricDelta(delta))
			}
		}
		fmt.Fprintln(&output, " no significance claim.")
	}
	return newBoundedOutputWriter(os.Stdout, DefaultTextOutputBytes).writeString(output.String())
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
	header := []string{"run_id", "task_id", "repository", "revision", "assigned_cohort", "observed_grepple_use", "complete", "outcome", "provider", "model", "turns", "tool_calls", "grepple_calls", "tokens", "cost", "elapsed_ms", "files_inspected", "files_edited", "test_fix_cycles"}
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
		if metricRunMissing(run, "model_usage") {
			tokens = ""
			cost = ""
		}
		row := []string{run.RunID, run.TaskID, run.Repository, run.Revision, run.AssignedCohort, strconv.FormatBool(run.ObservedGreppleUse), strconv.FormatBool(run.Complete), run.Outcome.Status, run.Provider, run.Model, turns, strconv.Itoa(run.Tools.Total), strconv.Itoa(run.Tools.Grepple), tokens, cost, strconv.FormatInt(elapsed, 10), strconv.Itoa(run.DistinctInspectedFiles), strconv.Itoa(run.DistinctEditedFiles), strconv.Itoa(run.TestFixCycles)}
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
	var output strings.Builder
	fmt.Fprintf(&output, "Agent utility comparison (%s): %s -> %s\n", report.GroupBy, report.Baseline.Name, report.Target.Name)
	for _, name := range metricNames {
		fmt.Fprintf(&output, "%s\t%s\n", name, formatMetricDelta(report.Delta.Metrics[name]))
	}
	fmt.Fprintln(&output, "Descriptive deltas only; no significance claim.")
	return newBoundedOutputWriter(os.Stdout, DefaultTextOutputBytes).writeString(output.String())
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
