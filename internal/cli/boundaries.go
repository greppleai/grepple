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

const maxHumanBoundaryPatterns = 5

type boundariesArgs struct {
	JSON           bool     `arg:"--json" help:"emit the complete boundary report as JSON"`
	MinOccurrences int      `arg:"--min-occurrences" placeholder:"N" help:"minimum callers sharing a reported pattern (default 2)"`
	MaxFiles       int      `arg:"--max-files" placeholder:"N" help:"parse at most N discovered files (0 = unlimited)"`
	Limit          int      `arg:"--limit" placeholder:"N" help:"maximum candidates per human-output section (default 20; 0 = unlimited; JSON is complete)"`
	MaxOutputBytes int      `arg:"--max-output-bytes" placeholder:"N" help:"cap human output (default 16384; 0 = unlimited; JSON is uncapped)"`
	NoCache        bool     `arg:"--no-cache" help:"bypass the content-addressed .grepple boundary cache"`
	Paths          []string `arg:"positional" placeholder:"PATH" help:"file, directory, or glob to include; defaults to the working directory"`
}

func (boundariesArgs) Description() string {
	return "Find repeated owner-file workflows and concrete type spread across source boundaries."
}

type boundariesOutput struct {
	Schema         string                      `json:"schema"`
	Paths          []string                    `json:"paths"`
	Files          int                         `json:"files"`
	Candidates     []search.BoundaryCandidate  `json:"candidates"`
	TypeBoundaries []search.BoundaryTypeSpread `json:"typeBoundaries"`
	Truncation     *navigationGraphTruncation  `json:"truncation,omitempty"`
}

func runBoundaries(args []string) error {
	values := boundariesArgs{MinOccurrences: 2, MaxOutputBytes: DefaultTextOutputBytes, Limit: 20}
	argumentParser, err := arg.NewParser(arg.Config{Program: "grepple boundaries"}, &values)
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
	graphOutput, _, err := buildCachedBoundaryGraph(values.Paths, values.MaxFiles, !values.NoCache)
	if err != nil {
		return err
	}
	graph := parser.NavigationGraph{Declarations: graphOutput.Declarations, Calls: graphOutput.Calls, TypeUsages: graphOutput.TypeUsages, MemberAccesses: graphOutput.MemberAccesses}
	candidates, err := search.AnalyzeBoundaries(graph, values.MinOccurrences)
	if err != nil {
		return err
	}
	typeBoundaries, err := search.AnalyzeTypeBoundaries(graph, values.MinOccurrences)
	if err != nil {
		return err
	}
	output := boundariesOutput{Schema: "grepple-boundaries-v2", Paths: boundaryDisplayPaths(values.Paths), Files: graphOutput.Files, Candidates: candidates, TypeBoundaries: typeBoundaries, Truncation: graphOutput.Truncation}
	if values.JSON {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetEscapeHTML(false)
		encoder.SetIndent("", "  ")
		return encoder.Encode(output)
	}
	return renderBoundaries(output, values.MinOccurrences, values.Limit, values.MaxOutputBytes)
}

