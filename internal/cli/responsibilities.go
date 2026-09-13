package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/alexflint/go-arg"
	"github.com/greppleai/grepple/parser"
	"github.com/greppleai/grepple/search"
)

type responsibilitiesArgs struct {
	JSON           bool     `arg:"--json" help:"emit the complete responsibility report as JSON"`
	MinOccurrences int      `arg:"--min-occurrences" placeholder:"N" help:"minimum callers sharing a reported pattern (default 2)"`
	MaxFiles       int      `arg:"--max-files" placeholder:"N" help:"parse at most N discovered files (0 = unlimited)"`
	MaxOutputBytes int      `arg:"--max-output-bytes" placeholder:"N" help:"cap human output (default 16384; 0 = unlimited; JSON is uncapped)"`
	NoCache        bool     `arg:"--no-cache" help:"bypass the content-addressed .grepple responsibility cache"`
	Type           string   `arg:"positional" placeholder:"TYPE" help:"exact receiver or container type name"`
	Paths          []string `arg:"positional" placeholder:"PATH" help:"file, directory, or glob to include; defaults to the working directory"`
}

func (responsibilitiesArgs) Description() string {
	return "Detect deterministic cross-function responsibility patterns from type method interactions in every navigation-supported language."
}

type responsibilitiesOutput struct {
	search.ResponsibilityReport
	Files      int                        `json:"files"`
	Truncation *navigationGraphTruncation `json:"truncation,omitempty"`
}

func runResponsibilities(args []string) error {
	values := responsibilitiesArgs{MinOccurrences: 2, MaxOutputBytes: DefaultTextOutputBytes}
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
	if values.Type == "" {
		return fmt.Errorf("grepple responsibilities requires TYPE")
	}
	if values.MinOccurrences < 1 {
		return fmt.Errorf("--min-occurrences must be positive")
	}
	if values.MaxFiles < 0 || values.MaxOutputBytes < 0 {
		return fmt.Errorf("--max-files and --max-output-bytes must be non-negative")
	}
	graphOutput, _, err := buildCachedResponsibilityGraph(values.Paths, values.MaxFiles, !values.NoCache)
	if err != nil {
		return err
	}
	report, err := search.AnalyzeResponsibilities(parser.NavigationGraph{Declarations: graphOutput.Declarations, Calls: graphOutput.Calls, TypeUsages: graphOutput.TypeUsages, MemberAccesses: graphOutput.MemberAccesses}, values.Type, values.MinOccurrences)
	if err != nil {
		return err
	}
	output := responsibilitiesOutput{ResponsibilityReport: report, Files: graphOutput.Files, Truncation: graphOutput.Truncation}
	if values.JSON {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetEscapeHTML(false)
		encoder.SetIndent("", "  ")
		return encoder.Encode(output)
	}
	return renderResponsibilities(output, values.MinOccurrences, values.MaxOutputBytes)
}

func renderResponsibilities(report responsibilitiesOutput, minimum, maxBytes int) error {
	output := stdoutWriter()
	if maxBytes > 0 {
		output = newBoundedOutputWriter(os.Stdout, maxBytes)
	}
	write := func(format string, values ...any) bool {
		return output.writeString(fmt.Sprintf(format, values...)+"\n") == nil
	}
	if !write("%s", report.Type) ||
		!write("  languages: %s", strings.Join(report.Languages, ", ")) ||
		!write("  consumers: %d functions / %d files / %d packages", report.Consumers.Functions, report.Consumers.Files, report.Consumers.Packages) ||
		!write("  external method surface: %d/%d methods", report.ExternalMethodSurface.External, report.ExternalMethodSurface.Declared) ||
		!write("  analyzed files: %d", report.Files) {
		return nil
	}
	if report.Truncation != nil && !write("  ! incomplete: %d files omitted by %s=%d", report.Truncation.Skipped, report.Truncation.Reason, report.Truncation.Limit) {
		return nil
	}
	if report.UnresolvedCalls > 0 && !write("  ! %d matched interactions use ambiguous or receiver-only resolution", report.UnresolvedCalls) {
		return nil
	}
	if !write("") || !write("Repeated method co-usage (minimum %d callers):", minimum) {
		return nil
	}
	if !writeResponsibilityPatterns(write, report.MethodCoUsage, " + ") {
		return nil
	}
	if !write("") || !write("Repeated ordered method sequences (minimum %d callers):", minimum) {
		return nil
	}
	if !writeResponsibilityPatterns(write, report.OrderedSequences, " -> ") {
		return nil
	}
	if !write("") || !write("Repeated member + method combinations (minimum %d callers):", minimum) {
		return nil
	}
	writeResponsibilityPatterns(write, report.MemberMethodCombinations, " + ")
	return nil
}

func writeResponsibilityPatterns(write func(string, ...any) bool, patterns []search.ResponsibilityPattern, separator string) bool {
	if len(patterns) == 0 {
		return write("  (none)")
	}
	for _, pattern := range patterns {
		if !write("  %s", strings.Join(pattern.Methods, separator)) ||
			!write("    %d occurrences / %d files / %d packages", pattern.Occurrences, pattern.Files, pattern.Packages) {
			return false
		}
	}
	return true
}
