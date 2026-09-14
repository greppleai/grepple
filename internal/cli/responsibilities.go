package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alexflint/go-arg"
	"github.com/greppleai/grepple/parser"
	"github.com/greppleai/grepple/search"
)

const maxHumanResponsibilityPatterns = 5

type responsibilitiesArgs struct {
	JSON           bool     `arg:"--json" help:"emit the complete responsibility report as JSON"`
	MinOccurrences int      `arg:"--min-occurrences" placeholder:"N" help:"minimum callers sharing a reported pattern (default 2)"`
	MaxFiles       int      `arg:"--max-files" placeholder:"N" help:"parse at most N discovered files (0 = unlimited)"`
	Limit          int      `arg:"--limit" placeholder:"N" help:"maximum type reports in human directory output (default 20; 0 = unlimited; JSON is complete)"`
	MaxOutputBytes int      `arg:"--max-output-bytes" placeholder:"N" help:"cap human output (default 16384; 0 = unlimited; JSON is uncapped)"`
	NoCache        bool     `arg:"--no-cache" help:"bypass the content-addressed .grepple responsibility cache"`
	Type           string   `arg:"--type" placeholder:"TYPE" help:"analyze one exact receiver or container type instead of discovering the directory"`
	Paths          []string `arg:"positional" placeholder:"PATH" help:"file, directory, or glob to include; defaults to the working directory"`
}

func (responsibilitiesArgs) Description() string {
	return "Rank deterministic cross-function responsibility patterns within one or more directories for every navigation-supported language."
}

type responsibilitiesOutput struct {
	search.ResponsibilityReport
	Files      int                        `json:"files"`
	Truncation *navigationGraphTruncation `json:"truncation,omitempty"`
}

type directoryResponsibilitiesOutput struct {
	Schema     string                        `json:"schema"`
	Paths      []string                      `json:"paths"`
	Files      int                           `json:"files"`
	Reports    []search.ResponsibilityReport `json:"reports"`
	Truncation *navigationGraphTruncation    `json:"truncation,omitempty"`
}

func runResponsibilities(args []string) error {
	values := responsibilitiesArgs{MinOccurrences: 2, MaxOutputBytes: DefaultTextOutputBytes, Limit: 20}
	argumentParser, err := arg.NewParser(arg.Config{Program: "grepple responsibilities"}, &values)
	if err != nil {
		return err
	}
	if err := argumentParser.Parse(args); err != nil {
		if errors.Is(err, arg.ErrHelp) {
			argumentParser.WriteHelp(os.Stdout)
			return nil
		}
		return err
	}
	if values.MinOccurrences < 1 {
		return fmt.Errorf("--min-occurrences must be positive")
	}
	if values.MaxFiles < 0 || values.MaxOutputBytes < 0 || values.Limit < 0 {
		return fmt.Errorf("--max-files, --max-output-bytes, and --limit must be non-negative")
	}
	graphOutput, _, err := buildCachedResponsibilityGraph(values.Paths, values.MaxFiles, !values.NoCache)
	if err != nil {
		return err
	}
	graph := parser.NavigationGraph{Declarations: graphOutput.Declarations, Calls: graphOutput.Calls, TypeUsages: graphOutput.TypeUsages, MemberAccesses: graphOutput.MemberAccesses}
	if values.Type != "" {
		report, err := search.AnalyzeResponsibilities(graph, values.Type, values.MinOccurrences)
		if err != nil {
			return err
		}
		output := responsibilitiesOutput{ResponsibilityReport: report, Files: graphOutput.Files, Truncation: graphOutput.Truncation}
		if values.JSON {
			return encodeResponsibilitiesJSON(output)
		}
		return renderResponsibilities(output, values.MinOccurrences, values.MaxOutputBytes)
	}
	reports, err := search.AnalyzeDirectoryResponsibilities(graph, values.MinOccurrences)
	if err != nil {
		return err
	}
	output := directoryResponsibilitiesOutput{Schema: "grepple-directory-responsibilities-v1", Paths: responsibilityDisplayPaths(values.Paths), Files: graphOutput.Files, Reports: reports, Truncation: graphOutput.Truncation}
	if values.JSON {
		return encodeResponsibilitiesJSON(output)
	}
	return renderDirectoryResponsibilities(output, values.MinOccurrences, values.Limit, values.MaxOutputBytes)
}

