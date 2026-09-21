package sources

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alexflint/go-arg"
	cliruntime "github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/filedigest"
	"github.com/greppleai/grepple/search"
)

const sourceScopeSchema = "grepple-source-scope-v1"

type sourceExplainArgs struct {
	JSON           bool     `arg:"--json" help:"emit complete source decisions as JSON"`
	Compact        bool     `arg:"--compact" help:"emit a bounded source-scope summary"`
	MaxOutputBytes int      `arg:"--max-output-bytes" placeholder:"N" help:"cap compact output (default 16384; 0 = unlimited)"`
	Paths          []string `arg:"positional" placeholder:"PATH" help:"file or directory to inspect; defaults to the repository root"`
}

// Config describes the repository configuration applied to source selection.
type Config struct {
	Loaded        bool   `json:"loaded"`
	Path          string `json:"path,omitempty"`
	Digest        string `json:"digest,omitempty"`
	IgnoreEnabled bool   `json:"ignoreEnabled"`
}

// Count is one deterministic source classification total.
type Count struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// Report describes repository source selection decisions.
type Report struct {
	Schema                  string                      `json:"schema"`
	Root                    string                      `json:"root"`
	Config                  Config                      `json:"config"`
	ProductionOnly          bool                        `json:"productionOnly"`
	DiscoveredFiles         int                         `json:"discoveredFiles"`
	SelectedFiles           int                         `json:"selectedFiles"`
	ExcludedFiles           int                         `json:"excludedFiles"`
	OmittedSubtrees         int                         `json:"omittedSubtrees"`
	UnconditionalExclusions []string                    `json:"unconditionalExclusions"`
	Classifications         []Count                     `json:"classifications"`
	Exclusions              []Count                     `json:"exclusions"`
	Decisions               []search.SourcePathDecision `json:"decisions"`
}

// Run executes the sources command.
func Run(args []string, dependencies Dependencies) error {
	if len(args) == 0 || isHelp(args[0]) {
		return cliruntime.NewOutput(dependencies.stdout()).WriteString("Explain repository source selection.\nUsage:\n  grepple sources explain (--compact | --json) [PATH ...]\n")
	}
	if args[0] != "explain" {
		return fmt.Errorf("unknown sources command %q", args[0])
	}
	values := sourceExplainArgs{MaxOutputBytes: 16 * 1024}
	parser, err := arg.NewParser(arg.Config{Program: "grepple sources explain"}, &values)
	if err != nil {
		return err
	}
	if err := parser.Parse(args[1:]); err != nil {
		if errors.Is(err, arg.ErrHelp) {
			parser.WriteHelp(dependencies.stdout())
			return nil
		}
		return err
	}
	if values.JSON == values.Compact {
		return fmt.Errorf("sources explain requires exactly one of --json or --compact")
	}
	if values.MaxOutputBytes < 0 {
		return fmt.Errorf("--max-output-bytes must not be negative")
	}
	report, err := Build(values.Paths, dependencies)
	if err != nil {
		return err
	}
	if values.JSON {
		return cliruntime.NewOutput(dependencies.stdout()).WriteJSON(report)
	}
	return renderSourceScopeReport(report, values.MaxOutputBytes, dependencies)
}

func isHelp(value string) bool { return value == "--help" || value == "-h" || value == "help" }