func boundaryDisplayPaths(paths []string) []string {
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

func renderBoundaries(report boundariesOutput, minimum, limit, maxBytes int) error {
	output := stdoutWriter()
	if maxBytes > 0 {
		output = newBoundedOutputWriter(os.Stdout, maxBytes)
	}
	write := func(format string, values ...any) bool {
		return output.writeString(fmt.Sprintf(format, values...)+"\n") == nil
	}
	visible := boundaryVisibleCount(len(report.Candidates), limit)
	typeVisible := boundaryVisibleCount(len(report.TypeBoundaries), limit)
	if !write("boundary analysis paths=%s files=%d workflow-candidates=%d workflow-shown=%d type-candidates=%d type-shown=%d", strings.Join(report.Paths, ","), report.Files, len(report.Candidates), visible, len(report.TypeBoundaries), typeVisible) {
		return nil
	}
	if report.Truncation != nil && !write("! incomplete: %d files omitted by %s=%d", report.Truncation.Skipped, report.Truncation.Reason, report.Truncation.Limit) {
		return nil
	}
	for index, candidate := range report.Candidates[:visible] {
		if index > 0 && !write("") {
			return nil
		}
		if !writeBoundaryCandidate(write, candidate, minimum) {
			return nil
		}
	}
	if omitted := len(report.Candidates) - visible; omitted > 0 {
		write("\n! omitted %d lower-ranked workflow candidates; use --limit 0 or --json for all", omitted)
	}
	if !writeBoundaryTypeSpreads(write, report.TypeBoundaries, limit) {
		return nil
	}
	if len(report.Candidates) == 0 && len(report.TypeBoundaries) == 0 {
		write("(no repeated cross-file workflow or type boundary patterns found)")
	}
	return nil
}

func writeBoundaryTypeSpreads(write func(string, ...any) bool, spreads []search.BoundaryTypeSpread, limit int) bool {
	if !write("\ntype boundary spread:") {
		return false
	}
	if len(spreads) == 0 {
		return write("  (none)")
	}
	visible := boundaryVisibleCount(len(spreads), limit)
	for _, spread := range spreads[:visible] {
		if !writeBoundaryTypeSpread(write, spread) {
			return false
		}
	}
	if omitted := len(spreads) - visible; omitted > 0 {
		return write("  +%d additional type boundary candidates; use --limit 0 or --json for all", omitted)
	}
	return true
}

func boundaryVisibleCount(count, limit int) int {
	if limit > 0 && count > limit {
		return limit
	}
	return count
}

func writeBoundaryTypeSpread(write func(string, ...any) bool, spread search.BoundaryTypeSpread) bool {
	origin := string(spread.Origin)
	if origin == "" {
		origin = "unresolved"
	}
	if !write("  type: %s [%s, origin=%s]", spread.CanonicalType, spread.Language, origin) ||
		!write("    reach: %d usages / %d functions / %d files / %d packages", spread.Usages, spread.Consumers.Functions, spread.Consumers.Files, spread.Consumers.Packages) ||
		!write("    source split: %d production files / %d test files", spread.Production.Files, spread.Tests.Files) ||
		!write("    roles: %d parameters / %d results / %d receivers / %d locals / %d unknown", spread.Roles.Parameters, spread.Roles.Results, spread.Roles.Receivers, spread.Roles.Locals, spread.Roles.Unknown) {
		return false
	}
	if spread.OwnerFile != "" && !write("    owner: %s", spread.OwnerFile) {
		return false
	}
	if len(spread.PublicExposures) > 0 && !writeBoundaryTypePublicExposures(write, spread) {
		return false
	}
	return write("    usages: %s", boundaryTypeLocations(spread.UsageDetails))
}

func writeBoundaryTypePublicExposures(write func(string, ...any) bool, spread search.BoundaryTypeSpread) bool {
	label := "public signature uses"
	prefix := ""
	if spread.External {
		label = "public external-type exposures"
		prefix = "! "
	}
	return write("    %s%d %s: %s", prefix, len(spread.PublicExposures), label, boundaryTypeLocations(spread.PublicExposures))
}

func boundaryTypeLocations(usages []search.BoundaryTypeUsage) string {
	const maximum = 3
	count := len(usages)
	if count > maximum {
		count = maximum
	}
	locations := make([]string, 0, count+1)
	for _, usage := range usages[:count] {
		suffix := usage.Role
		if usage.Test {
			suffix += ", test"
		}
		locations = append(locations, fmt.Sprintf("%s:%d %s (%s)", usage.Path, usage.Line, usage.Name, suffix))
	}
	if omitted := len(usages) - count; omitted > 0 {
		locations = append(locations, fmt.Sprintf("+%d more", omitted))
	}
	return strings.Join(locations, "; ")
}

func writeBoundaryCandidate(write func(string, ...any) bool, candidate search.BoundaryCandidate, minimum int) bool {
	if !write("owner: %s [%s]", candidate.OwnerFile, candidate.Language) ||
		!write("  external consumers: %d functions / %d files / %d packages", candidate.Consumers.Functions, candidate.Consumers.Files, candidate.Consumers.Packages) ||
		!write("  external callable surface: %d/%d callables", candidate.ExternalCallableSurface.External, candidate.ExternalCallableSurface.Declared) {
		return false
	}
	if candidate.UnresolvedCalls > 0 && !write("  ! %d interactions use same-owner candidate resolution", candidate.UnresolvedCalls) {
		return false
	}
	if !write("  repeated callable co-usage (minimum %d callers in 2+ files):", minimum) || !writeBoundaryPatterns(write, candidate.CallableCoUsage, " + ") {
		return false
	}
	if !write("  repeated ordered sequences (minimum %d callers in 2+ files):", minimum) || !writeBoundaryPatterns(write, candidate.OrderedSequences, " -> ") {
		return false
	}
	if !write("  repeated member + call combinations (minimum %d callers in 2+ files):", minimum) {
		return false
	}
	return writeBoundaryPatterns(write, candidate.MemberCallCombinations, " + ")
}

func writeBoundaryPatterns(write func(string, ...any) bool, patterns []search.BoundaryPattern, separator string) bool {
	if len(patterns) == 0 {
		return write("    (none)")
	}
	visible := len(patterns)
	if visible > maxHumanBoundaryPatterns {
		visible = maxHumanBoundaryPatterns
	}
	for _, pattern := range patterns[:visible] {
		if !write("    %s", strings.Join(pattern.Interactions, separator)) ||
			!write("      %d occurrences / %d files / %d packages", pattern.Occurrences, pattern.Files, pattern.Packages) ||
			!write("      callers: %s", boundaryPatternLocations(pattern.Consumers)) {
			return false
		}
	}
	if omitted := len(patterns) - visible; omitted > 0 {
		return write("    +%d additional patterns; use --json for complete results", omitted)
	}
	return true
}

func boundaryPatternLocations(consumers []search.BoundaryConsumer) string {
	const maximum = 3
	count := len(consumers)
	if count > maximum {
		count = maximum
	}
	locations := make([]string, 0, count+1)
	for _, consumer := range consumers[:count] {
		locations = append(locations, fmt.Sprintf("%s:%d %s", consumer.Path, consumer.Line, consumer.Name))
	}
	if omitted := len(consumers) - count; omitted > 0 {
		locations = append(locations, fmt.Sprintf("+%d more", omitted))
	}
	return strings.Join(locations, "; ")
}