func encodeResponsibilitiesJSON(value any) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func responsibilityDisplayPaths(paths []string) []string {
	if len(paths) == 0 {
		return []string{"."}
	}
	unique := make(map[string]bool, len(paths))
	for _, path := range paths {
		unique[filepath.ToSlash(filepath.Clean(path))] = true
	}
	normalized := make([]string, 0, len(unique))
	for path := range unique {
		normalized = append(normalized, path)
	}
	sort.Strings(normalized)
	return normalized
}

func renderResponsibilities(report responsibilitiesOutput, minimum, maxBytes int) error {
	output := stdoutWriter()
	if maxBytes > 0 {
		output = newBoundedOutputWriter(os.Stdout, maxBytes)
	}
	write := func(format string, values ...any) bool {
		return output.writeString(fmt.Sprintf(format, values...)+"\n") == nil
	}
	writeResponsibilityReport(write, report.ResponsibilityReport, minimum, report.Files, report.Truncation)
	return nil
}

func renderDirectoryResponsibilities(directory directoryResponsibilitiesOutput, minimum, limit, maxBytes int) error {
	output := stdoutWriter()
	if maxBytes > 0 {
		output = newBoundedOutputWriter(os.Stdout, maxBytes)
	}
	write := func(format string, values ...any) bool {
		return output.writeString(fmt.Sprintf(format, values...)+"\n") == nil
	}
	visible := len(directory.Reports)
	if limit > 0 && visible > limit {
		visible = limit
	}
	if !write("responsibilities paths=%s files=%d types=%d shown=%d", strings.Join(directory.Paths, ","), directory.Files, len(directory.Reports), visible) {
		return nil
	}
	if directory.Truncation != nil && !write("! incomplete: %d files omitted by %s=%d", directory.Truncation.Skipped, directory.Truncation.Reason, directory.Truncation.Limit) {
		return nil
	}
	for index, report := range directory.Reports[:visible] {
		if index > 0 && !write("") {
			return nil
		}
		if !writeResponsibilityReport(write, report, minimum, 0, nil) {
			return nil
		}
	}
	if omitted := len(directory.Reports) - visible; omitted > 0 {
		write("\n! omitted %d lower-ranked types; use --limit 0 or --json for all reports", omitted)
	}
	if len(directory.Reports) == 0 {
		write("(no type responsibility evidence found)")
	}
	return nil
}

func writeResponsibilityReport(write func(string, ...any) bool, report search.ResponsibilityReport, minimum, files int, truncation *navigationGraphTruncation) bool {
	if !write("%s", report.Type) ||
		!write("  languages: %s", strings.Join(report.Languages, ", ")) ||
		!write("  consumers: %d functions / %d files / %d packages", report.Consumers.Functions, report.Consumers.Files, report.Consumers.Packages) ||
		!write("  external method surface: %d/%d methods", report.ExternalMethodSurface.External, report.ExternalMethodSurface.Declared) {
		return false
	}
	if files > 0 && !write("  analyzed files: %d", files) {
		return false
	}
	if truncation != nil && !write("  ! incomplete: %d files omitted by %s=%d", truncation.Skipped, truncation.Reason, truncation.Limit) {
		return false
	}
	if report.UnresolvedCalls > 0 && !write("  ! %d matched interactions use ambiguous or receiver-only resolution", report.UnresolvedCalls) {
		return false
	}
	if !write("  repeated method co-usage (minimum %d callers):", minimum) || !writeResponsibilityPatterns(write, report.MethodCoUsage, " + ") {
		return false
	}
	if !write("  repeated ordered method sequences (minimum %d callers):", minimum) || !writeResponsibilityPatterns(write, report.OrderedSequences, " -> ") {
		return false
	}
	if !write("  repeated member + method combinations (minimum %d callers):", minimum) {
		return false
	}
	return writeResponsibilityPatterns(write, report.MemberMethodCombinations, " + ")
}

func writeResponsibilityPatterns(write func(string, ...any) bool, patterns []search.ResponsibilityPattern, separator string) bool {
	if len(patterns) == 0 {
		return write("    (none)")
	}
	visible := len(patterns)
	if visible > maxHumanResponsibilityPatterns {
		visible = maxHumanResponsibilityPatterns
	}
	for _, pattern := range patterns[:visible] {
		if !write("    %s", strings.Join(pattern.Methods, separator)) ||
			!write("      %d occurrences / %d files / %d packages", pattern.Occurrences, pattern.Files, pattern.Packages) {
			return false
		}
	}
	if omitted := len(patterns) - visible; omitted > 0 {
		return write("    +%d additional patterns; use --json for complete results", omitted)
	}
	return true
}
