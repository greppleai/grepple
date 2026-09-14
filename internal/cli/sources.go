package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alexflint/go-arg"
	"github.com/greppleai/grepple/search"
)

const sourceScopeSchema = "grepple-source-scope-v1"

type sourceExplainArgs struct {
	JSON           bool     `arg:"--json" help:"emit complete source decisions as JSON"`
	Compact        bool     `arg:"--compact" help:"emit a bounded source-scope summary"`
	MaxOutputBytes int      `arg:"--max-output-bytes" placeholder:"N" help:"cap compact output (default 16384; 0 = unlimited)"`
	Paths          []string `arg:"positional" placeholder:"PATH" help:"file or directory to inspect; defaults to the repository root"`
}

type sourceScopeConfig struct {
	Loaded        bool   `json:"loaded"`
	Path          string `json:"path,omitempty"`
	Digest        string `json:"digest,omitempty"`
	IgnoreEnabled bool   `json:"ignoreEnabled"`
}

type sourceScopeCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type sourceScopeReport struct {
	Schema                  string                      `json:"schema"`
	Root                    string                      `json:"root"`
	Config                  sourceScopeConfig           `json:"config"`
	ProductionOnly          bool                        `json:"productionOnly"`
	DiscoveredFiles         int                         `json:"discoveredFiles"`
	SelectedFiles           int                         `json:"selectedFiles"`
	ExcludedFiles           int                         `json:"excludedFiles"`
	OmittedSubtrees         int                         `json:"omittedSubtrees"`
	UnconditionalExclusions []string                    `json:"unconditionalExclusions"`
	Classifications         []sourceScopeCount          `json:"classifications"`
	Exclusions              []sourceScopeCount          `json:"exclusions"`
	Decisions               []search.SourcePathDecision `json:"decisions"`
}

func runSources(args []string) error {
	if len(args) == 0 || isExtractHelp(args[0]) {
		return stdoutWriter().writeString("Explain repository source selection.\nUsage:\n  grepple sources explain (--compact | --json) [PATH ...]\n")
	}
	if args[0] != "explain" {
		return fmt.Errorf("unknown sources command %q", args[0])
	}
	values := sourceExplainArgs{MaxOutputBytes: DefaultTextOutputBytes}
	parser, err := arg.NewParser(arg.Config{Program: "grepple sources explain"}, &values)
	if err != nil {
		return err
	}
	if err := parser.Parse(args[1:]); err != nil {
		if errors.Is(err, arg.ErrHelp) {
			parser.WriteHelp(os.Stdout)
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
	report, err := buildSourceScopeReport(values.Paths)
	if err != nil {
		return err
	}
	if values.JSON {
		return stdoutWriter().writeJSON(report)
	}
	return renderSourceScopeReport(report, values.MaxOutputBytes)
}

func buildSourceScopeReport(paths []string) (sourceScopeReport, error) {
	config, configPath, err := loadRepositoryConfig()
	if err != nil {
		return sourceScopeReport{}, err
	}
	root := mustGetwd()
	if configPath != "" {
		root = filepath.Dir(configPath)
	}
	if len(paths) == 0 {
		paths = []string{root}
	}
	options := search.SourceScopeOptions{Root: root, IgnoreRoot: root, ProductionOnly: activeRepositoryOptions.productionOnly}
	if !activeRepositoryOptions.ignoreDisabled {
		options.IgnorePaths = append([]string(nil), config.Ignore.Paths...)
	}
	decisions, err := search.InspectSourceScope(paths, options)
	if err != nil {
		return sourceScopeReport{}, err
	}
	report := sourceScopeReport{
		Schema: sourceScopeSchema, Root: displayRepositoryPath(root), ProductionOnly: options.ProductionOnly,
		Config:                  sourceScopeConfig{Loaded: configPath != "", Path: displayRepositoryPath(configPath), IgnoreEnabled: configPath != "" && !activeRepositoryOptions.ignoreDisabled},
		Decisions:               decisions,
		UnconditionalExclusions: []string{".git/**", ".grepple/**", ".worktrees/**"},
	}
	if configPath != "" {
		report.Config.Digest, err = fileSHA256(configPath)
		if err != nil {
			return sourceScopeReport{}, err
		}
	}
	classifications := map[string]int{}
	exclusions := map[string]int{}
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

func renderSourceScopeReport(report sourceScopeReport, maxBytes int) error {
	writer := architectureOutputWriter(maxBytes)
	config := "none"
	if report.Config.Loaded {
		config = fmt.Sprintf("%s@%s", report.Config.Path, shortSourceDigest(report.Config.Digest))
	}
	if err := writer.writeString(fmt.Sprintf("sources %s root=%s config=%s config-ignore=%t production-only=%t discovered=%d selected=%d excluded=%d omitted-subtrees=%d unconditional=%s\n", report.Schema, report.Root, config, report.Config.IgnoreEnabled, report.ProductionOnly, report.DiscoveredFiles, report.SelectedFiles, report.ExcludedFiles, report.OmittedSubtrees, strings.Join(report.UnconditionalExclusions, ","))); err != nil {
		return nil
	}
	if err := writer.writeString(fmt.Sprintf("classifications %s\n", formatSourceScopeCounts(report.Classifications))); err != nil {
		return nil
	}
	if err := writer.writeString(fmt.Sprintf("exclusions %s\n", formatSourceScopeCounts(report.Exclusions))); err != nil {
		return nil
	}
	for _, decision := range report.Decisions {
		if decision.Selected && !strings.Contains(decision.Reason, "bypass") {
			continue
		}
		if err := writer.writeString(fmt.Sprintf("%s %s reason=%s class=%s language=%s explicit=%t\n", sourceDecisionMarker(decision), decision.Path, decision.Reason, decision.Classification, decision.Language, decision.Explicit)); err != nil {
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

func sortedSourceScopeCounts(values map[string]int) []sourceScopeCount {
	names := make([]string, 0, len(values))
	for name := range values {
		if name != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	result := make([]sourceScopeCount, 0, len(names))
	for _, name := range names {
		result = append(result, sourceScopeCount{Name: name, Count: values[name]})
	}
	return result
}

func formatSourceScopeCounts(values []sourceScopeCount) string {
	if len(values) == 0 {
		return "none"
	}
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, fmt.Sprintf("%s:%d", value.Name, value.Count))
	}
	return strings.Join(parts, ",")
}

func fileSHA256(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

func shortSourceDigest(digest string) string {
	value := strings.TrimPrefix(digest, "sha256:")
	if len(value) > 12 {
		value = value[:12]
	}
	return "sha256:" + value
}

func displayRepositoryPath(path string) string {
	if path == "" {
		return ""
	}
	working, _ := os.Getwd()
	if relative, err := filepath.Rel(working, path); err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return filepath.ToSlash(relative)
	}
	return filepath.ToSlash(path)
}