// Build inspects and summarizes repository source selection.
func Build(paths []string, dependencies Dependencies) (Report, error) {
	environment, err := dependencies.environment()
	if err != nil {
		return Report{}, err
	}
	root, configPath := environment.Root, environment.ConfigPath
	if len(paths) == 0 {
		paths = []string{root}
	}
	options := search.SourceScopeOptions{Root: root, IgnoreRoot: root, ProductionOnly: environment.ProductionOnly}
	if !environment.IgnoreDisabled {
		options.IgnorePaths = append([]string(nil), environment.IgnorePaths...)
	}
	decisions, err := search.InspectSourceScope(paths, options)
	if err != nil {
		return Report{}, err
	}
	report := Report{
		Schema: sourceScopeSchema, Root: displayRepositoryPath(root, dependencies), ProductionOnly: options.ProductionOnly,
		Config:    Config{Loaded: configPath != "", Path: displayRepositoryPath(configPath, dependencies), IgnoreEnabled: configPath != "" && !environment.IgnoreDisabled},
		Decisions: decisions, UnconditionalExclusions: []string{".git/**", ".grepple/**", ".worktrees/**"},
	}
	if configPath != "" {
		report.Config.Digest, err = filedigest.SHA256(configPath)
		if err != nil {
			return Report{}, err
		}
	}
	classifications, exclusions := map[string]int{}, map[string]int{}
	for _, decision := range decisions {
		if decision.Subtree {
			report.OmittedSubtrees++
			continue
		}
		report.DiscoveredFiles++
		classifications[decision.Classification]++
		if decision.Selected {
			report.SelectedFiles++
		} else {
			report.ExcludedFiles++
			exclusions[decision.Reason]++
		}
	}
	report.Classifications = sortedSourceScopeCounts(classifications)
	report.Exclusions = sortedSourceScopeCounts(exclusions)
	return report, nil
}

func renderSourceScopeReport(report Report, maxBytes int, dependencies Dependencies) error {
	writer := cliruntime.NewBoundedOutput(dependencies.stdout(), maxBytes)
	config := "none"
	if report.Config.Loaded {
		config = fmt.Sprintf("%s@%s", report.Config.Path, shortSourceDigest(report.Config.Digest))
	}
	if err := writer.WriteString(fmt.Sprintf("sources %s root=%s config=%s config-ignore=%t production-only=%t discovered=%d selected=%d excluded=%d omitted-subtrees=%d unconditional=%s\n", report.Schema, report.Root, config, report.Config.IgnoreEnabled, report.ProductionOnly, report.DiscoveredFiles, report.SelectedFiles, report.ExcludedFiles, report.OmittedSubtrees, strings.Join(report.UnconditionalExclusions, ","))); err != nil {
		return nil
	}
	if err := writer.WriteString(fmt.Sprintf("classifications %s\n", FormatCounts(report.Classifications))); err != nil {
		return nil
	}
	if err := writer.WriteString(fmt.Sprintf("exclusions %s\n", FormatCounts(report.Exclusions))); err != nil {
		return nil
	}
	for _, decision := range report.Decisions {
		if decision.Selected && !strings.Contains(decision.Reason, "bypass") {
			continue
		}
		if err := writer.WriteString(fmt.Sprintf("%s %s reason=%s class=%s language=%s explicit=%t\n", sourceDecisionMarker(decision), decision.Path, decision.Reason, decision.Classification, decision.Language, decision.Explicit)); err != nil {
			return nil
		}
	}
	return nil
}

func sourceDecisionMarker(decision search.SourcePathDecision) string {
	if decision.Selected {
		return "S"
	}
	if decision.Subtree {
		return "O"
	}
	return "X"
}

func sortedSourceScopeCounts(values map[string]int) []Count {
	names := make([]string, 0, len(values))
	for name := range values {
		if name != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	result := make([]Count, 0, len(names))
	for _, name := range names {
		result = append(result, Count{Name: name, Count: values[name]})
	}
	return result
}

// FormatCounts renders deterministic source-scope count summaries.
func FormatCounts(values []Count) string {
	if len(values) == 0 {
		return "none"
	}
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, fmt.Sprintf("%s:%d", value.Name, value.Count))
	}
	return strings.Join(parts, ",")
}

func shortSourceDigest(digest string) string {
	value := strings.TrimPrefix(digest, "sha256:")
	if len(value) > 12 {
		value = value[:12]
	}
	return "sha256:" + value
}

func displayRepositoryPath(path string, dependencies Dependencies) string {
	if path == "" {
		return ""
	}
	working := dependencies.workingDirectory()
	if relative, err := filepath.Rel(working, path); err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return filepath.ToSlash(relative)
	}
	return filepath.ToSlash(path)
}
