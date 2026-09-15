package cli

import (
	"bytes"
	"encoding/csv"
	"flag"
	"fmt"
	"os"
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
	leaf         string
	groupBy      string
	repository   string
	model        string
	task         string
	cohort       string
	since        time.Time
	until        time.Time
	onlyComplete bool
	format       string
}

const metricsHelp = `Analyze local Pi session metrics without copying prompt, code, or tool content.

Usage:
  grepple metrics report [OPTIONS]
  grepple metrics export [OPTIONS]

Options:
  --input PATH          session JSONL file or directory (repeatable; default ~/.pi/agent/sessions)
  --leaf ID             select an explicit leaf (requires exactly one session)
  --group-by DIMENSION  cohort, observed, model, repository, or task (default cohort)
  --repository NAME     include one repository basename
  --model NAME          include provider/model or model
  --task ID             include one task ID
  --cohort NAME         include one assigned cohort
  --since RFC3339       include runs ending at or after this time
  --until RFC3339       include runs starting at or before this time
  --complete            exclude incomplete runs
  --format FORMAT       export format: json or csv (default json)
`

func runMetrics(args []string) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		return stdoutWriter().writeString(metricsHelp)
	}
	command := args[0]
	if command != "report" && command != "export" {
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
	report := agentmetrics.BuildReport(runs, options.groupBy, time.Now())
	if command == "export" {
		return exportMetrics(report, options.format)
	}
	return writeMetricsReport(report, options.groupBy)
}

func parseMetricsOptions(command string, args []string) (metricsOptions, bool, error) {
	options := metricsOptions{groupBy: "cohort", format: "json"}
	flags := flag.NewFlagSet("grepple metrics "+command, flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	flags.Var(&options.inputs, "input", "session JSONL file or directory")
	flags.StringVar(&options.leaf, "leaf", "", "explicit branch leaf")
	flags.StringVar(&options.groupBy, "group-by", options.groupBy, "grouping dimension")
	flags.StringVar(&options.repository, "repository", "", "repository filter")
	flags.StringVar(&options.model, "model", "", "model filter")
	flags.StringVar(&options.task, "task", "", "task filter")
	flags.StringVar(&options.cohort, "cohort", "", "cohort filter")
	flags.BoolVar(&options.onlyComplete, "complete", false, "complete runs only")
	flags.StringVar(&options.format, "format", options.format, "json or csv")
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
	if options.format != "json" && options.format != "csv" {
		return options, false, fmt.Errorf("--format must be json or csv")
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
	paths, err := agentmetrics.DiscoverSessions(options.inputs, home)
	if err != nil {
		return nil, err
	}
	if options.leaf != "" && len(paths) != 1 {
		return nil, fmt.Errorf("--leaf requires exactly one discovered session")
	}
	var result []agentmetrics.Run
	for _, path := range paths {
		runs, err := agentmetrics.AnalyzeFile(path, options.leaf)
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
		fmt.Fprintf(&output, "%s\t%d\t%s\t%.0f/%.0f\t%.4f/%.4f\t%.0f/%.0f\n", group.Name, group.SampleSize, success, group.Tokens.P50, group.Tokens.P90, group.Cost.P50, group.Cost.P90, group.ElapsedMS.P50, group.ElapsedMS.P90)
		if group.TokensPerSuccessfulTask != nil {
			fmt.Fprintf(&output, "  per successful task: tokens %.0f, cost %.4f, time %.0fms\n", *group.TokensPerSuccessfulTask, *group.CostPerSuccessfulTask, *group.TimePerSuccessfulTask)
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
	header := []string{"session_id", "run_id", "task_id", "repository", "revision", "assigned_cohort", "observed_grepple_use", "complete", "outcome", "provider", "model", "turns", "tool_calls", "grepple_calls", "tokens", "cost", "elapsed_ms", "files_inspected", "files_edited", "test_fix_cycles"}
	if err := writer.Write(header); err != nil {
		return err
	}
	for _, run := range report.Runs {
		elapsed := run.EndedAt.Sub(run.StartedAt).Milliseconds()
		row := []string{run.SessionID, run.RunID, run.TaskID, run.Repository, run.Revision, run.AssignedCohort, strconv.FormatBool(run.ObservedGreppleUse), strconv.FormatBool(run.Complete), run.Outcome.Status, run.Provider, run.Model, strconv.Itoa(run.Turns), strconv.Itoa(run.Tools.Total), strconv.Itoa(run.Tools.Grepple), strconv.FormatInt(run.Usage.TotalTokens, 10), strconv.FormatFloat(run.Usage.Cost, 'f', 6, 64), strconv.FormatInt(elapsed, 10), strconv.Itoa(run.DistinctInspectedFiles), strconv.Itoa(run.DistinctEditedFiles), strconv.Itoa(run.TestFixCycles)}
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
